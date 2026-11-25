import { useQueryClient } from "@tanstack/react-query";
import {
  createContext,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
  type RefObject,
} from "react";

import type { TerminalPreferencesStore } from "../../application/terminal/terminal-layout";
import type { TerminalOperations } from "../../application/terminal/terminal-operations";
import {
  terminalOperations,
  terminalPreferences,
} from "../../composition/terminal-composition";
import { useSession } from "../shell/use-session";
import {
  createTerminalStore,
  type RunbookEntryHandle,
  type TerminalSessionState,
  type TerminalSnapshot,
  type TerminalStore,
} from "./terminal-store";

export type {
  RunbookEntryHandle,
  TerminalSessionState,
} from "./terminal-store";

/** Public terminal API for the shell and docs runbook cells (WEB-UDOC-05). */
export interface TerminalApi {
  readonly isOpen: boolean;
  readonly close: () => void;
  /** Opens the drawer and, with `entryId`, scrolls to that entry. */
  readonly open: (options?: { entryId?: string }) => void;
  /** Appends a history entry tagged `[runbook]` (WEB-TERM-07). */
  readonly startRunbookEntry: (command: string) => RunbookEntryHandle;
  readonly toggle: () => void;
}

interface TerminalContextValue {
  api: Omit<TerminalApi, "isOpen">;
  panelId: string;
  preferences: TerminalPreferencesStore;
  store: TerminalStore;
  triggerRef: RefObject<HTMLButtonElement | null>;
}

const TerminalContext = createContext<TerminalContextValue | null>(null);

function useTerminalContext(): TerminalContextValue {
  const value = useContext(TerminalContext);
  if (value === null) {
    throw new Error("Terminal components must be inside TerminalProvider.");
  }
  return value;
}

export interface TerminalProviderProps {
  children: ReactNode;
  /** Defaults to the browser composition (worker runtime and probes). */
  operations?: TerminalOperations;
  /** Defaults to the console-namespaced `localStorage` preferences. */
  preferences?: TerminalPreferencesStore;
  /** Defaults to the BFF session resource read by `useSession`. */
  session?: TerminalSessionState;
  /** Lowers the 5,000-line limit, for stories and tests. */
  lineLimit?: number;
}

/** Maps the BFF session to the terminal's signed-in state (WEB-TERM-01). */
export function useConsoleTerminalSession(): TerminalSessionState {
  const { data: session, isError } = useSession();
  if (session === undefined) {
    // An unreadable session is treated as signed out, so the drawer offers
    // the sign-in link instead of waiting (WEB-UDOC-03).
    return isError ? { status: "signed-out" } : { status: "unknown" };
  }
  // No-auth development mode treats the reader as signed in.
  if (!session.authEnabled) {
    return { identity: "no-auth", status: "signed-in" };
  }
  return session.authenticated
    ? { identity: session.user?.sub ?? "", status: "signed-in" }
    : { status: "signed-out" };
}

function isTerminalToggleKey(event: KeyboardEvent): boolean {
  return (
    event.ctrlKey &&
    !event.altKey &&
    !event.metaKey &&
    (event.key === "`" || event.code === "Backquote")
  );
}

/**
 * Owns the page-session terminal: one in-memory history shared by typed
 * commands and runbook cells, the drawer open state, Ctrl+` from anywhere,
 * and focus return on close (WEB-TERM-01/07). History is cleared on sign-out
 * and when the session identity changes.
 */
export function TerminalProvider({
  children,
  lineLimit,
  operations = terminalOperations,
  preferences = terminalPreferences,
  session: sessionOverride,
}: TerminalProviderProps) {
  const queryClient = useQueryClient();
  const consoleSession = useConsoleTerminalSession();
  const session = sessionOverride ?? consoleSession;
  const [store] = useState(() =>
    createTerminalStore({
      ...(lineLimit === undefined ? {} : { lineLimit }),
      onGatewayStateMayHaveChanged: () => {
        // Covers the detail, list, and homepage summary roots.
        void queryClient.invalidateQueries({ queryKey: ["gateways"] });
      },
      operations,
    }),
  );
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const returnFocusRef = useRef<Element | null>(null);
  const panelId = useId();
  const isOpen = useSyncExternalStore(
    store.subscribe,
    () => store.getSnapshot().isOpen,
    () => false,
  );
  const wasOpenRef = useRef(isOpen);

  const sessionStatus = session.status;
  const sessionIdentity = session.identity;
  useEffect(() => {
    store.setSession(
      sessionIdentity === undefined
        ? { status: sessionStatus }
        : { identity: sessionIdentity, status: sessionStatus },
    );
  }, [sessionIdentity, sessionStatus, store]);

  const api = useMemo<TerminalContextValue["api"]>(() => {
    const open = (options?: { entryId?: string }) => {
      if (!store.getSnapshot().isOpen) {
        returnFocusRef.current = document.activeElement;
      }
      store.open(options);
    };
    const close = () => {
      store.close();
    };
    return {
      close,
      open,
      startRunbookEntry: (command) => store.startRunbookEntry(command),
      toggle: () => {
        if (store.getSnapshot().isOpen) {
          close();
        } else {
          open();
        }
      },
    };
  }, [store]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (isTerminalToggleKey(event)) {
        event.preventDefault();
        api.toggle();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [api]);

  // Closing returns focus to the element focused before opening, or to the
  // trigger when that element is gone (WEB-TERM-01).
  useEffect(() => {
    const wasOpen = wasOpenRef.current;
    wasOpenRef.current = isOpen;
    if (!wasOpen || isOpen) {
      return;
    }
    const previous = returnFocusRef.current;
    returnFocusRef.current = null;
    if (
      previous instanceof HTMLElement &&
      previous !== document.body &&
      previous.isConnected &&
      !previous.closest(`[data-terminal-panel="${panelId}"]`)
    ) {
      previous.focus();
      if (document.activeElement === previous) {
        return;
      }
    }
    triggerRef.current?.focus();
  }, [isOpen, panelId]);

  const value = useMemo<TerminalContextValue>(
    () => ({ api, panelId, preferences, store, triggerRef }),
    [api, panelId, preferences, store],
  );

  return (
    <TerminalContext.Provider value={value}>
      {children}
    </TerminalContext.Provider>
  );
}

/** The terminal API: open state, open/close/toggle, and runbook entries. */
export function useTerminal(): TerminalApi {
  const { api, store } = useTerminalContext();
  const isOpen = useSyncExternalStore(
    store.subscribe,
    () => store.getSnapshot().isOpen,
    () => false,
  );
  return useMemo(() => ({ ...api, isOpen }), [api, isOpen]);
}

/** Full terminal snapshot, for the drawer and trigger in this feature. */
export function useTerminalSnapshot(): TerminalSnapshot {
  const { store } = useTerminalContext();
  return useSyncExternalStore(
    store.subscribe,
    store.getSnapshot,
    store.getSnapshot,
  );
}

/** Internal wiring for the drawer and trigger in this feature. */
export function useTerminalInternals() {
  return useTerminalContext();
}
