import { render } from "@testing-library/react";
import { IntlProvider } from "react-intl";

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
}): MapNode {
  return fields as unknown as MapNode;
}

function renderTiles(node: MapNode): HTMLElement {
  const { container } = render(
    <IntlProvider locale="en" defaultLocale="en">
      <MetricTiles node={node} />
    </IntlProvider>,
  );
  return container;
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
      }),
    );
    const values = Array.from(
      container.querySelectorAll("[role='group'] span:nth-child(2)"),
    ).map((e) => e.textContent);
    expect(values).toEqual(["2", "7", "4"]);
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
    const groups = container.querySelectorAll("[role='group']");
    expect(groups).toHaveLength(3);
    for (const g of groups) {
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
    expect(container.querySelectorAll("svg")).toHaveLength(1);
  });
});
