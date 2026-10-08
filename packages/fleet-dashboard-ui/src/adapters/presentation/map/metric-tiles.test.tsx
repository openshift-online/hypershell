import { fireEvent, render } from "@testing-library/react";
import { IntlProvider } from "react-intl";
import { vi } from "vitest";

import { MetricTiles } from "./metric-tiles";

import type { MapNode } from "../../../domain/map/model";

/** The tiles read only this handful of population fields off the node; the rest of a
 *  MapNode is irrelevant here, so a minimal stub keeps the fixture honest and small. */
function nodeWith(fields: {
  sandboxes: number;
  sandboxHistory: readonly number[];
  users: number | null;
  userHistory: readonly number[];
  logins: number | null;
  loginsHistory: readonly number[];
  historyTimes?: readonly number[];
}): MapNode {
  return { historyTimes: [], ...fields } as unknown as MapNode;
}

function renderTiles(
  node: MapNode,
  opts: {
    active?: number | null;
    onActive?: (i: number | null) => void;
  } = {},
): HTMLElement {
  const { container } = render(
    <IntlProvider locale="en" defaultLocale="en">
      <MetricTiles
        node={node}
        active={opts.active ?? null}
        onActive={opts.onActive ?? (() => undefined)}
      />
    </IntlProvider>,
  );
  return container;
}

/** The interactive tile row: the shared-cursor slider. */
function row(container: HTMLElement): HTMLElement {
  const el = container.querySelector<HTMLElement>('[role="slider"]');
  if (!el) throw new Error("expected an interactive tile row");
  return el;
}

/** The three tile headline numbers, left to right. (CSS-module class names are hashed,
 *  so match on the stable substring rather than an exact class.) */
function headlines(container: HTMLElement): (string | null)[] {
  return Array.from(
    container.querySelectorAll('[class*="metricTileValue"]'),
  ).map((e) => e.textContent);
}

describe("MetricTiles", () => {
  it("renders the three population headline numbers", () => {
    const container = renderTiles(
      nodeWith({
        sandboxes: 2,
        sandboxHistory: [1, 2],
        users: 7,
        userHistory: [6, 7],
        logins: 4,
        loginsHistory: [3, 4],
        historyTimes: [100, 200],
      }),
    );
    expect(headlines(container)).toEqual(["2", "7", "4"]);
  });

  it("shows 0 for a null user/login count rather than blank", () => {
    const container = renderTiles(
      nodeWith({
        sandboxes: 0,
        sandboxHistory: [],
        users: null,
        userHistory: [],
        logins: null,
        loginsHistory: [],
      }),
    );
    // The three tiles each carry an aria-label; the wrapping row group does not.
    const tiles = Array.from(
      container.querySelectorAll("[role='group'][aria-label]"),
    );
    expect(tiles).toHaveLength(3);
    for (const g of tiles) {
      expect(g.getAttribute("aria-label")).toMatch(/: 0$/);
    }
  });

  it("draws a sparkline only when a population has >= 2 samples", () => {
    const container = renderTiles(
      nodeWith({
        sandboxes: 2,
        sandboxHistory: [1, 2, 3],
        users: 7,
        userHistory: [7], // single sample => no sparkline
        logins: 4,
        loginsHistory: [],
      }),
    );
    // Only the sandbox tile has >= 2 samples, so exactly one sparkline svg is drawn.
    // (Scope to the sparkline svg; the info "i" tooltips render svg icons too.)
    expect(
      container.querySelectorAll('svg[class*="metricTileSpark"]'),
    ).toHaveLength(1);
  });

  it("reflects the hovered sample across all three headlines when active", () => {
    const container = renderTiles(
      nodeWith({
        sandboxes: 9, // live value
        sandboxHistory: [1, 2, 3],
        users: 99,
        userHistory: [6, 7, 8],
        logins: 50,
        loginsHistory: [3, 4, 5],
        historyTimes: [100, 200, 300],
      }),
      { active: 0 },
    );
    // active=0 => the oldest sample, not the live scalars.
    expect(headlines(container)).toEqual(["1", "6", "3"]);
  });

  it("falls back to the live scalar when the cursor outruns a short series", () => {
    const container = renderTiles(
      nodeWith({
        sandboxes: 9,
        sandboxHistory: [1, 2], // only 2 samples
        users: 99,
        userHistory: [6, 7, 8],
        logins: 50,
        loginsHistory: [3, 4, 5],
        historyTimes: [100, 200, 300],
      }),
      { active: 2 }, // out of range for sandboxHistory
    );
    // Sandboxes falls back to the live 9; the others have the sample.
    expect(headlines(container)).toEqual(["9", "8", "5"]);
  });

  it("drives the shared cursor from the keyboard", () => {
    const onActive = vi.fn();
    const node = nodeWith({
      sandboxes: 2,
      sandboxHistory: [1, 2, 3],
      users: 7,
      userHistory: [6, 7, 8],
      logins: 4,
      loginsHistory: [3, 4, 5],
      historyTimes: [100, 200, 300],
    });

    // From idle, arrowing starts at the newest sample (n-1).
    let container = renderTiles(node, { active: null, onActive });
    fireEvent.keyDown(row(container), { key: "ArrowRight" });
    expect(onActive).toHaveBeenLastCalledWith(2);
    fireEvent.keyDown(row(container), { key: "ArrowLeft" });
    expect(onActive).toHaveBeenLastCalledWith(2);
    fireEvent.keyDown(row(container), { key: "Home" });
    expect(onActive).toHaveBeenLastCalledWith(0);
    fireEvent.keyDown(row(container), { key: "End" });
    expect(onActive).toHaveBeenLastCalledWith(2);

    // With a sample active, stepping and Escape behave relative to it.
    onActive.mockClear();
    container = renderTiles(node, { active: 1, onActive });
    fireEvent.keyDown(row(container), { key: "ArrowLeft" });
    expect(onActive).toHaveBeenLastCalledWith(0);
    fireEvent.keyDown(row(container), { key: "ArrowRight" });
    expect(onActive).toHaveBeenLastCalledWith(2);
    fireEvent.keyDown(row(container), { key: "Escape" });
    expect(onActive).toHaveBeenLastCalledWith(null);
  });

  it("is not interactive without at least two samples on the shared axis", () => {
    const container = renderTiles(
      nodeWith({
        sandboxes: 1,
        sandboxHistory: [1],
        users: 1,
        userHistory: [1],
        logins: 1,
        loginsHistory: [1],
        historyTimes: [100],
      }),
    );
    expect(container.querySelector('[role="slider"]')).toBeNull();
  });
});
