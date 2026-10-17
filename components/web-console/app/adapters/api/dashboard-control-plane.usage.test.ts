import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SDKClient } from "@openshift-online/hypershell-sdk";

import { createDashboardControlPlaneAdapter } from "./dashboard-control-plane";

const fetchMock = vi.fn();
const apiFactoryMock = vi.fn(() => ({}));
const adapter = createDashboardControlPlaneAdapter(
  apiFactoryMock as unknown as (correlationId: string) => SDKClient,
);
const context = {
  correlationId: "11111111-1111-4111-8111-111111111111",
  isAdmin: false,
  timeRange: 30,
} as const;

async function getUsageMetrics() {
  if (!adapter.getUsageMetrics) {
    throw new Error("usage metrics control plane method is missing");
  }
  return adapter.getUsageMetrics(context);
}

const gatewayResponse = {
  active_gateways: 1,
  daily_request_error_rate_percent: [
    { date: "2026-10-14", value: 1 },
    { date: "2026-10-15", value: null },
  ],
  daily_request_volume: [
    { date: "2026-10-14", value: 100 },
    { date: "2026-10-15", value: null },
  ],
  request_error_rate_percent: 1,
  request_volume: 198,
  total_gateways: 2,
  unused_gateways: 1,
};

const sandboxResponse = {
  active_sandboxes: 5,
  daily_utilization_percent: [
    { date: "2026-10-14", value: 50 },
    { date: "2026-10-15", value: null },
  ],
  sandbox_utilization_percent: 50,
  total_sandboxes: 10,
};

const lifetimeResponse = {
  daily_mean_seconds: [
    { date: "2026-10-14", value: 120 },
    { date: "2026-10-15", value: null },
  ],
  mean_seconds: 120,
  p50_seconds: 100,
  p95_seconds: 300,
};

function mockSuccessfulUsageResponses(): void {
  fetchMock.mockImplementation((url: string) => {
    if (url === "/api/metrics/usage/gateways?range=30") {
      return Promise.resolve({
        json: () => Promise.resolve(gatewayResponse),
        ok: true,
      });
    }
    if (url === "/api/metrics/usage/sandboxes?range=30") {
      return Promise.resolve({
        json: () => Promise.resolve(sandboxResponse),
        ok: true,
      });
    }
    if (url === "/api/metrics/usage/sandbox-lifetime?range=30") {
      return Promise.resolve({
        json: () => Promise.resolve(lifetimeResponse),
        ok: true,
      });
    }
    return Promise.reject(new Error(`unexpected fetch url: ${url}`));
  });
}

describe("createDashboardControlPlaneAdapter usage metrics", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    fetchMock.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("maps all usage sources and preserves partial daily data", async () => {
    mockSuccessfulUsageResponses();

    const metrics = await getUsageMetrics();

    expect(metrics.failedSources).toBeUndefined();
    expect(metrics.timeRange).toBe(30);
    expect(metrics.metrics).toEqual([
      {
        id: "gateway-request-volume",
        trend: {
          points: [
            { label: "2026-10-14", value: 100 },
            { label: "2026-10-15", value: null },
          ],
        },
        value: "198",
      },
      {
        id: "gateway-error-rate",
        trend: {
          points: [
            { label: "2026-10-14", value: 1 },
            { label: "2026-10-15", value: null },
          ],
        },
        value: "1",
      },
      {
        activeSharePercent: "50",
        id: "active-gateways",
        total: "2",
        value: "1",
      },
      { id: "unused-gateways", value: "1" },
      {
        id: "sandbox-utilization",
        total: "10",
        trend: {
          points: [
            { label: "2026-10-14", value: 50 },
            { label: "2026-10-15", value: null },
          ],
        },
        unit: "sandboxes",
        value: "50",
      },
      {
        id: "sandbox-lifetime",
        sandboxLifetime: { mean: "120", p50: "100", p95: "300" },
        trend: {
          points: [
            { label: "2026-10-14", value: 120 },
            { label: "2026-10-15", value: null },
          ],
        },
        value: "120",
      },
    ]);
  });

  it("keeps successful sources when one usage route fails", async () => {
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/usage/gateways?range=30") {
        return Promise.resolve({ ok: false, status: 502 });
      }
      if (url === "/api/metrics/usage/sandboxes?range=30") {
        return Promise.resolve({
          json: () => Promise.resolve(sandboxResponse),
          ok: true,
        });
      }
      if (url === "/api/metrics/usage/sandbox-lifetime?range=30") {
        return Promise.resolve({
          json: () => Promise.resolve(lifetimeResponse),
          ok: true,
        });
      }
      return Promise.reject(new Error(`unexpected fetch url: ${url}`));
    });

    const metrics = await getUsageMetrics();

    expect(metrics.failedSources).toEqual(["usage-gateways"]);
    expect(metrics.metrics.map((metric) => metric.id)).toEqual([
      "sandbox-utilization",
      "sandbox-lifetime",
    ]);
  });

  it("preserves partial lifetime values without substituting the mean", async () => {
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/usage/gateways?range=30") {
        return Promise.resolve({
          json: () => Promise.resolve({}),
          ok: true,
        });
      }
      if (url === "/api/metrics/usage/sandboxes?range=30") {
        return Promise.resolve({
          json: () => Promise.resolve({}),
          ok: true,
        });
      }
      if (url === "/api/metrics/usage/sandbox-lifetime?range=30") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              p50_seconds: 100,
              p95_seconds: 300,
            }),
          ok: true,
        });
      }
      return Promise.reject(new Error(`unexpected fetch url: ${url}`));
    });

    const metrics = await getUsageMetrics();
    const lifetime = metrics.metrics.find(
      (metric) => metric.id === "sandbox-lifetime",
    );

    expect(lifetime).toMatchObject({
      sandboxLifetime: { p50: "100", p95: "300" },
      value: "NaN",
    });
  });
});
