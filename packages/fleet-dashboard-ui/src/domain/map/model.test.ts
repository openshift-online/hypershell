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
    grafanaUrl: null,
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
    sandboxes: 0,
    sandboxesByCluster: [],
    sandboxHistory: [],
    logins: null,
    userHistory: [],
    loginsHistory: [],
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
  it("makes one column per promotion env (env.name) in server order", () => {
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
    // Each env is its own column (left -> right), so a cloud's sequential stages
    // read across rather than stacking into one env-label column.
    expect(model.columns.map((c) => c.key)).toEqual([
      "hub-int",
      "spoke-int",
      "hub-prod",
    ]);
    expect(model.columns[0]?.nodeIds).toEqual(["hub-int"]);
    expect(model.columns[1]?.nodeIds).toEqual(["spoke-int"]);
    expect(model.columns[2]?.nodeIds).toEqual(["hub-prod"]);
  });

  it("carries the env-type label on each column for the header bands", () => {
    const model = buildMapModel(
      promotion(["hub-int", "hub-prod"], {
        "hub-int": env({ name: "hub-int", envLabel: "int" }),
        "hub-prod": env({ name: "hub-prod", envLabel: "prod" }),
      }),
      emptyFleet,
    );
    expect(model.columns.map((c) => c.envLabel)).toEqual(["int", "prod"]);
  });

  it("stacks a managed-cluster spoke into its hub's column (name prefix)", () => {
    const model = buildMapModel(
      promotion(["env0mc", "env0", "env1"], {
        env0mc: env({
          name: "env0mc",
          role: "spoke",
          provider: "aws",
          envLabel: "int",
        }),
        env0: env({
          name: "env0",
          role: "hub",
          provider: "ibm",
          envLabel: "int",
        }),
        env1: env({
          name: "env1",
          role: "hub",
          provider: "ibm",
          envLabel: "int",
        }),
      }),
      emptyFleet,
    );
    // Hubs are the stages; the spoke (its name extends its hub's) joins the hub's
    // column rather than forming its own.
    expect(model.columns.map((c) => c.key)).toEqual(["env0", "env1"]);
    expect(model.columns[0]?.nodeIds).toEqual(["env0mc", "env0"]);
    // A hub and its managed-cluster spoke share one stage (no gate between them).
    // Each stage gets its own source gate to its right: env0's (-> env1) and env1's
    // terminal gate past the last stage.
    expect(model.gates).toHaveLength(2);
    expect(model.gates[0]?.fromColumnKey).toBe("env0");
    expect(model.gates[0]?.toColumnKey).toBe("env1");
    expect(model.gates[0]?.terminal).toBe(false);
    expect(model.gates[1]?.fromColumnKey).toBe("env1");
    expect(model.gates[1]?.terminal).toBe(true);
  });

  it("leaves envLabel null on a column when the server omits it", () => {
    const model = buildMapModel(
      promotion(["solo"], { solo: env({ name: "solo" }) }),
      emptyFleet,
    );
    expect(model.columns.map((c) => c.key)).toEqual(["solo"]);
    expect(model.columns[0]?.envLabel).toBeNull();
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
          grafanaUrl: "https://grafana",
          argoUrl: "https://argo",
        }),
      }),
      {
        instances: [
          inst({
            instance: "x",
            gateways: { running: 5, failed: 1 },
            gatewaysTotal: 6,
            gatewayHistory: [
              { running: 1, provisioning: 0, failed: 0 },
              { running: 2, provisioning: 0, failed: 0 },
              { running: 5, provisioning: 0, failed: 1 },
            ],
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
    expect(node?.gatewayHistory).toEqual([
      { running: 1, provisioning: 0, failed: 0 },
      { running: 2, provisioning: 0, failed: 0 },
      { running: 5, provisioning: 0, failed: 1 },
    ]);
    expect(node?.users).toBe(10);
    expect(node?.metrics.rpc.p95Ms).toBe(42);
    expect(node?.links.console).toBe("https://console");
    expect(node?.links.grafana).toBe("https://grafana");
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
  it("reports each SOURCE column's own gate badge on the gate to its right", () => {
    const model = buildMapModel(
      promotion(["hi", "hp"], {
        hi: env({
          name: "hi",
          envLabel: "int",
          role: "hub",
          gates: [{ name: "g", phase: "failed", governingInstance: null }],
          analysisUrl: "https://ci/hi",
          argoUrl: "https://argo/hi",
        }),
        hp: env({ name: "hp", envLabel: "prod", role: "hub" }),
      }),
      emptyFleet,
    );
    // One gate per column: hi's (-> hp) carries hi's failed badge + analysis link;
    // hp's is the terminal gate past the last stage.
    expect(model.gates).toHaveLength(2);
    expect(model.gates[0]?.fromColumnKey).toBe("hi");
    expect(model.gates[0]?.toColumnKey).toBe("hp");
    expect(model.gates[0]?.terminal).toBe(false);
    expect(model.gates[0]?.badge.tone).toBe("danger");
    expect(model.gates[0]?.analysisUrl).toBe("https://ci/hi");
    expect(model.gates[0]?.argoUrl).toBe("https://argo/hi");
    expect(model.gates[1]?.fromColumnKey).toBe("hp");
    expect(model.gates[1]?.toColumnKey).toBe("");
    expect(model.gates[1]?.terminal).toBe(true);
  });

  it("surfaces EVERY source gate as its own check, not just the worst-case", () => {
    const model = buildMapModel(
      promotion(["hi", "hp"], {
        hi: env({
          name: "hi",
          envLabel: "int",
          role: "hub",
          gates: [
            { name: "argocd-health", phase: "passed", governingInstance: null },
            {
              name: "hypershell-analysis",
              phase: "pending",
              governingInstance: null,
            },
          ],
        }),
        hp: env({ name: "hp", envLabel: "prod", role: "hub" }),
      }),
      emptyFleet,
    );
    // Both gates appear, each with its own tone - no gate name is hard-coded, and
    // the display no longer collapses to the single worst-case badge.
    expect(model.gates[0]?.checks).toEqual([
      { name: "argocd-health", badge: { tone: "success", labelKey: "passed" } },
      {
        name: "hypershell-analysis",
        badge: { tone: "info", labelKey: "pending" },
      },
    ]);
    // The summary badge still reflects the worst in-flight gate (info beats success).
    expect(model.gates[0]?.badge.tone).toBe("info");
  });

  it("drops empty-named gates from the per-gate checks", () => {
    const model = buildMapModel(
      promotion(["hi", "hp"], {
        hi: env({
          name: "hi",
          envLabel: "int",
          role: "hub",
          gates: [
            { name: "", phase: "passed", governingInstance: null },
            { name: "analysis", phase: "passed", governingInstance: null },
          ],
        }),
        hp: env({ name: "hp", envLabel: "prod", role: "hub" }),
      }),
      emptyFleet,
    );
    expect(model.gates[0]?.checks).toEqual([
      { name: "analysis", badge: { tone: "success", labelKey: "passed" } },
    ]);
  });

  it("marks a gate promoting when its DESTINATION has an open PR", () => {
    const model = buildMapModel(
      promotion(["a", "b"], {
        a: env({ name: "a", envLabel: "a", role: "hub" }),
        b: env({ name: "b", envLabel: "b", role: "hub", prState: "open" }),
      }),
      emptyFleet,
    );
    // a's gate feeds b (which is receiving) -> promoting; the terminal gate never is.
    expect(model.gates[0]?.promoting).toBe(true);
    expect(model.gates[1]?.terminal).toBe(true);
    expect(model.gates[1]?.promoting).toBe(false);
  });

  it("carries the destination's proposed bundle seed + version on a promoting gate", () => {
    const model = buildMapModel(
      promotion(["a", "b"], {
        a: env({ name: "a", envLabel: "a", role: "hub" }),
        b: env({
          name: "b",
          envLabel: "b",
          role: "hub",
          prState: "open",
          proposedDigest: "sha256:next",
          proposedRelease: "v2",
        }),
      }),
      emptyFleet,
    );
    expect(model.gates[0]?.promotingSeed).toBe("sha256:next");
    expect(model.gates[0]?.promotingVersion).toBe("v2");
  });

  it("leaves the promoting bundle null when nothing is in flight", () => {
    const model = buildMapModel(
      promotion(["a", "b"], {
        a: env({ name: "a", envLabel: "a", role: "hub" }),
        b: env({ name: "b", envLabel: "b", role: "hub", upToDate: true }),
      }),
      emptyFleet,
    );
    expect(model.gates[0]?.promoting).toBe(false);
    expect(model.gates[0]?.promotingSeed).toBeNull();
    expect(model.gates[0]?.promotingVersion).toBeNull();
  });

  it("uses the first node's gate when a source column has no hub", () => {
    const model = buildMapModel(
      promotion(["a", "b"], {
        a: env({ name: "a", envLabel: "a", role: "spoke" }),
        b: env({ name: "b", envLabel: "b", role: "spoke" }),
      }),
      emptyFleet,
    );
    // a's gate (no gates -> unknown) + b's terminal gate.
    expect(model.gates).toHaveLength(2);
    expect(model.gates[0]?.badge.tone).toBe("unknown");
  });

  it("produces a single terminal gate for a single column", () => {
    const model = buildMapModel(
      promotion(["a"], { a: env({ name: "a" }) }),
      emptyFleet,
    );
    expect(model.gates).toHaveLength(1);
    expect(model.gates[0]?.terminal).toBe(true);
    expect(model.gates[0]?.toColumnKey).toBe("");
  });
});

describe("buildMapModel - version drift", () => {
  // A spoke stacks into its hub's column (name extends the hub's), so the two share
  // an environment and their active digests are directly comparable.
  function intColumn(hubDigest: string, spokeDigest: string): PromotionData {
    return promotion(["int"], {
      int: env({
        name: "int",
        role: "hub",
        envLabel: "int",
        activeDigest: hubDigest,
      }),
      "int-edge": env({
        name: "int-edge",
        role: "spoke",
        envLabel: "int",
        activeDigest: spokeDigest,
      }),
    });
  }

  it("flags a spoke running a different digest than its hub", () => {
    const model = buildMapModel(intColumn("sha-A", "sha-B"), emptyFleet);
    const hub = model.nodes.find((n) => n.id === "int");
    const spoke = model.nodes.find((n) => n.id === "int-edge");
    expect(hub?.driftsFromColumn).toBe(false);
    expect(spoke?.driftsFromColumn).toBe(true);
  });

  it("does not flag a spoke that matches its hub's digest", () => {
    const model = buildMapModel(intColumn("sha-A", "sha-A"), emptyFleet);
    expect(model.nodes.find((n) => n.id === "int-edge")?.driftsFromColumn).toBe(
      false,
    );
  });

  it("never asserts drift when a digest is missing (unknowable)", () => {
    const model = buildMapModel(intColumn("sha-A", ""), emptyFleet);
    expect(model.nodes.find((n) => n.id === "int-edge")?.driftsFromColumn).toBe(
      false,
    );
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
