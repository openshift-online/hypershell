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

  it("rolls up the hub's own + co-located clusters, excluding the broken-out spoke", () => {
    const { promotion: p, fleet } = hubAndSpoke();
    const hub = nodeById(buildMapModel(p, fleet, topo()), "hub0");
    // 3 (hub-own) + 2 (co-located) = 5; the remote spoke's 1 is NOT on this card.
    expect(hub.gatewaysTotal).toBe(5);
    expect(hub.gateways).toEqual({ running: 5 });
    expect(hub.sandboxes).toBe(2);
    expect(hub.gatewaysByCluster.map((r) => r.managedCluster)).toEqual([
      "hub0-hub",
      "hub0-co",
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

  it("rolls up the hub's chin history across its clusters, excluding the spoke", () => {
    const { promotion: p, fleet } = hubAndSpoke();
    const hub = nodeById(buildMapModel(p, fleet, topo()), "hub0");
    // hub-own(3,3) + co-located(2,2) at each sample; the spoke's 1 is excluded.
    expect(hub.gatewayHistory).toEqual([
      { running: 5, provisioning: 0, failed: 0 },
      { running: 5, provisioning: 0, failed: 0 },
    ]);
    expect(hub.sandboxHistory).toEqual([2, 2]);
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

  it("rolls a co-located spoke with its own env into the hub, not its own card", () => {
    // hub0-co has a promotion env here, but topology lists it co-located (not a remote
    // spoke), so it must still roll into the hub and NOT be broken onto its own card.
    const { fleet } = hubAndSpoke();
    const p = promotion(["hub0", "hub0-co", "hub0-spoke"], {
      hub0: env({ name: "hub0", role: "hub", provider: "ibm" }),
      "hub0-co": env({ name: "hub0-co", role: "spoke", provider: "ibm" }),
      "hub0-spoke": env({ name: "hub0-spoke", role: "spoke", provider: "aws" }),
    });
    const model = buildMapModel(p, fleet, topo());
    const hub = nodeById(model, "hub0");
    const co = nodeById(model, "hub0-co");
    // Hub rolls up its own(3) + co-located(2) = 5; only the remote spoke's 1 is split out.
    expect(hub.gatewaysTotal).toBe(5);
    expect(hub.gatewaysByCluster.map((r) => r.managedCluster)).toEqual([
      "hub0-hub",
      "hub0-co",
    ]);
    // The co-located spoke's own node shows nothing (its counts are on the hub).
    expect(co.gatewaysTotal).toBe(0);
    expect(co.sandboxes).toBe(0);
  });
});
