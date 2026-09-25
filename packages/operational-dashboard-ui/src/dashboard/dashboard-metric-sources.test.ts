import { describe, expect, it } from "vitest";

import type { OperationalDashboardMetrics } from "../application/dashboard-types";
import {
  mergeOperationalDashboardMetrics,
  mergeReliabilityDashboardMetrics,
} from "./dashboard-metric-sources";

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

describe("mergeReliabilityDashboardMetrics", () => {
  const previousReliability: OperationalDashboardMetrics = {
    lastSuccessfulRefresh: new Date("2026-09-15T10:00:00.000Z"),
    metrics: [
      { id: "api-request-rate", unit: "req/s", value: "10.00" },
      { id: "api-error-rate", unit: "%", value: "1.00" },
      { id: "api-latency", unit: "sec", value: "0.080" },
    ],
  };

  it("preserves stale reliability metrics when api-reliability fails on refresh", () => {
    const next: OperationalDashboardMetrics = {
      failedSources: ["api-reliability"],
      lastSuccessfulRefresh: new Date("2026-09-15T11:00:00.000Z"),
      metrics: [],
    };

    expect(mergeReliabilityDashboardMetrics(previousReliability, next)).toEqual(
      {
        failedSources: ["api-reliability"],
        lastSuccessfulRefresh: next.lastSuccessfulRefresh,
        metrics: previousReliability.metrics,
      },
    );
  });

  it("returns the next payload when no reliability sources failed", () => {
    const next: OperationalDashboardMetrics = {
      lastSuccessfulRefresh: new Date("2026-09-15T11:00:00.000Z"),
      metrics: [
        { id: "api-request-rate", unit: "req/s", value: "12.50" },
        { id: "api-error-rate", unit: "%", value: "1.25" },
        { id: "api-latency", unit: "sec", value: "0.084" },
      ],
    };

    expect(mergeReliabilityDashboardMetrics(previousReliability, next)).toEqual(
      next,
    );
  });
});
