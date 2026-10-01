import { describe, expect, it } from "vitest";

import type { FleetData } from "../fleet";
import type { PromotionData, PromotionEnvironment } from "../promotion";
import { COL_GAP, computeLayout, MARGIN_X, NODE_H, NODE_W } from "./layout";
import { buildMapModel } from "./model";

function env(
  overrides: Partial<PromotionEnvironment> & { name: string },
): PromotionEnvironment {
  return {
    activeRelease: null,
    proposedRelease: null,
    gates: [],
    activeDigest: null,
    proposedDigest: null,
    role: null,
    provider: null,
    envLabel: null,
    cluster: null,
    argoHealth: null,
    argoSync: null,
    consoleUrl: null,
    argoUrl: null,
    prState: null,
    prUrl: null,
    analysisUrl: null,
    upToDate: false,
    ...overrides,
  };
}

function promotion(
  order: string[],
  environments: Record<string, PromotionEnvironment>,
): PromotionData {
  return {
    order,
    environments,
    releases: [],
    releaseByDigest: {},
    frontier: null,
  };
}

const noFleet: FleetData = { instances: [] };

describe("computeLayout", () => {
  it("spaces columns evenly left to right", () => {
    const model = buildMapModel(
      promotion(["a", "b", "c"], {
        a: env({ name: "a", envLabel: "a", role: "hub" }),
        b: env({ name: "b", envLabel: "b", role: "hub" }),
        c: env({ name: "c", envLabel: "c", role: "hub" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    expect(layout.columns.map((c) => c.x)).toEqual([
      MARGIN_X,
      MARGIN_X + (NODE_W + COL_GAP),
      MARGIN_X + 2 * (NODE_W + COL_GAP),
    ]);
  });

  it("gives every node a box and centres", () => {
    const model = buildMapModel(
      promotion(["a"], { a: env({ name: "a", role: "hub" }) }),
      noFleet,
    );
    const layout = computeLayout(model);
    const box = layout.nodes[0];
    expect(box?.w).toBe(NODE_W);
    expect(box?.h).toBe(NODE_H);
    expect(box?.cx).toBe((box?.x ?? 0) + NODE_W / 2);
    expect(box?.cy).toBe((box?.y ?? 0) + NODE_H / 2);
  });

  it("lays sequential promotion envs in one cloud left -> right, not stacked", () => {
    // Two instances in the same cloud (lane) are distinct promotion envs, so they
    // read across (one column each) instead of stacking in a single env column.
    const model = buildMapModel(
      promotion(["s1", "s2"], {
        s1: env({ name: "s1", envLabel: "int", role: "hub", provider: "ibm" }),
        s2: env({ name: "s2", envLabel: "int", role: "hub", provider: "ibm" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    expect(layout.nodes).toHaveLength(2);
    const [a, b] = layout.nodes;
    expect(a?.y).toBe(b?.y); // same cloud lane -> same row
    expect((b?.x ?? 0) - (a?.x ?? 0)).toBe(NODE_W + COL_GAP); // adjacent columns
  });

  it("groups consecutive columns sharing an env-type into one header band", () => {
    const model = buildMapModel(
      promotion(["a", "b", "c"], {
        a: env({ name: "a", envLabel: "int", role: "hub" }),
        b: env({ name: "b", envLabel: "int", role: "hub" }),
        c: env({ name: "c", envLabel: "prod", role: "hub" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    expect(layout.bands.map((x) => x.label)).toEqual(["int", "prod"]);
    const [intBand, prodBand] = layout.bands;
    const [colA, , colC] = layout.columns;
    // int band spans columns a..b; prod band covers just column c.
    expect(intBand?.x).toBe(colA?.x);
    expect(intBand?.width).toBe(NODE_W + COL_GAP + NODE_W);
    expect(prodBand?.x).toBe(colC?.x);
    expect(prodBand?.width).toBe(NODE_W);
  });

  it("positions a gate midway between its two columns on the hub lane", () => {
    const model = buildMapModel(
      promotion(["a", "b"], {
        a: env({ name: "a", envLabel: "a", role: "hub" }),
        b: env({ name: "b", envLabel: "b", role: "hub" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const hub = layout.lanes.find((l) => l.hostsHub);
    const [colA, colB] = layout.columns;
    expect(layout.gates).toHaveLength(1);
    expect(layout.gates[0]?.x).toBe(
      ((colA?.x ?? 0) + NODE_W + (colB?.x ?? 0)) / 2,
    );
    expect(layout.gates[0]?.y).toBe(hub?.centerY);
  });

  it("falls back to the vertical midpoint for gates when there is no hub lane", () => {
    const model = buildMapModel(
      promotion(["a", "b"], {
        a: env({ name: "a", envLabel: "a", role: "spoke" }),
        b: env({ name: "b", envLabel: "b", role: "spoke" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    expect(layout.gates).toHaveLength(1);
    expect(layout.gates[0]?.y).toBeGreaterThan(0);
  });

  it("reports a bounding box large enough for all content", () => {
    const model = buildMapModel(
      promotion(["a", "b"], {
        a: env({ name: "a", envLabel: "a", role: "hub" }),
        b: env({ name: "b", envLabel: "b", role: "spoke", provider: "aws" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const rightmost = Math.max(...layout.nodes.map((n) => n.x + n.w));
    const lowest = Math.max(...layout.nodes.map((n) => n.y + n.h));
    expect(layout.width).toBeGreaterThanOrEqual(rightmost);
    expect(layout.height).toBeGreaterThanOrEqual(lowest);
  });

  it("handles an empty model without throwing", () => {
    const layout = computeLayout(buildMapModel(promotion([], {}), noFleet));
    expect(layout.nodes).toHaveLength(0);
    expect(layout.width).toBeGreaterThan(0);
    expect(layout.height).toBeGreaterThan(0);
  });
});
