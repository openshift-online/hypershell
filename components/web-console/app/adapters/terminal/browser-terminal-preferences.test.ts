import { describe, expect, it } from "vitest";

import { defaultTerminalPreferences } from "../../application/terminal/terminal-layout";
import {
  createBrowserTerminalPreferences,
  terminalDockStorageKey,
  terminalSizesStorageKey,
} from "./browser-terminal-preferences";

function memoryStorage(initial: Record<string, string> = {}) {
  const values = new Map(Object.entries(initial));
  return {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value);
    },
    values,
  };
}

describe("browser terminal preferences", () => {
  // Verifies: WEB-TERM-02.01
  it("saves and restores the dock position and sizes under console keys", () => {
    const storage = memoryStorage();
    const store = createBrowserTerminalPreferences(storage);
    store.write({ dock: "right", sizes: { bottom: 240, right: 520 } });

    expect(storage.values.get(terminalDockStorageKey)).toBe("right");
    expect(storage.values.get(terminalSizesStorageKey)).toBe(
      JSON.stringify({ bottom: 240, right: 520 }),
    );
    expect(terminalDockStorageKey.startsWith("hypershell.")).toBe(true);
    expect(createBrowserTerminalPreferences(storage).read()).toEqual({
      dock: "right",
      sizes: { bottom: 240, right: 520 },
    });
  });

  // Verifies: WEB-TERM-02
  it("falls back to defaults for missing, malformed, or failing storage", () => {
    expect(createBrowserTerminalPreferences(memoryStorage()).read()).toEqual(
      defaultTerminalPreferences,
    );
    expect(
      createBrowserTerminalPreferences(
        memoryStorage({ [terminalSizesStorageKey]: "{not json" }),
      ).read(),
    ).toEqual(defaultTerminalPreferences);
    expect(createBrowserTerminalPreferences(null).read()).toEqual(
      defaultTerminalPreferences,
    );
    const failing = {
      getItem: () => {
        throw new Error("denied");
      },
      setItem: () => {
        throw new Error("quota");
      },
    };
    const store = createBrowserTerminalPreferences(failing);
    expect(store.read()).toEqual(defaultTerminalPreferences);
    expect(() => {
      store.write(defaultTerminalPreferences);
    }).not.toThrow();
  });
});
