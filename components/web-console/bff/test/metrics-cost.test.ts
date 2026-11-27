import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

import { describe, expect, it } from "vitest";

import {
  selectBillingOwner,
  type CostGateway,
} from "../src/cost-attribution.js";
import {
  createRateCardSource,
  parseRateCard,
  type RateCard,
} from "../src/cost-rate-card.js";
import {
  buildBreakdownResponse,
  buildMyCostGateways,
  buildMyCostSummary,
  buildSummaryResponse,
  computePlatformSnapshot,
  gatewayNamespaceRegex,
  hoursPerMonth,
  namespaceOwnerResolver,
  namespaceQuantityQuery,
  type MyCostGatewayInput,
  type PlatformSnapshotInput,
} from "../src/metrics-cost.js";

const gib = 1024 ** 3;

function rateCard(document: Record<string, unknown>): RateCard {
  return parseRateCard(
    JSON.stringify({ currency: "USD", version: "2026-11-26", ...document }),
  );
}

const cpuOnly = rateCard({
  defaults: { cpu_core_hour: 0.03, memory_gib_hour: 0 },
});

const cluster = { cluster: "c1", environment: "production", provider: "ibm" };

function snapshotInput(
  overrides: Partial<PlatformSnapshotInput> = {},
): PlatformSnapshotInput {
  return {
    clusterCpu: [{ labels: cluster, value: 64 }],
    clusterMemory: [{ labels: cluster, value: 0 }],
    gateways: [],
    namespaceCpu: [],
    namespaceMemory: [],
    nowSeconds: 1_000_000 + 3600,
    rangeHours: 1,
    rangeStartSeconds: 1_000_000,
    requestedCpu: [{ labels: cluster, value: 40 }],
    requestedMemory: [{ labels: cluster, value: 0 }],
    ...overrides,
  };
}

function gateway(
  id: string,
  mode: CostGateway["workspaceMode"] = "managed",
): CostGateway {
  return {
    clusterId: "cluster-1",
    id,
    name: `Gateway ${id}`,
    namespace: `openshell-${id}`,
    workspaceMode: mode,
  };
}

describe("cost rate card", () => {
  // Verifies: WEB-COST-03.02
  it("resolves cluster, then provider, then default rates per resource", () => {
    const card = rateCard({
      defaults: { cpu_core_hour: 0.031, memory_gib_hour: 0.0042 },
      providers: { ibm: { cpu_core_hour: 0.034, memory_gib_hour: 0.0046 } },
      clusters: { c1: { cpu_core_hour: 0.04 } },
    });

    expect(card.resolve("c1", "ibm")).toEqual({
      cpuCoreHour: 0.04,
      memoryGibHour: 0.0046,
    });
    expect(card.resolve("c2", "ibm")).toEqual({
      cpuCoreHour: 0.034,
      memoryGibHour: 0.0046,
    });
    expect(card.resolve("c3", "aws")).toEqual({
      cpuCoreHour: 0.031,
      memoryGibHour: 0.0042,
    });
  });

  // unpriced clusters
  // Verifies: WEB-COST-04
  it("leaves a cluster unpriced when no level has a rate", () => {
    const card = rateCard({
      providers: { aws: { cpu_core_hour: 0.03, memory_gib_hour: 0.004 } },
    });
    expect(card.resolve("c1", "ibm")).toBeUndefined();
  });

  it("rejects invalid rate cards without echoing values", () => {
    expect(() => parseRateCard("not json")).toThrow(/not valid JSON/u);
    expect(() =>
      parseRateCard(JSON.stringify({ currency: "usd", version: "v1" })),
    ).toThrow(/currency/u);
    expect(() =>
      parseRateCard(
        JSON.stringify({
          currency: "USD",
          version: "v1",
          defaults: { cpu_core_hour: -1 },
        }),
      ),
    ).toThrow(/defaults\.cpu_core_hour/u);
  });

  // Verifies: WEB-COST-03.01
  it("reports not-configured without a file and invalid for a bad file", async () => {
    expect(createRateCardSource(undefined).current()).toEqual({
      status: "not-configured",
    });

    const directory = await mkdtemp(path.join(tmpdir(), "rate-card-"));
    try {
      const file = path.join(directory, "rate-card.json");
      await writeFile(file, "{");
      const invalid = createRateCardSource(file);
      expect(invalid.current().status).toBe("invalid");
      invalid.close();

      await writeFile(
        file,
        JSON.stringify({
          currency: "USD",
          version: "v1",
          defaults: { cpu_core_hour: 0.03, memory_gib_hour: 0.004 },
        }),
      );
      const ready = createRateCardSource(file);
      expect(ready.current().status).toBe("ready");
      ready.close();
    } finally {
      await rm(directory, { force: true, recursive: true });
    }
  });
});

