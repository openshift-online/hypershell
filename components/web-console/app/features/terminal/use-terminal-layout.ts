import { useCallback, useState, useSyncExternalStore } from "react";

import {
  clampTerminalSize,
  terminalSizeBounds,
  type TerminalDock,
  type TerminalLayoutPreferences,
  type TerminalPreferencesStore,
} from "../../application/terminal/terminal-layout";

/** PatternFly `md` breakpoint; below it the drawer docks at the bottom. */
export const terminalWideQuery = "(min-width: 48rem)";

function subscribeToResize(listener: () => void) {
  window.addEventListener("resize", listener);
  return () => {
    window.removeEventListener("resize", listener);
  };
}

function subscribeToWideQuery(listener: () => void) {
  const query = window.matchMedia(terminalWideQuery);
  query.addEventListener("change", listener);
  return () => {
    query.removeEventListener("change", listener);
  };
}

export interface TerminalLayout {
  readonly bounds: { readonly max: number; readonly min: number };
  readonly canDockRight: boolean;
  readonly dock: TerminalDock;
  readonly size: number;
  /** Clamps and applies a size; `commit` also saves it. */
  readonly resize: (size: number, commit: boolean) => void;
  readonly setDock: (dock: TerminalDock) => void;
}

/**
 * Dock position and size per position (WEB-TERM-02/03). Preferences are read
 * once and saved through the preference port; the rendered size is clamped
 * again whenever the viewport changes.
 */
export function useTerminalLayout(
  store: TerminalPreferencesStore,
): TerminalLayout {
  const [preferences, setPreferences] = useState<TerminalLayoutPreferences>(
    () => store.read(),
  );
  const width = useSyncExternalStore(
    subscribeToResize,
    () => window.innerWidth,
    () => 1280,
  );
  const height = useSyncExternalStore(
    subscribeToResize,
    () => window.innerHeight,
    () => 800,
  );
  const canDockRight = useSyncExternalStore(
    subscribeToWideQuery,
    () => window.matchMedia(terminalWideQuery).matches,
    () => true,
  );
  const viewport = { height, width };
  const dock: TerminalDock = canDockRight ? preferences.dock : "bottom";
  const size = clampTerminalSize(dock, preferences.sizes[dock], viewport);
  const bounds = terminalSizeBounds(dock, viewport);

  const resize = useCallback(
    (next: number, commit: boolean) => {
      setPreferences((current) => {
        const updated: TerminalLayoutPreferences = {
          ...current,
          sizes: {
            ...current.sizes,
            [dock]: clampTerminalSize(dock, next, { height, width }),
          },
        };
        if (commit) {
          store.write(updated);
        }
        return updated;
      });
    },
    [dock, height, store, width],
  );

  const setDock = useCallback(
    (next: TerminalDock) => {
      setPreferences((current) => {
        const updated = { ...current, dock: next };
        store.write(updated);
        return updated;
      });
    },
    [store],
  );

  return { bounds, canDockRight, dock, resize, setDock, size };
}
