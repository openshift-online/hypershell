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
    // Single-cloud-hub assumption: every stage HUB runs on the one hub-hosting
    // provider (ibm), so they share one centered spine.
    const model = buildMapModel(
      promotion(["int", "stage", "prod"], {
        int: env({ name: "int", role: "hub", provider: "ibm" }),
        stage: env({ name: "stage", role: "hub", provider: "ibm" }),
        prod: env({ name: "prod", role: "hub", provider: "ibm" }),
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

  it("still keeps every hub on the spine even when hubs span providers", () => {
    // Out-of-scope multi-cloud-hub case (the spec defers the lane/spine tension): the
    // absolute R1 guarantee - every HUB rides the one spine - must still hold, driven
    // by role alone, even when the hubs are not all in one provider lane.
    const model = buildMapModel(
      promotion(["int", "stage", "prod"], {
        int: env({ name: "int", role: "hub", provider: "aws" }),
        stage: env({ name: "stage", role: "hub", provider: "ibm" }),
        prod: env({ name: "prod", role: "hub", provider: "gcp" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const boxes = boxesById(layout);
    expect(boxes.get("int")?.cy).toBe(layout.hubSpineY);
    expect(boxes.get("stage")?.cy).toBe(layout.hubSpineY);
    expect(boxes.get("prod")?.cy).toBe(layout.hubSpineY);
  });

  it("keeps the spine centered while spokes group by provider", () => {
    // HUBs run on one provider (ibm) and SPOKEs on others: the hubs still share the
    // centered spine and grouping the spokes into their provider lanes does not move
    // it off center.
    const model = buildMapModel(
      promotion(["int", "int-s", "prod", "prod-s"], {
        int: env({ name: "int", role: "hub", provider: "ibm" }),
        "int-s": env({ name: "int-s", role: "spoke", provider: "aws" }),
        prod: env({ name: "prod", role: "hub", provider: "ibm" }),
        "prod-s": env({ name: "prod-s", role: "spoke", provider: "aws" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const boxes = boxesById(layout);
    expect(boxes.get("int")?.cy).toBe(layout.hubSpineY);
    expect(boxes.get("prod")?.cy).toBe(layout.hubSpineY);
    expect(boxes.get("int-s")?.cy).not.toBe(layout.hubSpineY);
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
          int: env({ name: "int", role: "hub", provider: "ibm" }),
          "int-s": env({ name: "int-s", role: "spoke", provider: "aws" }),
          prod: env({ name: "prod", role: "hub", provider: "ibm" }),
        }),
        noFleet,
      );
    const a = computeLayout(build());
    const b = computeLayout(build());
    expect(a.hubSpineY).toBe(b.hubSpineY);
    expect(a.nodes).toEqual(b.nodes);
    expect(a.lanes).toEqual(b.lanes);
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

describe("computeLayout - provider lanes", () => {
  const boxesById = (layout: ReturnType<typeof computeLayout>) =>
    new Map(layout.nodes.map((n) => [n.id, n]));
  const laneByKey = (layout: ReturnType<typeof computeLayout>) =>
    new Map(layout.lanes.map((l) => [l.key, l]));

  it("renders one visible lane per cloud provider", () => {
    // AWS clusters and IBM clusters -> an AWS lane and an IBM lane as visible bands.
    const model = buildMapModel(
      promotion(["prod", "prod-s"], {
        prod: env({ name: "prod", role: "hub", provider: "ibm" }),
        "prod-s": env({ name: "prod-s", role: "spoke", provider: "aws" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const lanes = laneByKey(layout);
    expect([...lanes.keys()].sort()).toEqual(["aws", "ibm"]);
    const boxes = boxesById(layout);
    // Each cluster falls inside the vertical band of the provider it runs on.
    const inLane = (box: { cy: number } | undefined, key: string) => {
      const lane = lanes.get(key);
      return (
        box !== undefined &&
        lane !== undefined &&
        box.cy >= lane.y &&
        box.cy <= lane.y + lane.height
      );
    };
    expect(inLane(boxes.get("prod"), "ibm")).toBe(true);
    expect(inLane(boxes.get("prod-s"), "aws")).toBe(true);
  });

  it("groups a provider-less cluster into a single unassigned lane", () => {
    const model = buildMapModel(
      promotion(["prod", "prod-s"], {
        prod: env({ name: "prod", role: "hub", provider: "ibm" }),
        // No provider on the spoke -> it must land in the "none" (unassigned) lane,
        // and no provider name may be invented for it.
        "prod-s": env({ name: "prod-s", role: "spoke", provider: null }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const lanes = laneByKey(layout);
    expect(lanes.has("none")).toBe(true);
    expect(lanes.get("none")?.provider).toBeNull();
  });

  it("centers the hub-hosting lane and stacks the others around it", () => {
    // A hub-hosting IBM lane and a spoke-only AWS lane: the IBM lane carries the
    // centered spine; the AWS lane is placed off the spine.
    const model = buildMapModel(
      promotion(["prod", "prod-s"], {
        prod: env({ name: "prod", role: "hub", provider: "ibm" }),
        "prod-s": env({ name: "prod-s", role: "spoke", provider: "aws" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const lanes = laneByKey(layout);
    const ibm = lanes.get("ibm");
    const aws = lanes.get("aws");
    // The hub-hosting lane straddles the centered spine.
    expect(ibm).toBeDefined();
    expect(layout.hubSpineY).toBeGreaterThanOrEqual(ibm?.y ?? 0);
    expect(layout.hubSpineY).toBeLessThanOrEqual(
      (ibm?.y ?? 0) + (ibm?.height ?? 0),
    );
    // The spoke-only lane does not cross the spine.
    expect(aws).toBeDefined();
    const awsCrossesSpine =
      (aws?.y ?? 0) <= layout.hubSpineY &&
      layout.hubSpineY <= (aws?.y ?? 0) + (aws?.height ?? 0);
    expect(awsCrossesSpine).toBe(false);
    expect(ibm?.hostsHub).toBe(true);
    expect(aws?.hostsHub).toBe(false);
  });

  it("groups spokes into their provider lane while hubs ride the centered spine", () => {
    // HUBs on IBM, SPOKEs on IBM and AWS: the IBM hubs sit on the centered spine, the
    // AWS spokes group into a visible AWS lane off the spine, the IBM spokes group
    // into the IBM lane near the spine.
    const model = buildMapModel(
      promotion(["prod", "prod-aws", "prod-ibm"], {
        prod: env({ name: "prod", role: "hub", provider: "ibm" }),
        "prod-aws": env({ name: "prod-aws", role: "spoke", provider: "aws" }),
        "prod-ibm": env({ name: "prod-ibm", role: "spoke", provider: "ibm" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const boxes = boxesById(layout);
    const lanes = laneByKey(layout);
    // IBM hub on the spine.
    expect(boxes.get("prod")?.cy).toBe(layout.hubSpineY);
    // The AWS spoke rides the AWS lane, off the spine.
    const aws = lanes.get("aws");
    const awsSpoke = boxes.get("prod-aws");
    expect(awsSpoke?.cy).not.toBe(layout.hubSpineY);
    expect(awsSpoke?.cy).toBeGreaterThanOrEqual(aws?.y ?? 0);
    expect(awsSpoke?.cy).toBeLessThanOrEqual(
      (aws?.y ?? 0) + (aws?.height ?? 0),
    );
    // The IBM spoke stays in the IBM (hub-hosting) lane, near the spine.
    const ibm = lanes.get("ibm");
    const ibmSpoke = boxes.get("prod-ibm");
    expect(ibmSpoke?.cy).not.toBe(layout.hubSpineY);
    expect(ibmSpoke?.cy).toBeGreaterThanOrEqual(ibm?.y ?? 0);
    expect(ibmSpoke?.cy).toBeLessThanOrEqual(
      (ibm?.y ?? 0) + (ibm?.height ?? 0),
    );
  });

  it("does not sink the hub-hosting lane below the other lanes", () => {
    // The hub runs on provider "zeta" (sorts last) with a spoke on "alpha": the zeta
    // lane must still carry the centered spine, not be pushed below the alpha lane.
    const model = buildMapModel(
      promotion(["prod", "prod-s"], {
        prod: env({ name: "prod", role: "hub", provider: "zeta" }),
        "prod-s": env({ name: "prod-s", role: "spoke", provider: "alpha" }),
      }),
      noFleet,
    );
    const layout = computeLayout(model);
    const lanes = laneByKey(layout);
    const zeta = lanes.get("zeta");
    const alpha = lanes.get("alpha");
    expect(zeta?.hostsHub).toBe(true);
    // The hub-hosting lane carries the spine; the other lane is strictly off it.
    expect(layout.hubSpineY).toBeGreaterThanOrEqual(zeta?.y ?? 0);
    expect(layout.hubSpineY).toBeLessThanOrEqual(
      (zeta?.y ?? 0) + (zeta?.height ?? 0),
    );
    expect(alpha?.centerY).not.toBe(zeta?.centerY);
  });
});
