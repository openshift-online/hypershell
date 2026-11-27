/*
 * Terminal drawer docking and sizing rules (WEB-TERM-02, WEB-TERM-03).
 * Pure calculations plus the preference-store port; the browser storage
 * adapter lives in `adapters/terminal`.
 */

export type TerminalDock = "bottom" | "right";

export interface TerminalViewport {
  readonly height: number;
  readonly width: number;
}

export interface TerminalSizes {
  readonly bottom: number;
  readonly right: number;
}

export interface TerminalLayoutPreferences {
  readonly dock: TerminalDock;
  readonly sizes: TerminalSizes;
}

/** Non-sensitive drawer preferences (WEB-DATA-02); never command history. */
export interface TerminalPreferencesStore {
  read(): TerminalLayoutPreferences;
  write(preferences: TerminalLayoutPreferences): void;
}

/** CSS pixels per arrow-key press on the resize handle. */
export const terminalResizeStep = 24;

export const defaultTerminalPreferences: TerminalLayoutPreferences =
  Object.freeze({
    dock: "bottom",
    sizes: Object.freeze({ bottom: 320, right: 480 }),
  });

const bounds = {
  bottom: { maxShare: 0.7, min: 180, viewportAxis: "height" },
  right: { maxShare: 0.65, min: 320, viewportAxis: "width" },
} as const;

/**
 * Minimum and maximum drawer size for a dock: 180 px to 70% of the viewport
 * height at the bottom, 320 px to 65% of the viewport width at the right. A
 * viewport too small for the minimum keeps the minimum.
 */
export function terminalSizeBounds(
  dock: TerminalDock,
  viewport: TerminalViewport,
): { max: number; min: number } {
  const rule = bounds[dock];
  const max = Math.floor(viewport[rule.viewportAxis] * rule.maxShare);
  return { max: Math.max(rule.min, max), min: rule.min };
}

export function clampTerminalSize(
  dock: TerminalDock,
  size: number,
  viewport: TerminalViewport,
): number {
  const { max, min } = terminalSizeBounds(dock, viewport);
  if (!Number.isFinite(size)) {
    return min;
  }
  return Math.min(max, Math.max(min, Math.round(size)));
}

/** Validates untrusted stored preferences, falling back per field. */
export function parseTerminalPreferences(value: {
  dock: unknown;
  sizes: unknown;
}): TerminalLayoutPreferences {
  const dock =
    value.dock === "right" || value.dock === "bottom"
      ? value.dock
      : defaultTerminalPreferences.dock;
  const sizes =
    typeof value.sizes === "object" && value.sizes !== null
      ? (value.sizes as Record<string, unknown>)
      : {};
  const sizeOf = (key: TerminalDock): number => {
    const candidate = sizes[key];
    return typeof candidate === "number" &&
      Number.isFinite(candidate) &&
      candidate > 0
      ? candidate
      : defaultTerminalPreferences.sizes[key];
  };
  return { dock, sizes: { bottom: sizeOf("bottom"), right: sizeOf("right") } };
}
