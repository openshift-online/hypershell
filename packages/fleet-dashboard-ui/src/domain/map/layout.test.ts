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
    syncRetries: null,
    consoleUrl: null,
    grafanaUrl: null,
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

  it("positions a gate midway between its two columns on the hub spine", () => {
    const model = buildMapModel(
      promotion(["a", "b"], {
        a: env({ name: "a", envLabel: "a", role: "hub" }),
        b: env({ name: "b", envLabel: "b", role: "hub" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const [colA, colB] = layout.columns;
    // One gate per column: a's midway to b, plus b's terminal gate a half-column
    // past the final column. Both ride the hub spine.
    expect(layout.gates).toHaveLength(2);
    expect(layout.gates[0]?.x).toBe(
      ((colA?.x ?? 0) + NODE_W + (colB?.x ?? 0)) / 2,
    );
    expect(layout.gates[0]?.y).toBe(layout.hubSpineY);
    expect(layout.gates[1]?.y).toBe(layout.hubSpineY);
    expect(layout.gates[1]?.x).toBe((colB?.x ?? 0) + NODE_W + COL_GAP / 2);
  });

  it("falls back to the vertical midpoint for the spine when no node is a hub", () => {
    const model = buildMapModel(
      promotion(["a", "b"], {
        a: env({ name: "a", envLabel: "a", role: "spoke" }),
        b: env({ name: "b", envLabel: "b", role: "spoke" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    expect(layout.gates).toHaveLength(2);
    // Spine is the vertical midpoint of the laid-out content; gates + change ride it.
    expect(layout.hubSpineY).toBe((96 + (layout.height - 48)) / 2);
    expect(layout.gates[0]?.y).toBe(layout.hubSpineY);
    expect(layout.gates[1]?.y).toBe(layout.hubSpineY);
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

describe("computeLayout - hub spine", () => {
  const boxesById = (layout: ReturnType<typeof computeLayout>) =>
    new Map(layout.nodes.map((n) => [n.id, n]));

  it("puts every column's hub on one spine at the vertical center", () => {
    const model = buildMapModel(
      promotion(["int", "stage", "prod"], {
        int: env({ name: "int", role: "hub", provider: "aws" }),
        stage: env({ name: "stage", role: "hub", provider: "ibm" }),
        prod: env({ name: "prod", role: "hub", provider: "gcp" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const centers = layout.nodes.map((n) => n.cy);
    // Every hub shares the one spine Y...
    for (const cy of centers) {
      expect(cy).toBe(layout.hubSpineY);
    }
    // ...and that spine is the vertical center of the laid-out content.
    expect(layout.hubSpineY).toBe((96 + (layout.height - 48)) / 2);
  });

  it("keeps the spine centered regardless of provider lane", () => {
    // Two hubs on different providers plus spokes: the hubs still share the spine
    // and no provider grouping moves it off center.
    const model = buildMapModel(
      promotion(["int", "int-s", "prod", "prod-s"], {
        int: env({ name: "int", role: "hub", provider: "aws" }),
        "int-s": env({ name: "int-s", role: "spoke", provider: "ibm" }),
        prod: env({ name: "prod", role: "hub", provider: "ibm" }),
        "prod-s": env({ name: "prod-s", role: "spoke", provider: "aws" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const boxes = boxesById(layout);
    expect(boxes.get("int")?.cy).toBe(layout.hubSpineY);
    expect(boxes.get("prod")?.cy).toBe(layout.hubSpineY);
    expect(layout.hubSpineY).toBe((96 + (layout.height - 48)) / 2);
  });

  it("places the hub on the spine even when a spoke precedes it in input", () => {
    const model = buildMapModel(
      promotion(["stage-s", "stage"], {
        "stage-s": env({ name: "stage-s", role: "spoke", provider: "aws" }),
        stage: env({ name: "stage", role: "hub", provider: "ibm" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const boxes = boxesById(layout);
    expect(boxes.get("stage")?.cy).toBe(layout.hubSpineY);
    expect(boxes.get("stage-s")?.cy).not.toBe(layout.hubSpineY);
  });

  it("stacks two spokes symmetrically, one above and one below the spine", () => {
    const model = buildMapModel(
      promotion(["s", "s-a", "s-b"], {
        s: env({ name: "s", role: "hub", provider: "ibm" }),
        "s-a": env({ name: "s-a", role: "spoke", provider: "aws" }),
        "s-b": env({ name: "s-b", role: "spoke", provider: "gcp" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const boxes = boxesById(layout);
    const hub = boxes.get("s")?.cy ?? 0;
    expect(hub).toBe(layout.hubSpineY);
    const spokeCys = [boxes.get("s-a")?.cy ?? 0, boxes.get("s-b")?.cy ?? 0];
    expect(spokeCys.some((cy) => cy < hub)).toBe(true); // one above
    expect(spokeCys.some((cy) => cy > hub)).toBe(true); // one below
  });

  it("is deterministic: identical coordinates on repeated computation", () => {
    const build = () =>
      buildMapModel(
        promotion(["int", "int-s", "prod"], {
          int: env({ name: "int", role: "hub", provider: "aws" }),
          "int-s": env({ name: "int-s", role: "spoke", provider: "ibm" }),
          prod: env({ name: "prod", role: "hub", provider: "gcp" }),
        }),
        noFleet,
      );
    const a = computeLayout(build());
    const b = computeLayout(build());
    expect(a.hubSpineY).toBe(b.hubSpineY);
    expect(a.nodes).toEqual(b.nodes);
    expect(a.gates).toEqual(b.gates);
  });

  it("does not sink a hub-hosting provider below other providers' spokes", () => {
    // The hub runs on provider "zeta" (sorts last alphabetically) while a spoke runs
    // on "alpha". Role, not provider, decides the row: the hub stays on the spine.
    const model = buildMapModel(
      promotion(["prod-s", "prod"], {
        "prod-s": env({ name: "prod-s", role: "spoke", provider: "alpha" }),
        prod: env({ name: "prod", role: "hub", provider: "zeta" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const boxes = boxesById(layout);
    expect(boxes.get("prod")?.cy).toBe(layout.hubSpineY);
    expect(boxes.get("prod-s")?.cy).not.toBe(layout.hubSpineY);
  });
});
