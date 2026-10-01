import { describe, expect, it } from "vitest";

import {
  environmentGateBadge,
  governingInstance,
  orderedEnvironments,
} from "../../domain/promotion";
import { mapInstances, mapPromotion } from "./wire";

describe("mapPromotion", () => {
  // A payload shaped exactly like the BFF/promotion.json wire contract (§5).
  const wire = {
    order: ["hyp0mc0", "hyp0"],
    environments: {
      hyp0: {
        active: { version: "c2bdf68d", sha: "c2bdf68dc5" },
        proposed: { version: "c2bdf68d", sha: "c2bdf68dc5" },
        activeGates: [
          { key: "hypershell-analysis", phase: "success" },
          { key: "argocd-health", phase: "success" },
        ],
        proposedGates: [{ key: "dependents-successful", phase: "success" }],
      },
      hyp0mc0: {
        active: { version: "c2bdf68d" },
        proposed: { version: "c2bdf68d" },
        activeGates: [{ key: "argocd-health", phase: "success" }],
        proposedGates: [],
      },
    },
    releases: { c2bdf68d: { version: "c2bdf68d" } },
    frontier: { version: "c2bdf68d" },
  };

  it("projects the wire shape into the domain model", () => {
    const data = mapPromotion(wire);
    expect(data.order).toEqual(["hyp0mc0", "hyp0"]);
    const hyp0 = data.environments.hyp0;
    if (!hyp0) throw new Error("expected hyp0 environment");
    expect(hyp0.name).toBe("hyp0");
    expect(hyp0.activeRelease).toBe("c2bdf68d");
    expect(hyp0.gates.map((g) => g.name)).toEqual([
      "hypershell-analysis",
      "argocd-health",
    ]);
    expect(data.releases).toEqual(["c2bdf68d"]);
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
      "hyp0mc0",
      "hyp0",
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
      order: ["x"],
      environments: { x: { activeGates: [] } },
    });
    const x = data.environments.x;
    if (!x) throw new Error("expected x environment");
    expect(x.activeRelease).toBeNull();
    expect(x.proposedRelease).toBeNull();
    expect(x.gates).toEqual([]);
  });
});

describe("mapInstances", () => {
  it("expands bare instance names into summaries", () => {
    const data = mapInstances({ instances: ["hyp0", "hyp1"] });
    expect(data.instances.map((i) => i.name)).toEqual(["hyp0", "hyp1"]);
    expect(data.instances[0]).toMatchObject({
      name: "hyp0",
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
