import { describe, expect, it } from "vitest";

import { ZERO_RATE, type FleetData, type InstanceFleet } from "../fleet";
import type { PromotionData, PromotionEnvironment } from "../promotion";
import type { TopologyData } from "../topology";
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

function inst(
  overrides: Partial<InstanceFleet> & { instance: string },
): InstanceFleet {
  return {
    role: null,
    provider: null,
    gateways: {},
    gatewaysTotal: 0,
    gatewaysByCluster: [],
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
    historyByCluster: [],
    logins: null,
    userHistory: [],
    loginsHistory: [],
    historyTimes: [],
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
  it("lays one lane per provider, stable-alphabetical, not sinking hub clouds", () => {
    const model = buildMapModel(
      promotion(["a", "h", "b"], {
        a: env({ name: "a", role: "spoke", provider: "aws" }),
        h: env({ name: "h", role: "hub", provider: "ibm" }),
        b: env({ name: "b", role: "spoke", provider: "gcp" }),
      }),
      emptyFleet,
    );
    // Plain alphabetical by key; the hub-hosting flag is labeling metadata and no
    // longer reorders lanes (placement is role-driven in the layout).
    expect(model.lanes.map((l) => l.key)).toEqual(["aws", "gcp", "ibm"]);
    expect(model.lanes.map((l) => l.hostsHub)).toEqual([false, false, true]);
    expect(model.lanes.map((l) => l.provider)).toEqual(["aws", "gcp", "ibm"]);
  });

  it("does not sink a hub-hosting provider that sorts first alphabetically", () => {
    const model = buildMapModel(
      promotion(["a", "h", "b"], {
        a: env({ name: "a", role: "spoke", provider: "mmm" }),
        h: env({ name: "h", role: "hub", provider: "aaa" }),
        b: env({ name: "b", role: "spoke", provider: "zzz" }),
      }),
      emptyFleet,
    );
    // The hub cloud "aaa" stays first (alpha), proving hub-hosting no longer sinks.
    expect(model.lanes.map((l) => l.key)).toEqual(["aaa", "mmm", "zzz"]);
    expect(model.lanes.map((l) => l.hostsHub)).toEqual([true, false, false]);
  });

  // Verifies: FLT-MAP-07.02
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
  // Verifies: FLT-SYNC-03.01, FLT-SYNC-03.02
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
          syncRetries: 9,
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
    // The gate carries its SOURCE env's sync-retry count.
    expect(model.gates[0]?.syncRetries).toBe(9);
    expect(model.gates[1]?.fromColumnKey).toBe("hp");
    expect(model.gates[1]?.toColumnKey).toBe("");
    expect(model.gates[1]?.terminal).toBe(true);
    // The terminal gate's source env had no sync data: null stays null.
    expect(model.gates[1]?.syncRetries).toBeNull();
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

describe("buildMapModel - per-cluster attribution", () => {
  // Topology marking hub0-spoke a REMOTE spoke of hub0 and hub0-co a co-located one.
  // Breakout is driven off this (the managed_cluster identity), not the env name, so the
  // rollup/slice attribute on the same key classifySpokes does.
  function topo(): TopologyData {
    return {
      hub0: {
        instance: "hub0",
        hub: { instance: "hub0", dnsLabel: null, remoteSpokes: ["hub0-spoke"] },
        spokes: [{ name: "hub0-co" }],
      },
    };
  }

  // One hub ("hub0") with a remote spoke ("hub0-spoke") that stacks into its column by
  // name prefix. The hub instance reports three managed-cluster rows: its own controller
  // ("hub0-hub"), a co-located spoke ("hub0-co") and the remote spoke ("hub0-spoke").
  // The remote spoke gets its own node card; the hub card rolls up the other two.
  function hubAndSpoke(): { promotion: PromotionData; fleet: FleetData } {
    return {
      promotion: promotion(["hub0", "hub0-spoke"], {
        hub0: env({ name: "hub0", role: "hub", provider: "ibm" }),
        "hub0-spoke": env({
          name: "hub0-spoke",
          role: "spoke",
          provider: "aws",
        }),
      }),
      fleet: {
        instances: [
          inst({
            instance: "hub0",
            gateways: { running: 6 },
            gatewaysTotal: 6,
            gatewaysByCluster: [
              {
                managedCluster: "hub0-hub",
                gateways: { running: 3 },
                total: 3,
              },
              { managedCluster: "hub0-co", gateways: { running: 2 }, total: 2 },
              {
                managedCluster: "hub0-spoke",
                gateways: { running: 1 },
                total: 1,
              },
            ],
            sandboxes: 3,
            sandboxesByCluster: [
              { managedCluster: "hub0-hub", count: 1 },
              { managedCluster: "hub0-co", count: 1 },
              { managedCluster: "hub0-spoke", count: 1 },
            ],
            gatewayHistory: [{ running: 6, provisioning: 0, failed: 0 }],
            sandboxHistory: [3],
            historyTimes: [100, 200],
            historyByCluster: [
              {
                managedCluster: "hub0-hub",
                gatewayHistory: [
                  { running: 3, provisioning: 0, failed: 0 },
                  { running: 3, provisioning: 0, failed: 0 },
                ],
                sandboxHistory: [1, 1],
              },
              {
                managedCluster: "hub0-co",
                gatewayHistory: [
                  { running: 2, provisioning: 0, failed: 0 },
                  { running: 2, provisioning: 0, failed: 0 },
                ],
                sandboxHistory: [1, 1],
              },
              {
                managedCluster: "hub0-spoke",
                gatewayHistory: [
                  { running: 1, provisioning: 0, failed: 0 },
                  { running: 1, provisioning: 0, failed: 0 },
                ],
                sandboxHistory: [0, 1],
              },
            ],
            rpc: { rate: 5, errorPct: 0, p95Ms: 0 },
            users: 42,
          }),
        ],
      },
    };
  }

  function nodeById(model: ReturnType<typeof buildMapModel>, id: string) {
    const n = model.nodes.find((x) => x.id === id);
    if (!n) {
      throw new Error(`no node ${id}`);
    }
    return n;
  }

  it("keeps only the hub's own cluster rows, excluding remote and co-located spokes", () => {
    const { promotion: p, fleet } = hubAndSpoke();
    const hub = nodeById(buildMapModel(p, fleet, topo()), "hub0");
    // Only hub-own (3); the co-located (2) and remote (1) spokes have their own cards.
    expect(hub.gatewaysTotal).toBe(3);
    expect(hub.gateways).toEqual({ running: 3 });
    expect(hub.sandboxes).toBe(1);
    expect(hub.gatewaysByCluster.map((r) => r.managedCluster)).toEqual([
      "hub0-hub",
    ]);
  });

  it("gives the remote spoke its own counts from the hub's per-cluster row", () => {
    const { promotion: p, fleet } = hubAndSpoke();
    const spoke = nodeById(buildMapModel(p, fleet, topo()), "hub0-spoke");
    expect(spoke.gatewaysTotal).toBe(1);
    expect(spoke.gateways).toEqual({ running: 1 });
    expect(spoke.sandboxes).toBe(1);
    // A single slice has nothing further to split, so no detail-panel attribution.
    expect(spoke.spokeAttribution).toBeNull();
  });

  it("keeps the hub's own chin history, excluding every broken-out spoke", () => {
    const { promotion: p, fleet } = hubAndSpoke();
    const hub = nodeById(buildMapModel(p, fleet, topo()), "hub0");
    // Only hub-own (3) at each sample; co-located and remote spokes are excluded.
    expect(hub.gatewayHistory).toEqual([
      { running: 3, provisioning: 0, failed: 0 },
      { running: 3, provisioning: 0, failed: 0 },
    ]);
    expect(hub.sandboxHistory).toEqual([1, 1]);
    expect(hub.historyTimes).toEqual([100, 200]);
  });

  it("gives the remote spoke its own chin history, zero-filling missing samples", () => {
    const { promotion: p, fleet } = hubAndSpoke();
    const spoke = nodeById(buildMapModel(p, fleet, topo()), "hub0-spoke");
    expect(spoke.gatewayHistory).toEqual([
      { running: 1, provisioning: 0, failed: 0 },
      { running: 1, provisioning: 0, failed: 0 },
    ]);
    // The spoke's ts-100 sandbox sample is 0, ts-200 is 1 (from historyByCluster).
    expect(spoke.sandboxHistory).toEqual([0, 1]);
    expect(spoke.historyTimes).toEqual([100, 200]);
  });

  it("does not borrow the hub's control-plane metrics onto the spoke card", () => {
    const { promotion: p, fleet } = hubAndSpoke();
    const spoke = nodeById(buildMapModel(p, fleet, topo()), "hub0-spoke");
    // RED metrics, users and friends are hub-level, so the spoke card leaves them empty.
    expect(spoke.metrics.rpc).toEqual(ZERO_RATE);
    expect(spoke.users).toBeNull();
  });

  it("keeps the unattributed (unknown) bucket on the hub card", () => {
    const fleet: FleetData = {
      instances: [
        inst({
          instance: "hub0",
          gatewaysByCluster: [
            { managedCluster: "hub0-hub", gateways: { running: 3 }, total: 3 },
            { managedCluster: "unknown", gateways: { failed: 4 }, total: 4 },
            {
              managedCluster: "hub0-spoke",
              gateways: { running: 1 },
              total: 1,
            },
          ],
          sandboxesByCluster: [
            { managedCluster: "unknown", count: 2 },
            { managedCluster: "hub0-spoke", count: 5 },
          ],
        }),
      ],
    };
    const p = promotion(["hub0", "hub0-spoke"], {
      hub0: env({ name: "hub0", role: "hub" }),
      "hub0-spoke": env({ name: "hub0-spoke", role: "spoke" }),
    });
    const hub = nodeById(buildMapModel(p, fleet, topo()), "hub0");
    // hub-own(3) + unknown(4) stay on the hub; only the spoke's 1 is split out.
    expect(hub.gatewaysTotal).toBe(7);
    expect(hub.gateways).toEqual({ running: 3, failed: 4 });
    expect(hub.sandboxes).toBe(2);
    expect(hub.gatewaysByCluster.map((r) => r.managedCluster)).toEqual([
      "hub0-hub",
      "unknown",
    ]);
  });

  it("leaves a lone hub's totals verbatim when no spoke is broken out", () => {
    const fleet: FleetData = {
      instances: [
        inst({
          instance: "solo",
          gateways: { running: 9 },
          gatewaysTotal: 9,
          gatewaysByCluster: [
            { managedCluster: "solo-hub", gateways: { running: 9 }, total: 9 },
          ],
          sandboxes: 4,
          gatewayHistory: [{ running: 9, provisioning: 0, failed: 0 }],
          sandboxHistory: [4],
          historyTimes: [100],
        }),
      ],
    };
    const p = promotion(["solo"], { solo: env({ name: "solo", role: "hub" }) });
    const hub = nodeById(buildMapModel(p, fleet), "solo");
    // No sibling spoke node, so the card is the instance total unchanged.
    expect(hub.gatewaysTotal).toBe(9);
    expect(hub.sandboxes).toBe(4);
    expect(hub.gatewayHistory).toEqual([
      { running: 9, provisioning: 0, failed: 0 },
    ]);
    expect(hub.sandboxHistory).toEqual([4]);
  });

  it("rolls a spoke into the hub when topology does not class it remote", () => {
    // Same fleet, but no topology: breakout is topology-driven, so with nothing marked
    // remote the spoke's counts stay rolled into the hub and its own card shows nothing.
    // This guards the Major finding: a spoke env is split out ONLY on the managed_cluster
    // identity classifySpokes uses, never on a bare env-name match that could diverge.
    const { promotion: p, fleet } = hubAndSpoke();
    const model = buildMapModel(p, fleet);
    const hub = nodeById(model, "hub0");
    const spoke = nodeById(model, "hub0-spoke");
    // Hub keeps the full instance total (all three clusters), unchanged from no-attribution.
    expect(hub.gatewaysTotal).toBe(6);
    expect(hub.gateways).toEqual({ running: 6 });
    expect(hub.sandboxes).toBe(3);
    // The spoke card borrows nothing - its counts live on the hub.
    expect(spoke.gatewaysTotal).toBe(0);
    expect(spoke.sandboxes).toBe(0);
  });

  it("gives a co-located spoke with its own env its own counts", () => {
    // hub0-co has a promotion env here and topology lists it co-located: it keeps its
    // env and takes its own slice, so the hub keeps only its own controller's rows.
    const { fleet } = hubAndSpoke();
    const p = promotion(["hub0", "hub0-co", "hub0-spoke"], {
      hub0: env({ name: "hub0", role: "hub", provider: "ibm" }),
      "hub0-co": env({ name: "hub0-co", role: "spoke", provider: "ibm" }),
      "hub0-spoke": env({ name: "hub0-spoke", role: "spoke", provider: "aws" }),
    });
    const model = buildMapModel(p, fleet, topo());
    const hub = nodeById(model, "hub0");
    const co = nodeById(model, "hub0-co");
    expect(hub.gatewaysTotal).toBe(3);
    expect(co.gatewaysTotal).toBe(2);
    expect(co.sandboxes).toBe(1);
    expect(co.fromTopology).toBe(false);
    expect(co.colocatedWith).toBe("hub0");
    // No duplicate node is created for a spoke that already has an env.
    expect(model.nodes.filter((n) => n.id === "hub0-co")).toHaveLength(1);
  });
});

describe("buildMapModel - co-located spokes and physical clusters", () => {
  function topo(): TopologyData {
    return {
      hub0: {
        instance: "hub0",
        hub: { instance: "hub0", dnsLabel: null, remoteSpokes: ["hub0-spoke"] },
        spokes: [{ name: "hub0-co" }],
      },
    };
  }

  function fleet(): FleetData {
    return {
      instances: [
        inst({
          instance: "hub0",
          gateways: { running: 6 },
          gatewaysTotal: 6,
          gatewaysByCluster: [
            { managedCluster: "hub0-hub", gateways: { running: 3 }, total: 3 },
            { managedCluster: "hub0-co", gateways: { running: 2 }, total: 2 },
            {
              managedCluster: "hub0-spoke",
              gateways: { running: 1 },
              total: 1,
            },
          ],
          sandboxesByCluster: [
            { managedCluster: "hub0-hub", count: 1 },
            { managedCluster: "hub0-co", count: 4 },
          ],
        }),
      ],
    };
  }

  // The live shape: the co-located spoke has NO promotion env of its own.
  function promo(): PromotionData {
    return promotion(["hub0", "hub0-spoke"], {
      hub0: env({
        name: "hub0",
        role: "hub",
        cluster: "phys-a",
        envLabel: "int",
        activeRelease: "v1",
        activeDigest: "sha256:aaa",
        argoHealth: "Healthy",
        capabilities: ["provider/ibm"],
        gates: [
          { name: "analysis", phase: "Succeeded", governingInstance: null },
        ],
        consoleUrl: "https://console.example",
      }),
      "hub0-spoke": env({
        name: "hub0-spoke",
        role: "spoke",
        provider: "ibm",
        cluster: "phys-b",
      }),
    });
  }

  function nodeById(model: ReturnType<typeof buildMapModel>, id: string) {
    const n = model.nodes.find((x) => x.id === id);
    if (!n) {
      throw new Error(`no node ${id}`);
    }
    return n;
  }

  // Verifies: FLT-MAP-08.01
  it("creates a node for a co-located spoke that has no promotion env", () => {
    const model = buildMapModel(promo(), fleet(), topo());
    const co = nodeById(model, "hub0-co");
    expect(co.fromTopology).toBe(true);
    expect(co.colocatedWith).toBe("hub0");
    expect(co.isHub).toBe(false);
    expect(co.role).toBe("spoke");
    expect(co.columnKey).toBe("hub0");
    // It lives on the hub's physical cluster and provider lane.
    expect(co.cluster).toBe("phys-a");
    expect(co.laneKey).toBe("ibm");
    // Its own slice of the hub's per-cluster rows.
    expect(co.gatewaysTotal).toBe(2);
    expect(co.sandboxes).toBe(4);
    // The hub no longer counts it.
    expect(nodeById(model, "hub0").gatewaysTotal).toBe(3);
    // Placed right after its hub in the column.
    expect(model.columns[0]?.nodeIds).toEqual([
      "hub0",
      "hub0-co",
      "hub0-spoke",
    ]);
  });

  it("mirrors the hub's release and Argo state but carries no gates or console", () => {
    const co = nodeById(buildMapModel(promo(), fleet(), topo()), "hub0-co");
    expect(co.version).toBe("v1");
    expect(co.digest).toBe("sha256:aaa");
    expect(co.argoHealth).toBe("Healthy");
    expect(co.driftsFromColumn).toBe(false);
    expect(co.gateNames).toEqual([]);
    expect(co.capabilities).toEqual([]);
    expect(co.links.console).toBeNull();
    // Hub-level control-plane metrics stay on the hub.
    expect(co.users).toBeNull();
  });

  it("keeps the gate on the hub's analysis, not the co-located spoke's", () => {
    const model = buildMapModel(promo(), fleet(), topo());
    expect(model.gates[0]?.name).toBe("analysis");
  });

  // Verifies: FLT-MAP-08.01
  it("groups the hub and its co-located spoke into one physical cluster", () => {
    const model = buildMapModel(promo(), fleet(), topo());
    expect(model.clusterGroups).toEqual([
      {
        id: "hub0::phys-a",
        cluster: "phys-a",
        columnKey: "hub0",
        laneKey: "ibm",
        nodeIds: ["hub0", "hub0-co"],
      },
    ]);
  });

  it("groups a hub with no cluster label and its co-located spoke by the hub", () => {
    const p = promo();
    const hub0 = p.environments.hub0;
    if (!hub0) {
      throw new Error("no hub0");
    }
    const unlabeled = promotion(["hub0"], {
      hub0: { ...hub0, cluster: null },
    });
    const model = buildMapModel(unlabeled, fleet(), topo());
    expect(model.clusterGroups).toHaveLength(1);
    expect(model.clusterGroups[0]?.cluster).toBeNull();
    expect(model.clusterGroups[0]?.nodeIds).toEqual(["hub0", "hub0-co"]);
  });

  // Verifies: FLT-MAP-08.04
  it("creates no container when every node runs on its own cluster", () => {
    const model = buildMapModel(promo(), fleet(), {
      hub0: {
        instance: "hub0",
        hub: { instance: "hub0", dnsLabel: null, remoteSpokes: ["hub0-spoke"] },
        spokes: [],
      },
    });
    expect(model.clusterGroups).toEqual([]);
    expect(model.nodes.map((n) => n.id)).toEqual(["hub0", "hub0-spoke"]);
  });

  it("creates no co-located node without topology", () => {
    const model = buildMapModel(promo(), fleet());
    expect(model.nodes.map((n) => n.id)).toEqual(["hub0", "hub0-spoke"]);
    expect(nodeById(model, "hub0").gatewaysTotal).toBe(6);
  });
});

describe("buildMapModel capabilities", () => {
  const emptyFleet: FleetData = { instances: [] };

  // FLT-CAP-01.01: capabilities thread from the environment to the map node in
  // order, unchanged.
  it("carries capabilities onto the node in order", () => {
    const promotion: PromotionData = {
      order: ["alpha"],
      environments: {
        alpha: env({
          name: "alpha",
          role: "hub",
          capabilities: ["provider/aws", "runtime/kata", "network/vpn"],
        }),
      },
      releases: [],
      releaseByDigest: {},
      frontier: null,
    };
    const model = buildMapModel(promotion, emptyFleet);
    const node = model.nodes.find((n) => n.id === "alpha");
    expect(node?.capabilities).toEqual([
      "provider/aws",
      "runtime/kata",
      "network/vpn",
    ]);
  });

  // FLT-CAP-01.02: an environment with no capabilities carries an empty set.
  it("defaults missing capabilities to an empty set", () => {
    const promotion: PromotionData = {
      order: ["alpha"],
      environments: { alpha: env({ name: "alpha", role: "hub" }) },
      releases: [],
      releaseByDigest: {},
      frontier: null,
    };
    const model = buildMapModel(promotion, emptyFleet);
    const node = model.nodes.find((n) => n.id === "alpha");
    expect(node?.capabilities).toEqual([]);
  });

  it("derives provider from the provider capability, grouping its lane", () => {
    const promotion: PromotionData = {
      order: ["alpha"],
      environments: {
        alpha: env({
          name: "alpha",
          role: "hub",
          capabilities: ["provider/aws"],
        }),
      },
      releases: [],
      releaseByDigest: {},
      frontier: null,
    };
    const model = buildMapModel(promotion, emptyFleet);
    const node = model.nodes.find((n) => n.id === "alpha");
    expect(node?.provider).toBe("aws");
    expect(node?.laneKey).toBe("aws");
  });
});

describe("buildMapModel - remote spokes without an environment", () => {
  // Verifies: FLT-MAP-08.05
  it("gives a remote spoke with no promotion env its own box", () => {
    const p = promotion(["hub0"], {
      hub0: env({
        name: "hub0",
        role: "hub",
        provider: "ibm",
        cluster: "phys-a",
        envLabel: "int",
        activeRelease: "v1",
        activeDigest: "sha256:aaa",
        argoHealth: "Healthy",
      }),
    });
    const fleet: FleetData = {
      instances: [
        inst({
          instance: "hub0",
          gatewaysByCluster: [
            { managedCluster: "hub0-hub", gateways: { running: 8 }, total: 8 },
            { managedCluster: "hub0-far", gateways: { running: 1 }, total: 1 },
          ],
          sandboxesByCluster: [{ managedCluster: "hub0-far", count: 2 }],
        }),
      ],
    };
    const topo: TopologyData = {
      hub0: {
        instance: "hub0",
        hub: { instance: "hub0", dnsLabel: null, remoteSpokes: ["hub0-far"] },
        spokes: [],
      },
    };
    const model = buildMapModel(p, fleet, topo);
    const far = model.nodes.find((n) => n.id === "hub0-far");
    const hub = model.nodes.find((n) => n.id === "hub0");
    expect(far).toBeDefined();
    expect(far?.columnKey).toBe("hub0");
    expect(far?.role).toBe("spoke");
    expect(far?.gatewaysTotal).toBe(1);
    expect(far?.sandboxes).toBe(2);
    expect(hub?.gatewaysTotal).toBe(8);
    // Nothing reports where or what it runs, so those stay unknown.
    expect(far?.deliveryUnknown).toBe(true);
    expect(far?.fromTopology).toBe(true);
    expect(far?.cluster).toBeNull();
    expect(far?.laneKey).toBe("none");
    expect(far?.version).toBeNull();
    expect(far?.argoHealth).toBeNull();
    expect(far?.envLabel).toBe("int");
    // It is not on the hub's physical cluster, so no outer box is drawn.
    expect(model.clusterGroups).toEqual([]);
    // The hub still governs the gate.
    expect(model.gates).toHaveLength(1);
    expect(hub?.deliveryUnknown).toBe(false);
  });

  it("does not duplicate a remote spoke that already has an env", () => {
    const p = promotion(["hub0", "hub0-far"], {
      hub0: env({ name: "hub0", role: "hub" }),
      "hub0-far": env({ name: "hub0-far", role: "spoke" }),
    });
    const model = buildMapModel(
      p,
      { instances: [] },
      {
        hub0: {
          instance: "hub0",
          hub: { instance: "hub0", dnsLabel: null, remoteSpokes: ["hub0-far"] },
          spokes: [],
        },
      },
    );
    const far = model.nodes.filter((n) => n.id === "hub0-far");
    expect(far).toHaveLength(1);
    expect(far[0]?.deliveryUnknown).toBe(false);
  });
});

describe("buildMapModel - remote spokes Argo CD reports", () => {
  // Verifies: FLT-MAP-09.05
  it("takes the cluster, lane and links from the unpromoted instance", () => {
    const p = {
      ...promotion(["hub0"], {
        hub0: env({
          name: "hub0",
          role: "hub",
          provider: "ibm",
          cluster: "phys-a",
          envLabel: "int",
        }),
      }),
      unpromoted: {
        "hub0-far": {
          cluster: "phys-far",
          role: null,
          // No provider label: the lane comes from the capability.
          provider: null,
          envLabel: null,
          capabilities: ["provider/ibm"],
          argoHealth: "Healthy",
          argoSync: "Synced",
          argoUrl: "https://argo.example/applications/phys-far/hub0-far",
          consoleUrl: null,
          grafanaUrl: null,
        },
      },
    };
    const topo: TopologyData = {
      hub0: {
        instance: "hub0",
        hub: { instance: "hub0", dnsLabel: null, remoteSpokes: ["hub0-far"] },
        spokes: [],
      },
    };
    const model = buildMapModel(p, { instances: [] }, topo);
    const far = model.nodes.find((n) => n.id === "hub0-far");
    expect(far?.cluster).toBe("phys-far");
    expect(far?.physicalClusterKey).toBe("phys-far");
    expect(far?.laneKey).toBe(
      model.nodes.find((n) => n.id === "hub0")?.laneKey,
    );
    expect(far?.argoHealth).toBe("Healthy");
    expect(far?.links.argo).toBe(
      "https://argo.example/applications/phys-far/hub0-far",
    );
    // Still outside promotion: no release, no promotion lines.
    expect(far?.deliveryUnknown).toBe(true);
    expect(far?.version).toBeNull();
    // A different physical cluster from the hub, so no outer box.
    expect(model.clusterGroups).toEqual([]);
  });
});

describe("buildMapModel - promotion lines", () => {
  // Verifies: FLT-MAP-08.06
  it("leaves a remote spoke outside promotion off the column's rails", () => {
    const p = promotion(["hub0", "hub0-r"], {
      hub0: env({ name: "hub0", role: "hub", provider: "ibm" }),
      "hub0-r": env({ name: "hub0-r", role: "spoke", provider: "ibm" }),
    });
    const model = buildMapModel(
      p,
      { instances: [] },
      {
        hub0: {
          instance: "hub0",
          hub: {
            instance: "hub0",
            dnsLabel: null,
            remoteSpokes: ["hub0-r", "hub0-far"],
          },
          spokes: [{ name: "hub0-co" }],
        },
      },
    );
    const col = model.columns[0];
    expect(col?.nodeIds).toEqual(["hub0", "hub0-co", "hub0-far", "hub0-r"]);
    // hub0-far has no promotion env and no delivery data: no rails.
    expect(col?.railNodeIds).toEqual(["hub0", "hub0-co", "hub0-r"]);
  });

  // A co-located spoke that has its own promotion environment: labelled with the
  // hub's cluster, listed before the hub, and gated on argocd-health.
  it("keeps a co-located spoke with its own env in the outer box and on the rails", () => {
    const p = promotion(["hub0-co", "hub0"], {
      "hub0-co": env({
        name: "hub0-co",
        role: "spoke",
        provider: "ibm",
        cluster: "phys-a",
        envLabel: "int",
        gates: [
          {
            name: "argocd-health",
            phase: "Succeeded",
            governingInstance: null,
          },
        ],
      }),
      hub0: env({
        name: "hub0",
        role: "hub",
        provider: "ibm",
        cluster: "phys-a",
        envLabel: "int",
        gates: [
          {
            name: "hypershell-analysis",
            phase: "Succeeded",
            governingInstance: null,
          },
        ],
      }),
    });
    const fleet: FleetData = {
      instances: [
        inst({
          instance: "hub0",
          gatewaysByCluster: [
            { managedCluster: "hub0-hub", gateways: { running: 6 }, total: 6 },
            { managedCluster: "hub0-co", gateways: { running: 4 }, total: 4 },
          ],
        }),
      ],
    };
    const model = buildMapModel(p, fleet, {
      hub0: {
        instance: "hub0",
        hub: { instance: "hub0", dnsLabel: null, remoteSpokes: [] },
        spokes: [{ name: "hub0-co" }],
      },
    });
    const co = model.nodes.find((n) => n.id === "hub0-co");
    expect(co?.fromTopology).toBe(false);
    expect(co?.columnKey).toBe("hub0");
    expect(co?.gatewaysTotal).toBe(4);
    expect(co?.gateNames).toEqual(["argocd-health"]);
    expect(model.nodes.find((n) => n.id === "hub0")?.gatewaysTotal).toBe(6);
    expect(model.clusterGroups.map((g) => [g.cluster, g.nodeIds])).toEqual([
      ["phys-a", ["hub0-co", "hub0"]],
    ]);
    expect(model.columns[0]?.railNodeIds).toEqual(["hub0-co", "hub0"]);
    // The hub, not the spoke listed first, governs the gate.
    expect(model.gates[0]?.name).toBe("hypershell-analysis");
  });
});
