import { describe, expect, it } from "vitest";

import type { ReleaseBundle } from "../promotion";
import {
  bundleList,
  deployedCount,
  deployedFor,
  seedForBundle,
} from "./bundles";
import type { MapNode } from "./model";

function bundle(overrides: Partial<ReleaseBundle>): ReleaseBundle {
  return {
    version: "v0",
    digest: null,
    tag: null,
    date: null,
    ...overrides,
  };
}

function node(overrides: Partial<MapNode> & { id: string }): MapNode {
  return {
    columnKey: "c",
    laneKey: "hub",
    laneKind: "hub",
    role: null,
    provider: null,
    cluster: null,
    seed: overrides.id,
    version: null,
    digest: null,
    proposedVersion: null,
    proposedDigest: null,
    state: "behind",
    upToDate: false,
    argoHealth: null,
    argoSync: null,
    prState: null,
    gateBadge: { tone: "unknown", labelKey: "unknown" },
    gateways: {},
    gatewaysTotal: 0,
    gatewayTone: "unknown",
    gatewayHistory: [],
    managedClusters: null,
    users: null,
    metrics: {
      rpc: { rate: 0, errorPct: 0, p95Ms: 0 },
      reconcile: { rate: 0, errorPct: 0, p95Ms: 0 },
      bff: { rate: 0, errorPct: 0, p95Ms: 0 },
      provisionP95Ms: null,
    },
    links: { console: null, argo: null, pr: null, analysis: null },
    ...overrides,
  };
}

describe("bundleList", () => {
  it("orders by date descending when dates exist", () => {
    const list = bundleList({
      old: bundle({ version: "v1", date: "2026-01-01" }),
      new: bundle({ version: "v3", date: "2026-03-01" }),
      mid: bundle({ version: "v2", date: "2026-02-01" }),
    });
    expect(list.map((b) => b.version)).toEqual(["v3", "v2", "v1"]);
  });

  it("sorts dated bundles before undated ones", () => {
    const list = bundleList({
      undated: bundle({ version: "v2" }),
      dated: bundle({ version: "v1", date: "2026-01-01" }),
    });
    expect(list[0]?.version).toBe("v1");
  });

  it("falls back to version descending without dates", () => {
    const list = bundleList({
      a: bundle({ version: "v1" }),
      b: bundle({ version: "v3" }),
      c: bundle({ version: "v2" }),
    });
    expect(list.map((b) => b.version)).toEqual(["v3", "v2", "v1"]);
  });

  it("breaks version ties by digest for stability", () => {
    const list = bundleList({
      a: bundle({ version: "v1", digest: "sha256:bbb" }),
      b: bundle({ version: "v1", digest: "sha256:aaa" }),
    });
    expect(list.map((b) => b.digest)).toEqual(["sha256:bbb", "sha256:aaa"]);
  });
});

describe("seedForBundle", () => {
  it("prefers the digest", () => {
    expect(seedForBundle(bundle({ version: "v1", digest: "sha256:x" }))).toBe(
      "sha256:x",
    );
  });

  it("falls back to the version", () => {
    expect(seedForBundle(bundle({ version: "v1" }))).toBe("v1");
  });
});

describe("deployedFor / deployedCount", () => {
  const nodes = [
    node({ id: "a", digest: "sha256:1", version: "v1" }),
    node({ id: "b", digest: "sha256:1", version: "v1" }),
    node({ id: "c", digest: "sha256:2", version: "v2" }),
  ];

  it("matches on digest when both sides have one", () => {
    const got = deployedFor(
      bundle({ version: "v1", digest: "sha256:1" }),
      nodes,
    );
    expect(got.map((n) => n.id)).toEqual(["a", "b"]);
  });

  it("matches on version when the bundle has no digest", () => {
    const got = deployedFor(bundle({ version: "v2" }), nodes);
    expect(got.map((n) => n.id)).toEqual(["c"]);
  });

  it("counts deployments", () => {
    expect(
      deployedCount(bundle({ version: "v1", digest: "sha256:1" }), nodes),
    ).toBe(2);
  });

  it("returns none when nothing matches", () => {
    expect(
      deployedFor(bundle({ version: "v9", digest: "sha256:9" }), nodes),
    ).toEqual([]);
  });
});
