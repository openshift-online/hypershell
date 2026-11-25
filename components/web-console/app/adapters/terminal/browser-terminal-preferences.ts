import {
  defaultTerminalPreferences,
  parseTerminalPreferences,
  type TerminalLayoutPreferences,
  type TerminalPreferencesStore,
} from "../../application/terminal/terminal-layout";

/** Console-namespaced keys for the non-sensitive drawer layout (WEB-TERM-02/03). */
export const terminalDockStorageKey = "hypershell.terminal.dock";
export const terminalSizesStorageKey = "hypershell.terminal.sizes";

interface PreferenceStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

function browserStorage(): PreferenceStorage | null {
  try {
    return typeof window === "undefined" ? null : window.localStorage;
  } catch {
    return null;
  }
}

function parseJson(text: string | null): unknown {
  if (text === null) {
    return undefined;
  }
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return undefined;
  }
}

/**
 * Stores only the dock position and the size per position in `localStorage`
 * (WEB-DATA-02). Command history never passes through this adapter. Storage
 * failures (private mode, quota, disabled storage) fall back to defaults.
 */
export function createBrowserTerminalPreferences(
  storage: PreferenceStorage | null = browserStorage(),
): TerminalPreferencesStore {
  return {
    read(): TerminalLayoutPreferences {
      if (storage === null) {
        return defaultTerminalPreferences;
      }
      try {
        return parseTerminalPreferences({
          dock: storage.getItem(terminalDockStorageKey),
          sizes: parseJson(storage.getItem(terminalSizesStorageKey)),
        });
      } catch {
        return defaultTerminalPreferences;
      }
    },
    write(preferences) {
      try {
        storage?.setItem(terminalDockStorageKey, preferences.dock);
        storage?.setItem(
          terminalSizesStorageKey,
          JSON.stringify({
            bottom: preferences.sizes.bottom,
            right: preferences.sizes.right,
          }),
        );
      } catch {
        // A preference that cannot be saved only resets on the next visit.
      }
    },
  };
}