describe("cost calculations", () => {
  // Verifies: WEB-COST-04.01
  it("prices idle capacity as allocatable minus requested", () => {
    const snapshot = computePlatformSnapshot(cpuOnly, snapshotInput());
    expect(snapshot.clusters[0]?.idleCost).toBeCloseTo(24 * 0.03, 10);
    expect(snapshot.clusters[0]?.idleCost).toBeCloseTo(0.72, 10);
  });

  // Verifies: WEB-COST-04.02
  it("never reports negative idle cost on an over-committed cluster", () => {
    const snapshot = computePlatformSnapshot(
      cpuOnly,
      snapshotInput({ requestedCpu: [{ labels: cluster, value: 80 }] }),
    );
    expect(snapshot.clusters[0]?.idleCost).toBe(0);
  });

  it("excludes unpriced clusters from totals and lists them", () => {
    const card = rateCard({
      providers: { aws: { cpu_core_hour: 0.03, memory_gib_hour: 0 } },
    });
    const aws = { cluster: "c2", environment: "production", provider: "aws" };
    const snapshot = computePlatformSnapshot(
      card,
      snapshotInput({
        clusterCpu: [
          { labels: cluster, value: 64 },
          { labels: aws, value: 10 },
        ],
        clusterMemory: [],
        requestedCpu: [{ labels: aws, value: 5 }],
        requestedMemory: [],
        namespaceCpu: [
          { labels: { ...cluster, namespace: "openshell-a" }, value: 8 },
        ],
      }),
    );
    expect(snapshot.unpricedClusters).toEqual(["c1"]);
    expect(snapshot.clusters.map((row) => row.cluster)).toEqual(["c2"]);
    expect(snapshot.gateways.size).toBe(0);
  });

  it("prices memory in GiB-hours", () => {
    const card = rateCard({
      defaults: { cpu_core_hour: 0, memory_gib_hour: 0.004 },
    });
    const snapshot = computePlatformSnapshot(
      card,
      snapshotInput({
        clusterCpu: [{ labels: cluster, value: 0 }],
        clusterMemory: [{ labels: cluster, value: 100 * gib }],
        requestedCpu: [],
        requestedMemory: [{ labels: cluster, value: 25 * gib }],
      }),
    );
    expect(snapshot.clusters[0]?.capacityCost).toBeCloseTo(0.4, 10);
    expect(snapshot.clusters[0]?.idleCost).toBeCloseTo(0.3, 10);
  });

  // "Gateway namespaces"
  // Verifies: WEB-COST-04
  it("matches managed workspace namespaces but not operator ones", () => {
    const regex = gatewayNamespaceRegex([
      gateway("a"),
      gateway("b", "operator"),
      gateway("c", "shared"),
    ]);
    const matches = (namespace: string) =>
      new RegExp(`^(?:${regex})$`, "u").test(namespace);
    expect(matches("openshell-a")).toBe(true);
    expect(matches("openshell-a-team")).toBe(true);
    expect(matches("openshell-b")).toBe(true);
    expect(matches("openshell-b-team")).toBe(false);
    expect(matches("openshell-c-team")).toBe(false);
    expect(matches("openshell-z")).toBe(false);
    expect(gatewayNamespaceRegex([])).toBe("^$");
    expect(() =>
      gatewayNamespaceRegex([{ ...gateway("a"), namespace: 'x"}' }]),
    ).toThrow();
  });

  it("constrains scoped queries to the namespace set before aggregation", () => {
    const query = namespaceQuantityQuery(
      "hypershell:namespace_cpu_requests:cores",
      24,
      gatewayNamespaceRegex([gateway("a")]),
    );
    expect(query).toBe(
      'sum by (cluster, environment, provider, namespace) (sum_over_time(hypershell:namespace_cpu_requests:cores{namespace=~"(openshell-a)(-[a-z0-9-]+)?"}[24h:1h]))',
    );
  });

  it("resolves namespaces to the longest matching gateway namespace", () => {
    const resolve = namespaceOwnerResolver([gateway("a"), gateway("a-b")]);
    expect(resolve("openshell-a-b").gateway?.id).toBe("a-b");
    expect(resolve("openshell-a-b-ws").workspace).toBe("ws");
    expect(resolve("openshell-a-ws")).toMatchObject({ workspace: "ws" });
    expect(resolve("openshell-gone")).toEqual({});
  });

  // Verifies: WEB-COST-05.01
  it("attributes gateway, workspace, deleted and unused gateway cost", () => {
    const gateways = [gateway("a"), gateway("b")];
    const snapshot = computePlatformSnapshot(
      cpuOnly,
      snapshotInput({
        gateways,
        namespaceCpu: [
          { labels: { ...cluster, namespace: "openshell-a" }, value: 2 },
          { labels: { ...cluster, namespace: "openshell-a-ws1" }, value: 6 },
          { labels: { ...cluster, namespace: "openshell-b" }, value: 4 },
          { labels: { ...cluster, namespace: "openshell-deleted" }, value: 10 },
        ],
      }),
    );
    expect(snapshot.gateways.get("a")?.cost).toBeCloseTo(8 * 0.03, 10);
    expect(snapshot.gateways.get("a")?.workspaces.get("ws1")).toBeCloseTo(
      0.18,
      10,
    );
    expect(snapshot.deletedGatewayCost).toBeCloseTo(0.3, 10);

    const summary = buildSummaryResponse(cpuOnly, 7, snapshot, {
      activeNamespaces: new Set(["openshell-a"]),
      activeUsers: 4,
      currentGatewayCount: 2,
      meanActiveSandboxes: 2,
    });
    const monthly = (hourly: number) =>
      Math.round(hourly * hoursPerMonth * 100) / 100;
    expect(summary).toMatchObject({
      estimate: true,
      currency: "USD",
      rate_card_version: "2026-11-26",
      monthly_total: monthly(64 * 0.03),
      idle: {
        monthly: monthly(24 * 0.03),
        unused_gateways_monthly: monthly(4 * 0.03),
      },
      per_gateway: monthly((12 * 0.03) / 2),
      per_sandbox: monthly((6 * 0.03) / 2),
    });
    expect(summary.per_active_user).toBeCloseTo(monthly(64 * 0.03) / 4, 2);

    const breakdown = buildBreakdownResponse(cpuOnly, 7, snapshot, {
      billingOwners: new Map([
        ["a", { userId: "u1", username: "u1" }],
        ["b", { userId: "u1", username: "u1" }],
      ]),
    });
    // Gateway server cost includes deleted gateways; sandboxes are workspaces.
    expect(breakdown.by_resource_class).toEqual({
      gateways: monthly(16 * 0.03),
      sandboxes: monthly(6 * 0.03),
      platform_overhead: monthly(18 * 0.03),
      idle: monthly(24 * 0.03),
    });
    expect(breakdown.by_billing_owner).toEqual([
      { username: "u1", gateway_count: 2, monthly: monthly(12 * 0.03) },
    ]);
  });

  // Verifies: WEB-COST-06
  it("extrapolates partial coverage from the covered hours and reports it", () => {
    const snapshot = computePlatformSnapshot(
      cpuOnly,
      snapshotInput({
        coverageStartSeconds: 1_000_000 + 3 * 86_400,
        nowSeconds: 1_000_000 + 5 * 86_400,
        rangeHours: 7 * 24,
      }),
    );
    expect(snapshot.coveredHours).toBe(48);
    expect(snapshot.coverageStart).toBeDefined();
  });
});

