import { describe, expect, it } from "vitest";

import type { OperationalDashboardMetrics } from "../application/dashboard-types";
import { mergeOperationalDashboardMetrics } from "./dashboard-metric-sources";

const previousMetrics: OperationalDashboardMetrics = {
  lastSuccessfulRefresh: new Date("2026-08-25T10:00:00.000Z"),
  metrics: [
    { id: "provisioned-gateways", value: "10" },
    { id: "memory", total: "32", unit: "GiB", value: "16" },
  ],
};

describe("mergeOperationalDashboardMetrics", () => {
  it("returns the next payload when there is no previous data", () => {
    const next: OperationalDashboardMetrics = {
      failedSources: ["cluster-memory"],
      lastSuccessfulRefresh: new Date("2026-08-25T11:00:00.000Z"),
      metrics: [{ id: "provisioned-gateways", value: "12" }],
    };

    expect(mergeOperationalDashboardMetrics(undefined, next)).toEqual(next);
  });

  it("preserves stale metrics for failed sources on refresh", () => {
    const next: OperationalDashboardMetrics = {
      failedSources: ["cluster-memory"],
      lastSuccessfulRefresh: new Date("2026-08-25T11:00:00.000Z"),
      metrics: [{ id: "provisioned-gateways", value: "12" }],
    };

    expect(mergeOperationalDashboardMetrics(previousMetrics, next)).toEqual({
      failedSources: ["cluster-memory"],
      lastSuccessfulRefresh: next.lastSuccessfulRefresh,
      metrics: [
        { id: "provisioned-gateways", value: "12" },
        { id: "memory", total: "32", unit: "GiB", value: "16" },
      ],
    });
  });

  it("returns the next payload unchanged when no sources failed", () => {
    const next: OperationalDashboardMetrics = {
      lastSuccessfulRefresh: new Date("2026-08-25T11:00:00.000Z"),
      metrics: [
        { id: "provisioned-gateways", value: "12" },
        { id: "memory", total: "64", unit: "GiB", value: "20" },
      ],
    };

    expect(mergeOperationalDashboardMetrics(previousMetrics, next)).toEqual(
      next,
    );
  });
});
