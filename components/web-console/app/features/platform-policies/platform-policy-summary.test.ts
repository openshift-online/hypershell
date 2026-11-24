import { describe, expect, it } from "vitest";

import {
  baselinePolicy,
  gpuPolicy,
  platformPolicies,
} from "./fixtures/platform-policy-fixtures";
import {
  affectedGateways,
  enforcementImpact,
  filterAndSortPolicies,
  policyTargetSummary,
  rolloutSummary,
  writeFailureFor,
} from "./platform-policy-summary";

describe("platform policy summaries", () => {
  // Verifies: WEB-PADM-03
  it("computes the gateways and 24-hour findings an enforce change affects", () => {
    const impact = enforcementImpact(platformPolicies("audit"));
    expect(impact.gatewayCount).toBe(14);
    expect(impact.wouldReject).toBe(12);
    expect(impact.wouldAddDefaults).toBe(41);
    expect(
      impact.rows.map(({ gatewayId, note, wouldReject }) => [
        gatewayId,
        note,
        wouldReject,
      ]),
    ).toEqual([
      ["ml-training-gateway", "pending", 10],
      ["payments-gateway", undefined, 2],
      ["edge-lab-gateway", "unsupported", 0],
    ]);
  });

  it("summarizes targets, rollout, and affected gateways", () => {
    expect(policyTargetSummary(baselinePolicy().target)).toEqual({
      count: 0,
      kind: "all",
    });
    expect(policyTargetSummary(gpuPolicy().target)).toEqual({
      count: 2,
      kind: "except",
    });
    expect(
      policyTargetSummary({ all: false, exclude: [], gatewayIds: ["a"] }),
    ).toEqual({ count: 1, kind: "selected" });
    expect(rolloutSummary(baselinePolicy())).toMatchObject({
      appearance: "danger",
      applied: 2,
      total: 4,
    });
    expect(rolloutSummary({ ...gpuPolicy(), gateways: [] })).toMatchObject({
      appearance: "good",
      applied: 12,
      total: 12,
    });
    expect(affectedGateways(platformPolicies("audit"))).toEqual({
      affected: 2,
      total: 14,
    });
  });

  it("filters, sorts, and maps write failures", () => {
    const policies = platformPolicies();
    expect(
      filterAndSortPolicies(policies, "", "desc").map(({ name }) => name),
    ).toEqual(["sandbox-resource-baseline", "gpu-sandbox-cap"]);
    expect(filterAndSortPolicies(policies, "GPU", "asc")).toHaveLength(1);
    expect(writeFailureFor("denied")).toBe("denied");
    expect(writeFailureFor("conflicted")).toBe("conflicted");
    expect(writeFailureFor("unavailable")).toBe("failed");
  });
});
