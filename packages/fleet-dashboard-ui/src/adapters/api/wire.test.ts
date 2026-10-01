import { describe, expect, it } from "vitest";

import {
  environmentGateBadge,
  governingInstance,
  orderedEnvironments,
} from "../../domain/promotion";
import { mapInstances, mapPromotion } from "./wire";

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
