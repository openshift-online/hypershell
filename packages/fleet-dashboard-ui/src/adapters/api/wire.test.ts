import { describe, expect, it } from "vitest";

import {
  environmentGateBadge,
  governingInstance,
  orderedEnvironments,
} from "../../domain/promotion";
import { mapFleet, mapInstances, mapPromotion } from "./wire";

// Fixtures use deliberately fictional environment/instance names (alpha/beta/
// gamma) and a fake release id. The fleet-dashboard source is public, so real
// fleet identifiers must never appear in it -- not even in tests
// (data-architecture.spec §3.5, enforced by scripts/check_fleet_dashboard_firewall.sh).
describe("mapPromotion", () => {
  // A payload shaped exactly like the BFF/promotion.json wire contract (§5).
  const wire = {
    order: ["alpha", "beta"],
    environments: {
      beta: {
        active: { version: "abc1234", sha: "abc1234def" },
        proposed: { version: "abc1234", sha: "abc1234def" },
        activeGates: [
          { key: "hypershell-analysis", phase: "success" },
          { key: "argocd-health", phase: "success" },
        ],
        proposedGates: [{ key: "dependents-successful", phase: "success" }],
      },
      alpha: {
        active: { version: "abc1234" },
        proposed: { version: "abc1234" },
        activeGates: [{ key: "argocd-health", phase: "success" }],
        proposedGates: [],
      },
    },
    releases: { abc1234: { version: "abc1234" } },
    frontier: { version: "abc1234" },
  };

  it("projects the wire shape into the domain model", () => {
    const data = mapPromotion(wire);
    expect(data.order).toEqual(["alpha", "beta"]);
    const beta = data.environments.beta;
    if (!beta) throw new Error("expected beta environment");
    expect(beta.name).toBe("beta");
    expect(beta.activeRelease).toBe("abc1234");
    expect(beta.gates.map((g) => g.name)).toEqual([
      "hypershell-analysis",
      "argocd-health",
    ]);
    expect(data.releases).toEqual(["abc1234"]);
  });

  it("produces a model the domain helpers consume without throwing", () => {
    // This is the regression guard: the un-mapped wire object has `activeGates`
    // but no `gates`, so governingInstance/environmentGateBadge threw at render.
    const data = mapPromotion(wire);
    for (const env of orderedEnvironments(data)) {
      expect(() => governingInstance(env)).not.toThrow();
      expect(() => environmentGateBadge(env)).not.toThrow();
    }
    expect(orderedEnvironments(data).map((e) => e.name)).toEqual([
      "alpha",
      "beta",
    ]);
  });

  it("tolerates missing/empty fields", () => {
    const data = mapPromotion({});
    expect(data.order).toEqual([]);
    expect(data.environments).toEqual({});
    expect(data.releases).toEqual([]);
  });

  it("prefers the bundle digest over the gitops SHA for release identity", () => {
    const data = mapPromotion({
      order: ["delta"],
      environments: {
        delta: {
          active: {
            version: "v20260930",
            sha: "aaaaaaaa",
            digest: "sha256:dig",
          },
          proposed: {
            version: "v20260930",
            sha: "bbbbbbbb",
            digest: "sha256:dig",
          },
          activeGates: [],
        },
      },
      releases: {
        "sha256:dig": {
          version: "v20260930",
          sha: "aaaaaaaa",
          digest: "sha256:dig",
        },
      },
    });
    const delta = data.environments.delta;
    if (!delta) throw new Error("expected delta environment");
    // Two envs on the same bundle collapse: active/proposed point at the digest,
    // not the differing per-env gitops SHAs.
    expect(delta.activeDigest).toBe("sha256:dig");
    expect(delta.proposedDigest).toBe("sha256:dig");
    expect(data.releaseByDigest["sha256:dig"]?.digest).toBe("sha256:dig");
  });

  it("falls back to the SHA as digest when no bundle digest is present", () => {
    const data = mapPromotion({
      order: ["epsilon"],
      environments: {
        epsilon: {
          active: { version: "shortsha", sha: "cccccccc" },
          activeGates: [],
        },
      },
    });
    const epsilon = data.environments.epsilon;
    if (!epsilon) throw new Error("expected epsilon environment");
    expect(epsilon.activeDigest).toBe("cccccccc");
  });

  it("maps absent release versions to null", () => {
    const data = mapPromotion({
      order: ["gamma"],
      environments: { gamma: { activeGates: [] } },
    });
    const gamma = data.environments.gamma;
    if (!gamma) throw new Error("expected gamma environment");
    expect(gamma.activeRelease).toBeNull();
    expect(gamma.proposedRelease).toBeNull();
    expect(gamma.gates).toEqual([]);
  });
});

