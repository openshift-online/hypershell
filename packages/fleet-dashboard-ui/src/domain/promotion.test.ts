import { describe, expect, it } from "vitest";

import {
  environmentGateBadge,
  governingInstance,
  hasPendingPromotion,
  orderedEnvironments,
  type PromotionData,
  type PromotionEnvironment,
} from "./promotion";

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

describe("orderedEnvironments", () => {
  it("renders columns in the server-defined order, never re-sorting", () => {
    const promotion: PromotionData = {
      order: ["c", "a", "b"],
      environments: {
        a: env({ name: "a" }),
        b: env({ name: "b" }),
        c: env({ name: "c" }),
      },
      releases: [],
      releaseByDigest: {},
      frontier: null,
    };
    expect(orderedEnvironments(promotion).map((e) => e.name)).toEqual([
      "c",
      "a",
      "b",
    ]);
  });

  it("skips ordered keys that have no environment (fail-soft)", () => {
    const promotion: PromotionData = {
      order: ["a", "ghost", "b"],
      environments: { a: env({ name: "a" }), b: env({ name: "b" }) },
      releases: [],
      releaseByDigest: {},
      frontier: null,
    };
    expect(orderedEnvironments(promotion).map((e) => e.name)).toEqual([
      "a",
      "b",
    ]);
  });

  it("appends environments missing from order so none disappear", () => {
    const promotion: PromotionData = {
      order: ["a"],
      environments: { a: env({ name: "a" }), extra: env({ name: "extra" }) },
      releases: [],
      releaseByDigest: {},
      frontier: null,
    };
    expect(orderedEnvironments(promotion).map((e) => e.name)).toEqual([
      "a",
      "extra",
    ]);
  });
});

describe("governingInstance", () => {
  it("derives the instance from gate data, never from the env name", () => {
    const e = env({
      name: "somewhere",
      gates: [
        { name: "g1", phase: "success", governingInstance: null },
        { name: "g2", phase: "pending", governingInstance: "reported-inst" },
      ],
    });
    expect(governingInstance(e)).toBe("reported-inst");
  });

  it("returns null when the server reports no governing instance", () => {
    const e = env({
      name: "somewhere",
      gates: [{ name: "g1", phase: "success", governingInstance: null }],
    });
    expect(governingInstance(e)).toBeNull();
  });
});

describe("environmentGateBadge", () => {
  it("returns unknown when there are no gates", () => {
    expect(environmentGateBadge(env({ name: "x" })).tone).toBe("unknown");
  });

  it("surfaces the worst gate (danger dominates success)", () => {
    const e = env({
      name: "x",
      gates: [
        { name: "ok", phase: "success", governingInstance: null },
        { name: "bad", phase: "failed", governingInstance: null },
      ],
    });
    expect(environmentGateBadge(e).tone).toBe("danger");
  });

  it("shows the spinner (info) when any gate is pending, even beside a passed gate", () => {
    const e = env({
      name: "x",
      gates: [
        { name: "argocd-health", phase: "succeeded", governingInstance: null },
        { name: "analysis", phase: "pending", governingInstance: null },
      ],
    });
    // Roll-up: pending (info) beats good (success), so an in-flight gate surfaces.
    expect(environmentGateBadge(e).tone).toBe("info");
  });

  it("lets a good gate outrank an unknown one (unknown never masks a result)", () => {
    const e = env({
      name: "x",
      gates: [
        { name: "ok", phase: "success", governingInstance: null },
        { name: "mystery", phase: "weird", governingInstance: null },
      ],
    });
    expect(environmentGateBadge(e).tone).toBe("success");
  });

  it("still lets a failure dominate a pending gate", () => {
    const e = env({
      name: "x",
      gates: [
        { name: "analysis", phase: "pending", governingInstance: null },
        { name: "bad", phase: "failed", governingInstance: null },
      ],
    });
    expect(environmentGateBadge(e).tone).toBe("danger");
  });
});

describe("hasPendingPromotion", () => {
  it("is true when proposed differs from active", () => {
    expect(
      hasPendingPromotion(
        env({ name: "x", activeRelease: "v1", proposedRelease: "v2" }),
      ),
    ).toBe(true);
  });

  it("is false when proposed matches active", () => {
    expect(
      hasPendingPromotion(
        env({ name: "x", activeRelease: "v1", proposedRelease: "v1" }),
      ),
    ).toBe(false);
  });

  it("is false when nothing is proposed", () => {
    expect(hasPendingPromotion(env({ name: "x", activeRelease: "v1" }))).toBe(
      false,
    );
  });
});
