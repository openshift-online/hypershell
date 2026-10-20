import { describe, expect, it } from "vitest";

import type { GatewayClusterBreakdown, SandboxClusterCount } from "./fleet";
import { classifySpokes, hasSpokeRows } from "./spoke-attribution";
import type { InstanceTopology } from "./topology";

// Fictional fleet names only (firewall): a hub "alpha" with the co-located spoke
// "beta" and the remote spoke "gamma". The hub's own controller row follows the
// "<instance>-hub" convention: "alpha-hub".
function topology(overrides: Partial<InstanceTopology> = {}): InstanceTopology {
  return {
    instance: "alpha",
    hub: { instance: "alpha", dnsLabel: "alpha", remoteSpokes: ["gamma"] },
    spokes: [{ name: "beta" }],
    ...overrides,
  };
}

function gw(managedCluster: string, total: number): GatewayClusterBreakdown {
  return { managedCluster, gateways: { Ready: total }, total };
}

function sb(managedCluster: string, count: number): SandboxClusterCount {
  return { managedCluster, count };
}

describe("classifySpokes", () => {
  it("splits hub-own, co-located and remote rows against topology", () => {
    const result = classifySpokes(
      [gw("alpha-hub", 5), gw("beta", 3), gw("gamma", 2)],
      [sb("alpha-hub", 1), sb("beta", 4), sb("gamma", 2)],
      topology(),
    );

    expect(result.hasTopology).toBe(true);
    expect(result.hubOwn).toEqual({
      managedCluster: "alpha-hub",
      gateways: { Ready: 5 },
      gatewaysTotal: 5,
      sandboxes: 1,
    });
    expect(result.coLocated.map((r) => r.managedCluster)).toEqual(["beta"]);
    expect(result.remote.map((r) => r.managedCluster)).toEqual(["gamma"]);
    expect(result.unknown).toEqual([]);
  });

  it("matches the hub-own row by DNS label when the controller name differs", () => {
    const result = classifySpokes(
      [gw("alpha", 2)],
      [],
      topology({
        hub: { instance: "other", dnsLabel: "alpha", remoteSpokes: [] },
      }),
    );

    expect(result.hubOwn?.managedCluster).toBe("alpha");
    expect(result.unknown).toEqual([]);
  });

  it("drops every row into unknown with hasTopology false when topology is null", () => {
    const result = classifySpokes([gw("beta", 3)], [sb("gamma", 1)], null);

    expect(result.hasTopology).toBe(false);
    expect(result.hubOwn).toBeNull();
    expect(result.coLocated).toEqual([]);
    expect(result.remote).toEqual([]);
    expect(result.unknown.map((r) => r.managedCluster)).toEqual([
      "beta",
      "gamma",
    ]);
  });

  it("routes rows topology cannot place into unknown", () => {
    const result = classifySpokes(
      [gw("beta", 1), gw("stray", 1), gw("unknown", 9)],
      [],
      topology(),
    );

    expect(result.coLocated.map((r) => r.managedCluster)).toEqual(["beta"]);
    // "unknown" (busiest) sorts before "stray"; neither is in topology.
    expect(result.unknown.map((r) => r.managedCluster)).toEqual([
      "unknown",
      "stray",
    ]);
  });

  it("merges gateway-only and sandbox-only rows by managed cluster", () => {
    const result = classifySpokes(
      [gw("beta", 4)],
      [sb("beta", 2), sb("gamma", 7)],
      topology(),
    );

    expect(result.coLocated).toEqual([
      {
        managedCluster: "beta",
        gateways: { Ready: 4 },
        gatewaysTotal: 4,
        sandboxes: 2,
      },
    ]);
    // gamma had no gateways: kept with the missing side zeroed.
    expect(result.remote).toEqual([
      {
        managedCluster: "gamma",
        gateways: {},
        gatewaysTotal: 0,
        sandboxes: 7,
      },
    ]);
  });

  it("accumulates both sides when a managed cluster appears more than once", () => {
    // GMCA-05 guarantees one row per managed_cluster, but a malformed payload with
    // duplicate rows must sum (never last-write-wins) so it cannot silently undercount.
    const result = classifySpokes(
      [
        { managedCluster: "beta", gateways: { Ready: 3 }, total: 3 },
        {
          managedCluster: "beta",
          gateways: { Ready: 1, Pending: 2 },
          total: 3,
        },
      ],
      [sb("beta", 4), sb("beta", 5)],
      topology(),
    );

    expect(result.coLocated).toEqual([
      {
        managedCluster: "beta",
        gateways: { Ready: 4, Pending: 2 },
        gatewaysTotal: 6,
        sandboxes: 9,
      },
    ]);
  });

  it("skips rows with an empty managed-cluster name", () => {
    const result = classifySpokes(
      [gw("", 5), gw("beta", 1)],
      [sb("", 3)],
      topology(),
    );

    expect(result.coLocated.map((r) => r.managedCluster)).toEqual(["beta"]);
    expect(result.unknown).toEqual([]);
    expect(result.hubOwn).toBeNull();
  });

  it("orders each bucket busiest-first, ties broken by name", () => {
    const result = classifySpokes(
      [gw("gamma", 2), gw("delta", 2), gw("epsilon", 9)],
      [],
      topology({
        spokes: [{ name: "gamma" }, { name: "delta" }, { name: "epsilon" }],
        hub: { instance: "alpha", dnsLabel: "alpha", remoteSpokes: [] },
      }),
    );

    expect(result.coLocated.map((r) => r.managedCluster)).toEqual([
      "epsilon",
      "delta",
      "gamma",
    ]);
  });

  it("breaks equal gateway totals by sandbox count before name", () => {
    const result = classifySpokes(
      [gw("gamma", 2), gw("delta", 2)],
      // Equal gateway totals: delta has more sandboxes, so it sorts first even
      // though "gamma" would lose the name tie-break.
      [sb("gamma", 1), sb("delta", 5)],
      topology({
        spokes: [{ name: "gamma" }, { name: "delta" }],
        hub: { instance: "alpha", dnsLabel: "alpha", remoteSpokes: [] },
      }),
    );

    expect(result.coLocated.map((r) => r.managedCluster)).toEqual([
      "delta",
      "gamma",
    ]);
  });

  it("takes only the first hub-own match; later ones go to unknown", () => {
    const result = classifySpokes(
      [gw("alpha-hub", 5), gw("alpha", 1)],
      [],
      topology(),
    );

    expect(result.hubOwn?.managedCluster).toBe("alpha-hub");
    expect(result.unknown.map((r) => r.managedCluster)).toEqual(["alpha"]);
  });
});

describe("hasSpokeRows", () => {
  it("is false when only a hub-own row exists", () => {
    const result = classifySpokes([gw("alpha-hub", 5)], [], topology());
    expect(result.hubOwn).not.toBeNull();
    expect(hasSpokeRows(result)).toBe(false);
  });

  it("is true when co-located, remote or unknown rows exist", () => {
    expect(hasSpokeRows(classifySpokes([gw("beta", 1)], [], topology()))).toBe(
      true,
    );
    expect(hasSpokeRows(classifySpokes([gw("gamma", 1)], [], topology()))).toBe(
      true,
    );
    expect(hasSpokeRows(classifySpokes([gw("stray", 1)], [], null))).toBe(true);
  });
});
