import { describe, expect, it } from "vitest";

import {
  clampTerminalSize,
  defaultTerminalPreferences,
  parseTerminalPreferences,
  terminalSizeBounds,
} from "./terminal-layout";

const viewport = { height: 1000, width: 2000 };

describe("terminal layout", () => {
  // Verifies: WEB-TERM-03.01
  it("clamps bottom sizes to 180 px through 70% of the viewport height", () => {
    expect(terminalSizeBounds("bottom", viewport)).toEqual({
      max: 700,
      min: 180,
    });
    expect(clampTerminalSize("bottom", 166, viewport)).toBe(180);
    expect(clampTerminalSize("bottom", 900, viewport)).toBe(700);
    expect(clampTerminalSize("bottom", 300.4, viewport)).toBe(300);
  });

  // Verifies: WEB-TERM-03
  it("clamps right sizes to 320 px through 65% of the viewport width", () => {
    expect(terminalSizeBounds("right", viewport)).toEqual({
      max: 1300,
      min: 320,
    });
    expect(clampTerminalSize("right", 100, viewport)).toBe(320);
    expect(clampTerminalSize("right", 5000, viewport)).toBe(1300);
  });

  // Verifies: WEB-TERM-03
  it("keeps the minimum on a viewport too small for it", () => {
    expect(terminalSizeBounds("bottom", { height: 200, width: 300 })).toEqual({
      max: 180,
      min: 180,
    });
    expect(clampTerminalSize("right", Number.NaN, viewport)).toBe(320);
  });

  // Verifies: WEB-TERM-02
  it("validates stored preferences field by field", () => {
    expect(
      parseTerminalPreferences({
        dock: "right",
        sizes: { bottom: 250, right: 500 },
      }),
    ).toEqual({ dock: "right", sizes: { bottom: 250, right: 500 } });
    expect(
      parseTerminalPreferences({
        dock: "left",
        sizes: { bottom: -1, right: "x" },
      }),
    ).toEqual(defaultTerminalPreferences);
    expect(parseTerminalPreferences({ dock: null, sizes: null })).toEqual(
      defaultTerminalPreferences,
    );
  });
});
