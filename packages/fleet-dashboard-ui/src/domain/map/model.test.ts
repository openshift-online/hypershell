import { describe, expect, it } from "vitest";

import { ZERO_RATE, type FleetData, type InstanceFleet } from "../fleet";
import type { PromotionData, PromotionEnvironment } from "../promotion";
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

function inst(
  overrides: Partial<InstanceFleet> & { instance: string },
): InstanceFleet {
  return {
    role: null,
    provider: null,
    gateways: {},
    gatewaysTotal: 0,
    managedClusters: null,
    users: null,
    rpc: ZERO_RATE,
    reconcile: ZERO_RATE,
    bff: ZERO_RATE,
    provisionP95Ms: null,
    gatewayHistory: [],
    ...overrides,
  };
}

function promotion(
  order: string[],
  environments: Record<string, PromotionEnvironment>,
  extra: Partial<PromotionData> = {},
): PromotionData {
  return {
    order,
    environments,
    releases: [],
    releaseByDigest: {},
    frontier: null,
    ...extra,
  };
}

const emptyFleet: FleetData = { instances: [] };

describe("buildMapModel - columns", () => {
  it("groups nodes into columns by envLabel in server order", () => {
    const model = buildMapModel(
      promotion(["hub-int", "spoke-int", "hub-prod"], {
        "hub-int": env({ name: "hub-int", envLabel: "int", role: "hub" }),
        "spoke-int": env({
          name: "spoke-int",
          envLabel: "int",
          role: "spoke",
        }),
        "hub-prod": env({ name: "hub-prod", envLabel: "prod", role: "hub" }),
      }),
      emptyFleet,
    );
    expect(model.columns.map((c) => c.key)).toEqual(["int", "prod"]);
    expect(model.columns[0]?.nodeIds).toEqual(["hub-int", "spoke-int"]);
    expect(model.columns[1]?.nodeIds).toEqual(["hub-prod"]);
  });

  it("falls back to the instance key when envLabel is absent", () => {
    const model = buildMapModel(
      promotion(["solo"], { solo: env({ name: "solo" }) }),
      emptyFleet,
    );
    expect(model.columns.map((c) => c.key)).toEqual(["solo"]);
  });
});

describe("buildMapModel - lanes", () => {
  it("lays one lane per provider, sinking hub-hosting clouds to the bottom", () => {
    const model = buildMapModel(
      promotion(["a", "h", "b"], {
        a: env({ name: "a", role: "spoke", provider: "aws" }),
        h: env({ name: "h", role: "hub", provider: "ibm" }),
        b: env({ name: "b", role: "spoke", provider: "gcp" }),
      }),
      emptyFleet,
    );
    // spoke-only clouds first (alpha), the hub-hosting cloud last
    expect(model.lanes.map((l) => l.key)).toEqual(["aws", "gcp", "ibm"]);
    expect(model.lanes.map((l) => l.hostsHub)).toEqual([false, false, true]);
    expect(model.lanes.map((l) => l.provider)).toEqual(["aws", "gcp", "ibm"]);
  });

  it("groups instances without a provider under a single lane key", () => {
    const model = buildMapModel(
      promotion(["a", "b"], {
        a: env({ name: "a", role: "spoke" }),
        b: env({ name: "b", role: "spoke" }),
      }),
      emptyFleet,
    );
    expect(model.lanes).toHaveLength(1);
    expect(model.lanes[0]?.key).toBe("none");
    expect(model.lanes[0]?.provider).toBeNull();
  });
});

describe("buildMapModel - nodes", () => {
  it("merges fleet gateway + metric data by instance name", () => {
    const model = buildMapModel(
      promotion(["x"], {
        x: env({
          name: "x",
          role: "hub",
          activeDigest: "sha256:abc",
          activeRelease: "v1",
          provider: "ibm",
          cluster: "c1",
          consoleUrl: "https://console",
          argoUrl: "https://argo",
        }),
      }),
      {
        instances: [
          inst({
            instance: "x",
            gateways: { running: 5, failed: 1 },
            gatewaysTotal: 6,
            gatewayHistory: [1, 2, 6],
            users: 10,
            rpc: { rate: 3, errorPct: 0.1, p95Ms: 42 },
          }),
        ],
      },
    );
    const node = model.nodes[0];
    expect(node?.seed).toBe("sha256:abc");
    expect(node?.gatewaysTotal).toBe(6);
    expect(node?.gatewayTone).toBe("danger");
    expect(node?.gatewayHistory).toEqual([1, 2, 6]);
    expect(node?.users).toBe(10);
    expect(node?.metrics.rpc.p95Ms).toBe(42);
    expect(node?.links.console).toBe("https://console");
  });

  it("seeds the identicon from the instance key when no digest exists", () => {
    const model = buildMapModel(
      promotion(["y"], { y: env({ name: "y" }) }),
      emptyFleet,
    );
    expect(model.nodes[0]?.seed).toBe("y");
    expect(model.nodes[0]?.metrics.rpc).toEqual(ZERO_RATE);
    expect(model.nodes[0]?.gatewayTone).toBe("unknown");
  });
});

describe("buildMapModel - gates", () => {
  it("bridges adjacent columns using the destination column's hub gate badge", () => {
    const model = buildMapModel(
      promotion(["hi", "hp"], {
        hi: env({ name: "hi", envLabel: "int", role: "hub" }),
        hp: env({
          name: "hp",
          envLabel: "prod",
          role: "hub",
          gates: [{ name: "g", phase: "failed", governingInstance: null }],
        }),
      }),
      emptyFleet,
    );
    expect(model.gates).toHaveLength(1);
    expect(model.gates[0]?.fromColumnKey).toBe("int");
    expect(model.gates[0]?.toColumnKey).toBe("prod");
    expect(model.gates[0]?.badge.tone).toBe("danger");
  });

  it("marks a gate promoting when the destination has an open PR", () => {
    const model = buildMapModel(
      promotion(["a", "b"], {
        a: env({ name: "a", envLabel: "a", role: "hub" }),
        b: env({ name: "b", envLabel: "b", role: "hub", prState: "open" }),
      }),
      emptyFleet,
    );
    expect(model.gates[0]?.promoting).toBe(true);
  });

  it("uses the first node when a column has no hub", () => {
    const model = buildMapModel(
      promotion(["a", "b"], {
        a: env({ name: "a", envLabel: "a", role: "spoke" }),
        b: env({ name: "b", envLabel: "b", role: "spoke" }),
      }),
      emptyFleet,
    );
    expect(model.gates).toHaveLength(1);
    expect(model.gates[0]?.badge.tone).toBe("unknown");
  });

  it("produces no gates for a single column", () => {
    const model = buildMapModel(
      promotion(["a"], { a: env({ name: "a" }) }),
      emptyFleet,
    );
    expect(model.gates).toHaveLength(0);
  });
});

describe("buildMapModel - frontier", () => {
  it("passes the server frontier bundle through", () => {
    const frontier = {
      version: "v9",
      digest: "sha256:f",
      tag: null,
      date: null,
      prs: [],
    };
    const model = buildMapModel(
      promotion(["a"], { a: env({ name: "a" }) }, { frontier }),
      emptyFleet,
    );
    expect(model.frontier).toEqual(frontier);
  });
});