describe("mapFleet", () => {
  it("flattens the keyed wire map into a sorted instance list", () => {
    const data = mapFleet({
      zeta: { gateways: { running: 2 }, gatewaysTotal: 2 },
      alpha: { instance: "alpha", gatewaysTotal: 5 },
    });
    expect(data.instances.map((i) => i.instance)).toEqual(["alpha", "zeta"]);
  });

  it("defaults rate triples, history and nullable gauges", () => {
    const data = mapFleet({ a: {} });
    const inst = data.instances[0];
    expect(inst?.instance).toBe("a");
    expect(inst?.rpc).toEqual({ rate: 0, errorPct: 0, p95Ms: 0 });
    expect(inst?.gatewayHistory).toEqual([]);
    expect(inst?.managedClusters).toBeNull();
    expect(inst?.role).toBeNull();
    expect(inst?.sandboxes).toBe(0);
    expect(inst?.sandboxesByCluster).toEqual([]);
  });

  it("carries metrics, totals and history through", () => {
    const data = mapFleet({
      a: {
        gateways: { running: 3, failed: 1 },
        gatewaysTotal: 4,
        users: 12,
        rpc: { rate: 2, errorPct: 0.5, p95Ms: 40 },
        provisionP95Ms: 120,
        gatewayHistory: [
          { running: 1 },
          { running: 2, provisioning: 1 },
          { failed: 4 },
        ],
        sandboxes: 9,
        sandboxesByCluster: [
          { cluster: "c2", count: 6 },
          { cluster: "c1", count: 3 },
          // A row with no cluster label is dropped (identity-less, unplottable).
          { count: 2 },
        ],
      },
    });
    const inst = data.instances[0];
    expect(inst?.gatewaysTotal).toBe(4);
    expect(inst?.users).toBe(12);
    expect(inst?.rpc.p95Ms).toBe(40);
    expect(inst?.provisionP95Ms).toBe(120);
    expect(inst?.sandboxes).toBe(9);
    expect(inst?.sandboxesByCluster).toEqual([
      { cluster: "c2", count: 6 },
      { cluster: "c1", count: 3 },
    ]);
    // Missing phase fields default to 0 (num()), so every sample is fully shaped.
    expect(inst?.gatewayHistory).toEqual([
      { running: 1, provisioning: 0, failed: 0 },
      { running: 2, provisioning: 1, failed: 0 },
      { running: 0, provisioning: 0, failed: 4 },
    ]);
  });

  it("tolerates a null/empty payload", () => {
    expect(mapFleet({}).instances).toEqual([]);
    expect(mapFleet(null).instances).toEqual([]);
  });
});

describe("mapInstances", () => {
  it("expands bare instance names into summaries", () => {
    const data = mapInstances({ instances: ["alpha", "beta"] });
    expect(data.instances.map((i) => i.name)).toEqual(["alpha", "beta"]);
    expect(data.instances[0]).toMatchObject({
      name: "alpha",
      role: null,
      provider: null,
      region: null,
      health: null,
    });
  });

  it("tolerates a null/absent list", () => {
    expect(mapInstances({}).instances).toEqual([]);
    expect(mapInstances({ instances: null }).instances).toEqual([]);
  });
});
