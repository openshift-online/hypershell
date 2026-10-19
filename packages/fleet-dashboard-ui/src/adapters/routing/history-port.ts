// The single place that touches the browser History API and `window.location`
// (GLASS-NAV-04). Everything above this adapter speaks in framework-free
// RouteLocation values from the domain route codec.

import type { RouteLocation } from "../../domain/navigation/route";

/** A narrow driving port over browser history/location for the route layer. */
export interface HistoryPort {
  /** Read the current location (path + search). */
  read(): RouteLocation;
  /**
   * Build the browser-facing URL for a location, including any reverse-proxy base
   * path (GLASS-NAV-19). Link `<a href>` values must go through this so copy-link,
   * open-in-new-tab, and Ctrl/Cmd-click reach the correct base-path URL, matching
   * what `push`/`replace` navigate to (GLASS-NAV-08).
   */
  href(location: RouteLocation): string;
  /** Push a new history entry (meaningful navigation). */
  push(location: RouteLocation): void;
  /** Replace the current history entry (coalesced/text changes). */
  replace(location: RouteLocation): void;
  /**
   * Subscribe to external history changes (Back/Forward). Returns an unsubscribe.
   * Programmatic push/replace do NOT fire this, so there is no feedback loop
   * (GLASS-NAV-07).
   */
  subscribe(listener: () => void): () => void;
}

function hrefOf(location: RouteLocation): string {
  return `${location.path}${location.search}`;
}

/**
 * An in-memory HistoryPort with a real Back/Forward stack. Used by tests and any
 * non-browser host; it keeps the route layer exercisable without a live `window`.
 */
export function createMemoryHistory(
  initial: RouteLocation = { path: "/", search: "" },
): HistoryPort & { back: () => void; forward: () => void } {
  const stack: RouteLocation[] = [initial];
  let index = 0;
  let listener: (() => void) | null = null;
  return {
    read: () => stack[index] ?? initial,
    // The memory host has no base path, so the link URL is the bare location.
    href: (location) => hrefOf(location),
    push: (location) => {
      stack.splice(index + 1);
      stack.push(location);
      index = stack.length - 1;
    },
    replace: (location) => {
      stack[index] = location;
    },
    subscribe: (cb) => {
      listener = cb;
      return () => {
        if (listener === cb) {
          listener = null;
        }
      };
    },
    back: () => {
      if (index > 0) {
        index -= 1;
        listener?.();
      }
    },
    forward: () => {
      if (index < stack.length - 1) {
        index += 1;
        listener?.();
      }
    },
  };
}

// The application route segments the codec serializes. Used only to derive a
// reverse-proxy base path from the initial location so it round-trips (GLASS-NAV-19).
const TAB_SEGMENTS = new Set(["map", "sdlc", "e2e"]);

/**
 * Derive the deployment base path: the initial pathname with a trailing application
 * route segment removed. At the document root this is the empty string.
 */
function deriveBasePath(pathname: string): string {
  const segments = pathname.split("/").filter((s) => s.length > 0);
  const last = segments[segments.length - 1];
  if (last !== undefined && TAB_SEGMENTS.has(last)) {
    segments.pop();
  }
  const base = `/${segments.join("/")}`;
  return base === "/" ? "" : base;
}

/** Strip the base path from a pathname, leaving the codec's root-relative path. */
function stripBasePath(pathname: string, base: string): string {
  if (base !== "" && pathname.startsWith(base)) {
    const rest = pathname.slice(base.length);
    return rest === "" ? "/" : rest;
  }
  return pathname;
}

/**
 * The production adapter around `window.history` and `window.location`. It captures
 * the reverse-proxy base path once and re-applies it on every push/replace so deep
 * links keep working after in-app navigation and reload (GLASS-NAV-19).
 */
export function createBrowserHistory(): HistoryPort {
  const basePath = deriveBasePath(window.location.pathname);
  return {
    read: () => ({
      path: stripBasePath(window.location.pathname, basePath),
      search: window.location.search,
    }),
    // Re-apply the base path so shown link URLs match what push/replace navigate to.
    href: (location) => basePath + hrefOf(location),
    push: (location) => {
      window.history.pushState(null, "", basePath + hrefOf(location));
    },
    replace: (location) => {
      window.history.replaceState(null, "", basePath + hrefOf(location));
    },
    subscribe: (listener) => {
      window.addEventListener("popstate", listener);
      return () => {
        window.removeEventListener("popstate", listener);
      };
    },
  };
}
