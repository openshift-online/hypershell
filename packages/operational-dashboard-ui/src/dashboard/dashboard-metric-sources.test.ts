import { describe, expect, it } from "vitest";

import type {
  OperationalDashboardMetrics,
  UsageDashboardMetrics,
} from "../application/dashboard-types";
import {
  mergeOperationalDashboardMetrics,
  mergeReliabilityDashboardMetrics,
  mergeUsageDashboardMetrics,
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
      { id: "api-request-rate", unit: "requests/sec", value: "10.00" },
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
        { id: "api-request-rate", unit: "requests/sec", value: "12.500" },
        { id: "api-error-rate", unit: "%", value: "1.250" },
        { id: "api-latency", unit: "sec", value: "0.084" },
      ],
    };

    expect(mergeReliabilityDashboardMetrics(previousReliability, next)).toEqual(
      next,
    );
  });
});

describe("mergeUsageDashboardMetrics", () => {
  const previousUsage: UsageDashboardMetrics = {
    lastSuccessfulRefresh: new Date("2026-10-15T10:00:00.000Z"),
    metrics: [
      { id: "gateway-request-volume", unit: "requests", value: "100" },
      { id: "gateway-error-rate", unit: "%", value: "1.00" },
      { id: "sandbox-utilization", total: "20", unit: "sandboxes", value: "8" },
    ],
  };

  it("preserves only stale metrics from the failed usage source", () => {
    const next: UsageDashboardMetrics = {
      failedSources: ["usage-gateways"],
      lastSuccessfulRefresh: new Date("2026-10-15T11:00:00.000Z"),
      metrics: [
        {
          id: "sandbox-utilization",
          total: "24",
          unit: "sandboxes",
          value: "10",
        },
      ],
    };

    expect(mergeUsageDashboardMetrics(previousUsage, next)).toEqual({
      failedSources: ["usage-gateways"],
      lastSuccessfulRefresh: next.lastSuccessfulRefresh,
      metrics: [
        {
          id: "sandbox-utilization",
          total: "24",
          unit: "sandboxes",
          value: "10",
        },
        { id: "gateway-request-volume", unit: "requests", value: "100" },
        { id: "gateway-error-rate", unit: "%", value: "1.00" },
      ],
    });
  });

  it("returns the next payload when every usage source succeeds", () => {
    const next: UsageDashboardMetrics = {
      lastSuccessfulRefresh: new Date("2026-10-15T11:00:00.000Z"),
      metrics: [
        { id: "gateway-request-volume", unit: "requests", value: "120" },
      ],
    };

    expect(mergeUsageDashboardMetrics(previousUsage, next)).toEqual(next);
  });
});