describe("billing owner", () => {
  const grant = (
    id: string,
    username: string,
    grantedAt: string,
    role = "owner",
  ) => ({
    granted_at: grantedAt,
    role,
    role_binding_id: id,
    user_id: `id-${username}`,
    username,
  });

  // Verifies: WEB-COST-09.02
  it("picks the oldest owner grant, then the lowest binding ID", () => {
    expect(
      selectBillingOwner([
        grant("rb3", "u3", "2026-01-03T00:00:00Z"),
        grant("rb0", "viewer", "2025-01-01T00:00:00Z", "user"),
        grant("rb2", "u2", "2026-01-01T00:00:00Z"),
        grant("rb1", "u1", "2026-01-01T00:00:00Z"),
      ]),
    ).toEqual({ userId: "id-u1", username: "u1" });
  });

  // Verifies: WEB-COST-09.04
  it("moves to the next-oldest owner when the billing owner leaves", () => {
    expect(
      selectBillingOwner([
        grant("rb2", "u2", "2026-01-02T00:00:00Z"),
        grant("rb3", "u3", "2026-01-03T00:00:00Z"),
      ])?.username,
    ).toBe("u2");
  });
});

describe("my cost", () => {
  const a = gateway("a");
  const b = gateway("b");
  const scoped = {
    costs: new Map([
      ["a", { cluster: "c1", cost: 7.3, workspaces: new Map([["ws1", 5]]) }],
      ["b", { cluster: "c1", cost: 1.46, workspaces: new Map([["ws2", 1]]) }],
    ]),
    coveredHours: hoursPerMonth,
    unpricedClusters: [],
  };
  const inputs: MyCostGatewayInput[] = [
    {
      gateway: a,
      callerRole: "owner",
      billingOwner: { userId: "u1", username: "u1" },
      isBillingOwner: true,
    },
    {
      gateway: b,
      callerRole: "user",
      billingOwner: { userId: "u9", username: "u9" },
      isBillingOwner: false,
    },
  ];

  // Verifies: WEB-COST-09.01
  it("separates charged and shared gateways", () => {
    const summary = buildMyCostSummary(cpuOnly, 7, inputs, scoped, []);
    expect(summary.charged).toEqual({
      gateway_count: 1,
      monthly: 7.3,
      range_total: 7.3,
    });
    expect(summary.shared).toEqual({ gateway_count: 1, monthly: 1.46 });
    expect(summary.empty).toBe(false);

    const gateways = buildMyCostGateways(
      cpuOnly,
      7,
      inputs,
      scoped,
      (input) => () => input.callerRole === "owner",
      new Set(["openshell-a"]),
    );
    expect(gateways.gateways).toEqual([
      expect.objectContaining({
        id: "a",
        charged: true,
        role: "owner",
        unused: false,
      }),
      expect.objectContaining({
        id: "b",
        charged: false,
        billing_owner: "u9",
        unused: true,
      }),
    ]);
    // Workspace rows only on owned gateways (WEB-COST-09 step 4).
    expect(gateways.workspaces).toEqual([
      {
        gateway_id: "a",
        gateway_name: "Gateway a",
        workspace: "ws1",
        monthly: 5,
      },
    ]);
  });

  // Verifies: WEB-COST-09.03
  it("marks a user with no gateways as empty", () => {
    const summary = buildMyCostSummary(
      cpuOnly,
      7,
      [],
      { ...scoped, costs: new Map() },
      [],
    );
    expect(summary.empty).toBe(true);
  });
});
