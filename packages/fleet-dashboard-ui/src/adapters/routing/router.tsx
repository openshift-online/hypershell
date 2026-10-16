// The routing driving adapter: it makes the URL the authoritative source of route
// state (GLASS-NAV-05, GLASS-NAV-07), applies the push-versus-replace policy
// (GLASS-NAV-06), and lets components read route state, dispatch typed navigation
// actions, and build real link hrefs (GLASS-NAV-08) without touching the History
// API themselves (GLASS-NAV-04).

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

import {
  isReplaceAction,
  reduceRoute,
  type NavAction,
} from "../../domain/navigation/actions";
import {
  parseRoute,
  routeToHref,
  type RouteState,
} from "../../domain/navigation/route";
import { createBrowserHistory, type HistoryPort } from "./history-port";

interface NavigateOptions {
  /** Force a history replace, overriding the per-action push/replace policy. */
  readonly replace?: boolean;
}

interface RouterValue {
  /** The current, URL-derived route state. */
  readonly route: RouteState;
  /** Dispatch a typed navigation action (push/replace decided by policy). */
  readonly navigate: (action: NavAction, options?: NavigateOptions) => void;
  /** Build an href for the state an action would produce (for `<a href>`). */
  readonly hrefFor: (action: NavAction) => string;
}

const RouterContext = createContext<RouterValue | null>(null);

export function RouterProvider({
  history,
  children,
}: {
  readonly history?: HistoryPort;
  readonly children: React.ReactNode;
}): React.ReactElement {
  // A single history adapter for the lifetime of the provider (created once).
  const [hist] = useState<HistoryPort>(() => history ?? createBrowserHistory());

  // Parse the URL synchronously on first render, before any data resolves
  // (GLASS-NAV-05).
  const [route, setRoute] = useState<RouteState>(() => parseRoute(hist.read()));

  // Mirror the live route into a ref so `navigate`, which only runs in event
  // handlers and effects, always reduces from the latest state without closing over
  // a stale value. The ref is synced in an effect (never written during render).
  const routeRef = useRef(route);
  useEffect(() => {
    routeRef.current = route;
  }, [route]);

  // Reconcile from the URL on Back/Forward. The URL is authoritative; we only
  // read it here and never echo it back as a new entry (GLASS-NAV-07).
  useEffect(() => {
    return hist.subscribe(() => {
      setRoute(parseRoute(hist.read()));
    });
  }, [hist]);

  const navigate = useCallback(
    (action: NavAction, options?: NavigateOptions) => {
      const current = routeRef.current;
      const next = reduceRoute(current, action);
      const nextHref = routeToHref(next);
      // A no-op navigation must not create a duplicate history entry.
      if (nextHref === routeToHref(current)) {
        return;
      }
      const location = {
        path: href2path(nextHref),
        search: href2search(nextHref),
      };
      if (options?.replace ?? isReplaceAction(action)) {
        hist.replace(location);
      } else {
        hist.push(location);
      }
      routeRef.current = next;
      setRoute(next);
    },
    [hist],
  );

  // hrefFor runs during render (links compute their href), so it reduces from the
  // route state directly rather than the ref.
  const hrefFor = useCallback(
    (action: NavAction) => routeToHref(reduceRoute(route, action)),
    [route],
  );

  const value = useMemo<RouterValue>(
    () => ({ route, navigate, hrefFor }),
    [route, navigate, hrefFor],
  );

  return (
    <RouterContext.Provider value={value}>{children}</RouterContext.Provider>
  );
}

function href2path(href: string): string {
  const i = href.indexOf("?");
  return i === -1 ? href : href.slice(0, i);
}

function href2search(href: string): string {
  const i = href.indexOf("?");
  return i === -1 ? "" : href.slice(i);
}

/** Read the router. Throws if used outside a RouterProvider (composition error). */
export function useRouter(): RouterValue {
  const value = useContext(RouterContext);
  if (value === null) {
    throw new Error("useRouter must be used within a RouterProvider");
  }
  return value;
}

/** Convenience: the current route state. */
export function useRoute(): RouteState {
  return useRouter().route;
}

/**
 * Props for a real link that performs a client-side navigation on a plain click
 * but lets the browser handle copy-link, open-in-new-tab, and Ctrl/Cmd-click
 * (GLASS-NAV-08). Spread onto an `<a>` or a PatternFly `component="a"` element.
 */
export function useNavLink(action: NavAction): {
  readonly href: string;
  readonly onClick: (event: React.MouseEvent) => void;
} {
  const { hrefFor, navigate } = useRouter();
  const href = hrefFor(action);
  // Not memoized: callers pass a fresh inline `action` object each render, so a
  // useCallback keyed on `action` could never stabilize. A plain closure is both
  // correct and cheaper than a memo that always misses.
  const onClick = (event: React.MouseEvent) => {
    // Defer to the browser for modified clicks and non-primary buttons so
    // open-in-new-tab and background-tab clicks keep working.
    if (
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey
    ) {
      return;
    }
    event.preventDefault();
    navigate(action);
  };
  return { href, onClick };
}
