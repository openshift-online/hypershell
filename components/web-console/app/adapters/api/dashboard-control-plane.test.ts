import type { SDKClient } from "@openshift-online/hypershell-sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createDashboardControlPlaneAdapter } from "./dashboard-control-plane";
import type { PlatformInventoryMetricsResponse } from "./platform-inventory-aggregation";

const fetchMock = vi.fn();
const gatewaysListMock = vi.fn();
const gatewayReleasesListMock = vi.fn();
const apiFactory = vi.fn(
  () =>
    ({
      gateways: {
        list: gatewaysListMock,
      },
      gatewayReleases: {
        list: gatewayReleasesListMock,
      },
    }) as unknown as SDKClient,
);

const adapter = createDashboardControlPlaneAdapter(apiFactory);
const context = {
  correlationId: "11111111-1111-4111-8111-111111111111",
};

const mockClusterPodsResponse = {
  available_pods: 1452,
  capacity_pods: 2000,
  phase_failed_pods: 16,
  phase_pending_pods: 12,
  phase_running_pods: 500,
  phase_succeeded_pods: 20,
  phase_unknown_pods: 0,
  used_pods: 548,
};

const mockGatewayProvisionDurationResponse = {
  mean_seconds: 315,
  observation_count: 2,
  p50_seconds: 288,
  p95_seconds: 726,
};

const mockGatewayProvisionOutcomesResponse = {
  failure_count_24h: 1,
  hourly_success_rate: [
    {
      failure_count: 1,
      hour: "2026-08-09T12:00",
      success_count: 4,
      success_rate_percent: 80,
    },
    {
      failure_count: 0,
      hour: "2026-08-09T13:00",
      success_count: 5,
      success_rate_percent: 100,
    },
  ],
  success_count_24h: 9,
  success_rate_percent: 90,
};

const defaultGatewayPhaseCounts = {
  Running: 100,
  Provisioning: 50,
};

function defaultPlatformInventory(): PlatformInventoryMetricsResponse {
  return {
    managed_clusters: {
      by_provider: {},
      by_region: {},
      by_status: {},
      created_last_30_days: 0,
      total: 0,
    },
  };
}

interface RegisteredUsersMockResponse {
  created_last_7_days?: number;
  created_last_30_days?: number;
  daily_unique_logins?: { count: number; date: string }[];
  total_registered: number;
  unique_logins_last_7_days?: number;
  unique_logins_last_30_days?: number;
}

interface DashboardMetricsMockOptions {
  activeSandboxes?: number;
  omitProvisionDuration?: boolean;
  omitProvisionOutcomes?: boolean;
  platformInventory?: PlatformInventoryMetricsResponse;
  provisionOutcomes?: Omit<
    typeof mockGatewayProvisionOutcomesResponse,
    "success_rate_percent"
  > & {
    success_rate_percent: number | null;
  };
  registeredUsers?: RegisteredUsersMockResponse;
}

interface DashboardFetchMockResponse {
  json: () => Promise<unknown>;
  ok: boolean;
  status?: number;
}

type DashboardFetchMock = (url: string) => Promise<DashboardFetchMockResponse>;

function getDashboardFetchMock(): DashboardFetchMock | undefined {
  return fetchMock.getMockImplementation() as DashboardFetchMock | undefined;
}

function delegateDashboardFetch(
  baseFetch: DashboardFetchMock | undefined,
  url: string,
): Promise<DashboardFetchMockResponse> {
  if (baseFetch === undefined) {
    throw new Error(`Unexpected fetch to ${url}`);
  }
  return baseFetch(url);
}

function mockClusterMetricsResponses(
  capacityBytes: number,
  usedBytes: number,
  gatewayPhaseCounts: Record<string, number> = defaultGatewayPhaseCounts,
  options: DashboardMetricsMockOptions = {},
): void {
  const activeSandboxes = options.activeSandboxes ?? 0;
  const registeredUsers =
    options.registeredUsers ?? defaultRegisteredUsersResponse(0);
  const platformInventory =
    options.platformInventory ?? defaultPlatformInventory();

  fetchMock.mockImplementation((url: string) => {
    if (url === "/api/metrics/gateways") {
      return Promise.resolve({
        json: () => Promise.resolve({ counts: gatewayPhaseCounts }),
        ok: true,
      });
    }
    if (url === "/api/metrics/gateway-sandboxes") {
      return Promise.resolve({
        json: () => Promise.resolve({ active_sandboxes: activeSandboxes }),
        ok: true,
      });
    }
    if (url === "/api/metrics/registered-users") {
      return Promise.resolve({
        json: () => Promise.resolve(registeredUsers),
        ok: true,
      });
    }
    if (url === "/api/metrics/platform-inventory") {
      return Promise.resolve({
        json: () => Promise.resolve(platformInventory),
        ok: true,
      });
    }
    if (url === "/api/metrics/cluster-memory") {
      return Promise.resolve({
        json: () =>
          Promise.resolve({
            available_bytes: capacityBytes - usedBytes,
            capacity_bytes: capacityBytes,
            used_bytes: usedBytes,
          }),
        ok: true,
      });
    }
    if (url === "/api/metrics/cluster-cpu") {
      return Promise.resolve({
        json: () =>
          Promise.resolve({
            available_cores: 11.8,
            capacity_cores: 60,
            used_cores: 48.2,
          }),
        ok: true,
      });
    }
    if (url === "/api/metrics/cluster-pods") {
      return Promise.resolve({
        json: () => Promise.resolve(mockClusterPodsResponse),
        ok: true,
      });
    }
    if (url === "/api/metrics/cluster-nodes") {
      return Promise.resolve({
        json: () =>
          Promise.resolve({
            not_ready_nodes: 0,
            ready_nodes: 8,
            total_nodes: 8,
          }),
        ok: true,
      });
    }
    if (url === "/api/metrics/gateway-provision-duration") {
      if (options.omitProvisionDuration) {
        return Promise.resolve({
          ok: false,
          status: 502,
        });
      }
      return Promise.resolve({
        json: () => Promise.resolve(mockGatewayProvisionDurationResponse),
        ok: true,
      });
    }
    if (url === "/api/metrics/gateway-provision-outcomes") {
      if (options.omitProvisionOutcomes) {
        return Promise.resolve({
          ok: false,
          status: 502,
        });
      }
      return Promise.resolve({
        json: () =>
          Promise.resolve(
            options.provisionOutcomes ?? mockGatewayProvisionOutcomesResponse,
          ),
        ok: true,
      });
    }
    return Promise.reject(new Error(`unexpected fetch url: ${url}`));
  });
}

function resolveStandardPrometheusSupportRoutes(
  url: string,
  options: DashboardMetricsMockOptions = {},
) {
  const activeSandboxes = options.activeSandboxes ?? 0;
  const registeredUsers =
    options.registeredUsers ?? defaultRegisteredUsersResponse(0);
  const platformInventory =
    options.platformInventory ?? defaultPlatformInventory();

  if (url === "/api/metrics/gateway-sandboxes") {
    return Promise.resolve({
      json: () => Promise.resolve({ active_sandboxes: activeSandboxes }),
      ok: true,
    });
  }
  if (url === "/api/metrics/registered-users") {
    return Promise.resolve({
      json: () => Promise.resolve(registeredUsers),
      ok: true,
    });
  }
  if (url === "/api/metrics/platform-inventory") {
    return Promise.resolve({
      json: () => Promise.resolve(platformInventory),
      ok: true,
    });
  }
  if (url === "/api/metrics/gateway-provision-duration") {
    if (options.omitProvisionDuration) {
      return Promise.resolve({
        ok: false,
        status: 502,
      });
    }
    return Promise.resolve({
      json: () => Promise.resolve(mockGatewayProvisionDurationResponse),
      ok: true,
    });
  }
  if (url === "/api/metrics/gateway-provision-outcomes") {
    if (options.omitProvisionOutcomes) {
      return Promise.resolve({
        ok: false,
        status: 502,
      });
    }
    return Promise.resolve({
      json: () =>
        Promise.resolve(
          options.provisionOutcomes ?? mockGatewayProvisionOutcomesResponse,
        ),
      ok: true,
    });
  }
  return undefined;
}

beforeEach(() => {
  vi.stubGlobal("fetch", fetchMock);
  fetchMock.mockReset();
  gatewaysListMock.mockReset();
  gatewayReleasesListMock.mockReset();
  gatewaysListMock.mockResolvedValue({
    items: [
      { release_id: "rel-1" },
      { release_id: "rel-1" },
      { release_id: "rel-2" },
      { release_id: "" },
    ],
    page: 1,
    size: 100,
    total: 4,
  });
  gatewayReleasesListMock.mockResolvedValue({
    items: [
      { id: "rel-1", name: "OpenShell 2.0" },
      { id: "rel-2", name: "OpenShell 2.1" },
    ],
    page: 1,
    size: 100,
    total: 2,
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function defaultRegisteredUsersResponse(total = 0) {
  return { total_registered: total };
}

describe("createDashboardControlPlaneAdapter", () => {
  it("aggregates Prometheus dashboard metrics into operational metrics", async () => {
    mockClusterMetricsResponses(
      254468212736,
      236223201280,
      { Provisioning: 50, Running: 100 },
      {
        activeSandboxes: 200,
        registeredUsers: { total_registered: 42 },
      },
    );

    const metrics = await adapter.getOperationalMetrics(context);

    const gatewaysMetric = metrics.metrics.find(
      (metric) => metric.id === "provisioned-gateways",
    );
    const sandboxesMetric = metrics.metrics.find(
      (metric) => metric.id === "provisioned-sandboxes",
    );
    const registeredUsersMetric = metrics.metrics.find(
      (metric) => metric.id === "registered-users",
    );
    const memoryMetric = metrics.metrics.find(
      (metric) => metric.id === "memory",
    );
    const cpuMetric = metrics.metrics.find((metric) => metric.id === "cpu");
    const podsMetric = metrics.metrics.find((metric) => metric.id === "pods");
    const nodesMetric = metrics.metrics.find((metric) => metric.id === "nodes");
    const provisionTimeMetric = metrics.metrics.find(
      (metric) => metric.id === "provision-time",
    );
    const provisionReliabilityMetric = metrics.metrics.find(
      (metric) => metric.id === "provision-reliability",
    );

    expect(gatewaysMetric?.value).toBe("150");
    expect(gatewaysMetric?.status).toEqual({
      degraded: 0,
      failed: 0,
      healthy: 100,
      provisioning: 50,
    });
    expect(sandboxesMetric?.value).toBe("200");
    expect(registeredUsersMetric).toEqual({
      id: "registered-users",
      value: "42",
    });
    expect(memoryMetric).toEqual({
      id: "memory",
      total: "237",
      unit: "GiB",
      value: "220",
    });
    expect(cpuMetric).toEqual({
      id: "cpu",
      total: "60",
      unit: "cores",
      value: "48",
    });
    expect(podsMetric).toEqual({
      id: "pods",
      podPhases: {
        failed: 16,
        pending: 12,
        running: 500,
        succeeded: 20,
        unknown: 0,
      },
      total: "2000",
      unit: "pods",
      value: "548",
    });
    expect(nodesMetric).toEqual({
      id: "nodes",
      status: {
        failed: 0,
        healthy: 8,
      },
      value: "8",
    });
    expect(provisionTimeMetric).toEqual({
      id: "provision-time",
      provisionDuration: {
        mean: "315.00",
        p50: "288.00",
        p95: "726.00",
      },
      unit: "sec",
      value: "315.00",
    });
    expect(provisionReliabilityMetric).toEqual({
      id: "provision-reliability",
      provisionOutcomes: {
        failureCount24h: "1",
        successCount24h: "9",
        successRatePercent: "90.0",
      },
      successRateTrend: {
        points: [
          { label: "2026-08-09T12:00", value: 80 },
          { label: "2026-08-09T13:00", value: 100 },
        ],
      },
      value: "90.0",
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/cluster-memory", {
      credentials: "same-origin",
      signal: undefined,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/cluster-cpu", {
      credentials: "same-origin",
      signal: undefined,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/cluster-pods", {
      credentials: "same-origin",
      signal: undefined,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/cluster-nodes", {
      credentials: "same-origin",
      signal: undefined,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/gateways", {
      credentials: "same-origin",
      signal: undefined,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/gateway-sandboxes", {
      credentials: "same-origin",
      signal: undefined,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/registered-users", {
      credentials: "same-origin",
      signal: undefined,
    });
  });

  it("maps Prometheus gateway phase counts into display-status buckets", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2, {
      Running: 1,
      Degraded: 1,
      Failed: 1,
    });

    const metrics = await adapter.getOperationalMetrics(context);
    const gatewaysMetric = metrics.metrics.find(
      (metric) => metric.id === "provisioned-gateways",
    );

    expect(gatewaysMetric?.value).toBe("3");
    expect(gatewaysMetric?.status).toEqual({
      degraded: 1,
      failed: 1,
      healthy: 1,
      provisioning: 0,
    });
  });

  it("loads sandbox totals from Prometheus", async () => {
    mockClusterMetricsResponses(
      1024 ** 3,
      512 * 1024 ** 2,
      defaultGatewayPhaseCounts,
      {
        activeSandboxes: 5,
      },
    );

    const metrics = await adapter.getOperationalMetrics(context);
    const sandboxesMetric = metrics.metrics.find(
      (metric) => metric.id === "provisioned-sandboxes",
    );

    expect(sandboxesMetric?.value).toBe("5");
  });

  it("maps gateway provision duration histogram into average, P50, and P95 seconds", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);

    const metrics = await adapter.getOperationalMetrics(context);
    const provisionTimeMetric = metrics.metrics.find(
      (metric) => metric.id === "provision-time",
    );

    expect(provisionTimeMetric).toEqual({
      id: "provision-time",
      provisionDuration: {
        mean: "315.00",
        p50: "288.00",
        p95: "726.00",
      },
      unit: "sec",
      value: "315.00",
    });
  });

  it("maps gateway provision outcomes into provision reliability", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);

    const metrics = await adapter.getOperationalMetrics(context);
    const provisionReliabilityMetric = metrics.metrics.find(
      (metric) => metric.id === "provision-reliability",
    );

    expect(provisionReliabilityMetric).toEqual({
      id: "provision-reliability",
      provisionOutcomes: {
        failureCount24h: "1",
        successCount24h: "9",
        successRatePercent: "90.0",
      },
      successRateTrend: {
        points: [
          { label: "2026-08-09T12:00", value: 80 },
          { label: "2026-08-09T13:00", value: 100 },
        ],
      },
      value: "90.0",
    });
  });

  it("omits provision reliability when success rate is null", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2, undefined, {
      provisionOutcomes: {
        failure_count_24h: 0,
        hourly_success_rate: [],
        success_count_24h: 0,
        success_rate_percent: null,
      },
    });

    const metrics = await adapter.getOperationalMetrics(context);

    expect(
      metrics.metrics.find((metric) => metric.id === "provision-reliability"),
    ).toBeUndefined();
    expect(
      metrics.metrics.find((metric) => metric.id === "provision-time"),
    ).toBeDefined();
  });

  it("omits provision time when the BFF provision duration route is unavailable", async () => {
    mockClusterMetricsResponses(
      1024 ** 3,
      512 * 1024 ** 2,
      defaultGatewayPhaseCounts,
      {
        omitProvisionDuration: true,
      },
    );

    const metrics = await adapter.getOperationalMetrics(context);
    const provisionTimeMetric = metrics.metrics.find(
      (metric) => metric.id === "provision-time",
    );
    const memoryMetric = metrics.metrics.find(
      (metric) => metric.id === "memory",
    );

    expect(provisionTimeMetric).toBeUndefined();
    expect(
      metrics.metrics.find((metric) => metric.id === "provision-reliability"),
    ).toBeDefined();
    expect(memoryMetric).toEqual({
      id: "memory",
      total: "1",
      unit: "GiB",
      value: "1",
    });
    expect(
      metrics.metrics.find((metric) => metric.id === "provisioned-gateways"),
    ).toBeDefined();
  });

  it("omits provision reliability when the BFF provision outcomes route is unavailable", async () => {
    mockClusterMetricsResponses(
      1024 ** 3,
      512 * 1024 ** 2,
      defaultGatewayPhaseCounts,
      {
        omitProvisionOutcomes: true,
      },
    );

    const metrics = await adapter.getOperationalMetrics(context);

    expect(
      metrics.metrics.find((metric) => metric.id === "provision-reliability"),
    ).toBeUndefined();
    expect(
      metrics.metrics.find((metric) => metric.id === "provision-time"),
    ).toBeDefined();
  });

  it("omits gateway metrics when the gateway-sandboxes route fails", async () => {
    fetchMock.mockImplementation((url: string) => {
      const base = {
        activeSandboxes: 0,
        registeredUsers: defaultRegisteredUsersResponse(0),
        platformInventory: defaultPlatformInventory(),
      };
      if (url === "/api/metrics/gateway-sandboxes") {
        return Promise.resolve({ ok: false, status: 502 });
      }
      if (url === "/api/metrics/gateways") {
        return Promise.resolve({
          json: () => Promise.resolve({ counts: defaultGatewayPhaseCounts }),
          ok: true,
        });
      }
      if (url === "/api/metrics/registered-users") {
        return Promise.resolve({
          json: () => Promise.resolve(base.registeredUsers),
          ok: true,
        });
      }
      if (url === "/api/metrics/platform-inventory") {
        return Promise.resolve({
          json: () => Promise.resolve(base.platformInventory),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-memory") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              available_bytes: 512 * 1024 ** 2,
              capacity_bytes: 1024 ** 3,
              used_bytes: 512 * 1024 ** 2,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-cpu") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              available_cores: 11.8,
              capacity_cores: 60,
              used_cores: 48.2,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-pods") {
        return Promise.resolve({
          json: () => Promise.resolve(mockClusterPodsResponse),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-nodes") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              not_ready_nodes: 0,
              ready_nodes: 8,
              total_nodes: 8,
            }),
          ok: true,
        });
      }
      const support = resolveStandardPrometheusSupportRoutes(url);
      if (support !== undefined) {
        return support;
      }
      return Promise.reject(new Error(`unexpected fetch url: ${url}`));
    });

    const metrics = await adapter.getOperationalMetrics(context);

    expect(metrics.failedSources).toEqual(["gateway-metrics"]);
    expect(
      metrics.metrics.find((metric) => metric.id === "provisioned-sandboxes"),
    ).toBeUndefined();
    expect(
      metrics.metrics.find((metric) => metric.id === "provisioned-gateways"),
    ).toBeUndefined();
    expect(
      metrics.metrics.find((metric) => metric.id === "memory"),
    ).toBeDefined();
  });

  it("forwards abort signals to Prometheus metric fetches", async () => {
    const controller = new AbortController();
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);

    await adapter.getOperationalMetrics({
      ...context,
      signal: controller.signal,
    });

    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/cluster-memory", {
      credentials: "same-origin",
      signal: controller.signal,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/cluster-cpu", {
      credentials: "same-origin",
      signal: controller.signal,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/cluster-pods", {
      credentials: "same-origin",
      signal: controller.signal,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/cluster-nodes", {
      credentials: "same-origin",
      signal: controller.signal,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/gateways", {
      credentials: "same-origin",
      signal: controller.signal,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/gateway-sandboxes", {
      credentials: "same-origin",
      signal: controller.signal,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/registered-users", {
      credentials: "same-origin",
      signal: controller.signal,
    });
    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/platform-inventory", {
      credentials: "same-origin",
      signal: controller.signal,
    });
  });

  it("omits memory metrics when cluster memory is unavailable", async () => {
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/gateways") {
        return Promise.resolve({
          json: () => Promise.resolve({ counts: defaultGatewayPhaseCounts }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-memory") {
        return Promise.resolve({
          ok: false,
          status: 502,
        });
      }
      if (url === "/api/metrics/cluster-cpu") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              available_cores: 11.8,
              capacity_cores: 60,
              used_cores: 48.2,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-pods") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              ...mockClusterPodsResponse,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-nodes") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              not_ready_nodes: 0,
              ready_nodes: 8,
              total_nodes: 8,
            }),
          ok: true,
        });
      }
      const support = resolveStandardPrometheusSupportRoutes(url);
      if (support !== undefined) {
        return support;
      }
      return Promise.reject(new Error(`unexpected fetch url: ${url}`));
    });
    const metrics = await adapter.getOperationalMetrics(context);

    expect(metrics.failedSources).toEqual(["cluster-memory"]);
    expect(
      metrics.metrics.find((metric) => metric.id === "memory"),
    ).toBeUndefined();
    expect(
      metrics.metrics.find((metric) => metric.id === "provisioned-gateways"),
    ).toBeDefined();
  });

  it("omits CPU metrics when cluster CPU is unavailable", async () => {
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/gateways") {
        return Promise.resolve({
          json: () => Promise.resolve({ counts: defaultGatewayPhaseCounts }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-memory") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              available_bytes: 512 * 1024 ** 2,
              capacity_bytes: 1024 ** 3,
              used_bytes: 512 * 1024 ** 2,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-cpu") {
        return Promise.resolve({
          ok: false,
          status: 502,
        });
      }
      if (url === "/api/metrics/cluster-pods") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              ...mockClusterPodsResponse,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-nodes") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              not_ready_nodes: 0,
              ready_nodes: 8,
              total_nodes: 8,
            }),
          ok: true,
        });
      }
      const support = resolveStandardPrometheusSupportRoutes(url);
      if (support !== undefined) {
        return support;
      }
      return Promise.reject(new Error(`unexpected fetch url: ${url}`));
    });
    const metrics = await adapter.getOperationalMetrics(context);

    expect(metrics.failedSources).toEqual(["cluster-cpu"]);
    expect(
      metrics.metrics.find((metric) => metric.id === "cpu"),
    ).toBeUndefined();
    expect(
      metrics.metrics.find((metric) => metric.id === "provisioned-gateways"),
    ).toBeDefined();
  });

  it("omits pod metrics when cluster pods are unavailable", async () => {
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/gateways") {
        return Promise.resolve({
          json: () => Promise.resolve({ counts: defaultGatewayPhaseCounts }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-memory") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              available_bytes: 512 * 1024 ** 2,
              capacity_bytes: 1024 ** 3,
              used_bytes: 512 * 1024 ** 2,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-cpu") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              available_cores: 11.8,
              capacity_cores: 60,
              used_cores: 48.2,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-pods") {
        return Promise.resolve({
          ok: false,
          status: 502,
        });
      }
      if (url === "/api/metrics/cluster-nodes") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              not_ready_nodes: 0,
              ready_nodes: 8,
              total_nodes: 8,
            }),
          ok: true,
        });
      }
      const support = resolveStandardPrometheusSupportRoutes(url);
      if (support !== undefined) {
        return support;
      }
      return Promise.reject(new Error(`unexpected fetch url: ${url}`));
    });
    const metrics = await adapter.getOperationalMetrics(context);

    expect(metrics.failedSources).toEqual(["cluster-pods"]);
    expect(
      metrics.metrics.find((metric) => metric.id === "pods"),
    ).toBeUndefined();
    expect(
      metrics.metrics.find((metric) => metric.id === "provisioned-gateways"),
    ).toBeDefined();
  });

  it("omits node metrics when cluster nodes are unavailable", async () => {
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/gateways") {
        return Promise.resolve({
          json: () => Promise.resolve({ counts: defaultGatewayPhaseCounts }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-memory") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              available_bytes: 512 * 1024 ** 2,
              capacity_bytes: 1024 ** 3,
              used_bytes: 512 * 1024 ** 2,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-cpu") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              available_cores: 11.8,
              capacity_cores: 60,
              used_cores: 48.2,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-pods") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              ...mockClusterPodsResponse,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-nodes") {
        return Promise.resolve({
          ok: false,
          status: 502,
        });
      }
      const support = resolveStandardPrometheusSupportRoutes(url);
      if (support !== undefined) {
        return support;
      }
      return Promise.reject(new Error(`unexpected fetch url: ${url}`));
    });
    const metrics = await adapter.getOperationalMetrics(context);

    expect(metrics.failedSources).toEqual(["cluster-nodes"]);
    expect(
      metrics.metrics.find((metric) => metric.id === "nodes"),
    ).toBeUndefined();
    expect(
      metrics.metrics.find((metric) => metric.id === "provisioned-gateways"),
    ).toBeDefined();
  });

  it("fails when every metric source is unavailable", async () => {
    fetchMock.mockRejectedValue(new Error("network down"));
    gatewaysListMock.mockRejectedValue(new Error("network down"));
    gatewayReleasesListMock.mockRejectedValue(new Error("network down"));

    await expect(adapter.getOperationalMetrics(context)).rejects.toThrow(
      "All operational dashboard metric sources failed",
    );
  });

  it("aggregates managed cluster inventory into operational metrics", async () => {
    mockClusterMetricsResponses(
      1024 ** 3,
      512 * 1024 ** 2,
      defaultGatewayPhaseCounts,
      {
        platformInventory: {
          managed_clusters: {
            by_provider: {
              aws: 5,
              gcp: 2,
              ibm: 93,
              openshift: 50,
            },
            by_region: {
              "eu-west-1 (aws)": 1,
              "eu-west-1 (gcp)": 2,
              "eu-west-1 (ibm)": 93,
              "unknown (openshift)": 50,
              "us-east-1 (aws)": 4,
            },
            by_status: {
              Failed: 50,
              Ready: 99,
              unknown: 1,
            },
            created_last_30_days: 2,
            total: 150,
          },
        },
      },
    );

    const metrics = await adapter.getOperationalMetrics(context);

    const clustersMetric = metrics.metrics.find(
      (metric) => metric.id === "managed-clusters",
    );

    expect(clustersMetric).toEqual({
      createdLast30Days: "2",
      id: "managed-clusters",
      inventoryProviders: {
        aws: 5,
        gcp: 2,
        ibm: 93,
        openshift: 50,
      },
      inventoryRegions: {
        "eu-west-1 (aws)": 1,
        "eu-west-1 (gcp)": 2,
        "eu-west-1 (ibm)": 93,
        "unknown (openshift)": 50,
        "us-east-1 (aws)": 4,
      },
      inventoryStatus: {
        Failed: 50,
        Ready: 99,
        unknown: 1,
      },
      value: "150",
    });
  });

  it("omits platform inventory metrics when the platform-inventory route fails", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/platform-inventory") {
        return Promise.resolve({ ok: false, status: 502 });
      }
      if (url === "/api/metrics/gateways") {
        return Promise.resolve({
          json: () => Promise.resolve({ counts: defaultGatewayPhaseCounts }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-memory") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              available_bytes: 512 * 1024 ** 2,
              capacity_bytes: 1024 ** 3,
              used_bytes: 512 * 1024 ** 2,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-cpu") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              available_cores: 11.8,
              capacity_cores: 60,
              used_cores: 48.2,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-pods") {
        return Promise.resolve({
          json: () => Promise.resolve(mockClusterPodsResponse),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-nodes") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              not_ready_nodes: 0,
              ready_nodes: 8,
              total_nodes: 8,
            }),
          ok: true,
        });
      }
      const support = resolveStandardPrometheusSupportRoutes(url);
      if (support !== undefined) {
        return support;
      }
      return Promise.reject(new Error(`unexpected fetch url: ${url}`));
    });

    const metrics = await adapter.getOperationalMetrics(context);

    expect(metrics.failedSources).toEqual(["platform-inventory"]);
    expect(
      metrics.metrics.find((metric) => metric.id === "managed-clusters"),
    ).toBeUndefined();
    expect(
      metrics.metrics.find((metric) => metric.id === "memory"),
    ).toBeDefined();
  });

  it("forwards abort signals to the platform-inventory route", async () => {
    const controller = new AbortController();
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);

    await adapter.getOperationalMetrics({
      ...context,
      signal: controller.signal,
    });

    expect(fetchMock).toHaveBeenCalledWith("/api/metrics/platform-inventory", {
      credentials: "same-origin",
      signal: controller.signal,
    });
  });

  it("maps registered user adoption fields into operational metrics", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2, undefined, {
      registeredUsers: {
        created_last_7_days: 12,
        created_last_30_days: 48,
        daily_unique_logins: [
          { count: 5, date: "2026-09-01" },
          { count: 7, date: "2026-09-02" },
        ],
        total_registered: 450,
        unique_logins_last_7_days: 186,
        unique_logins_last_30_days: 312,
      },
    });

    const metrics = await adapter.getOperationalMetrics(context);
    const registeredUsersMetric = metrics.metrics.find(
      (metric) => metric.id === "registered-users",
    );

    expect(registeredUsersMetric).toEqual({
      createdLast7Days: "12",
      createdLast30Days: "48",
      id: "registered-users",
      trend: {
        points: [
          { label: "2026-09-01", value: 5 },
          { label: "2026-09-02", value: 7 },
        ],
      },
      uniqueLoginsLast7Days: "186",
      uniqueLoginsLast30Days: "312",
      value: "450",
    });
  });

  it("aggregates gateway release distribution into gateway-releases metric", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);

    const metrics = await adapter.getOperationalMetrics(context);
    const gatewayReleasesMetric = metrics.metrics.find(
      (metric) => metric.id === "gateway-releases",
    );

    expect(gatewayReleasesMetric).toEqual({
      id: "gateway-releases",
      releaseDistribution: {
        "OpenShell 2.0": 2,
        "OpenShell 2.1": 1,
        unknown: 1,
      },
      value: "4",
    });
    expect(gatewaysListMock).toHaveBeenCalledWith(
      { orderBy: "name asc", page: 1, size: 100 },
      { signal: undefined },
    );
    expect(gatewayReleasesListMock).toHaveBeenCalledWith(
      { orderBy: "name asc", page: 1, size: 100 },
      { signal: undefined },
    );
  });

  it("omits gateway-releases when gateway list aggregation fails", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);
    gatewaysListMock.mockRejectedValue(new Error("gateway list failed"));

    const metrics = await adapter.getOperationalMetrics(context);

    expect(metrics.failedSources).toEqual(["gateway-release-distribution"]);
    expect(
      metrics.metrics.find((metric) => metric.id === "gateway-releases"),
    ).toBeUndefined();
    expect(
      metrics.metrics.find((metric) => metric.id === "provisioned-gateways"),
    ).toBeDefined();
  });

  it("maps gateway daily_fleet_totals into provisioned-gateways trend", async () => {
    mockClusterMetricsResponses(
      1024 ** 3,
      512 * 1024 ** 2,
      {},
      { activeSandboxes: 4 },
    );
    const baseFetch = getDashboardFetchMock();
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/gateways") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              counts: { Running: 5 },
              daily_fleet_totals: [
                { date: "2026-09-01", total: 18 },
                { date: "2026-09-02", total: 20 },
              ],
            }),
          ok: true,
        });
      }
      return delegateDashboardFetch(baseFetch, url);
    });

    const metrics = await adapter.getOperationalMetrics(context);
    const gatewayMetric = metrics.metrics.find(
      (metric) => metric.id === "provisioned-gateways",
    );

    expect(gatewayMetric?.trend).toEqual({
      points: [
        { label: "2026-09-01", value: 18 },
        { label: "2026-09-02", value: 20 },
      ],
    });
  });

  it("omits provisioned-gateways trend when daily_fleet_totals is absent", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);
    const baseFetch = getDashboardFetchMock();
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/gateways") {
        return Promise.resolve({
          json: () => Promise.resolve({ counts: { Running: 5 } }),
          ok: true,
        });
      }
      return delegateDashboardFetch(baseFetch, url);
    });

    const metrics = await adapter.getOperationalMetrics(context);
    const gatewayMetric = metrics.metrics.find(
      (metric) => metric.id === "provisioned-gateways",
    );

    expect(gatewayMetric?.value).toBe("5");
    expect(gatewayMetric?.trend).toBeUndefined();
  });

  it("maps sandbox hourly and daily trends onto provisioned-sandboxes", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);
    const baseFetch = getDashboardFetchMock();
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/gateway-sandboxes") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              active_sandboxes: 4,
              hourly_active_sandboxes: [{ hour: "2026-09-15T12:00", count: 8 }],
              daily_active_sandboxes: [{ date: "2026-09-01", count: 5 }],
            }),
          ok: true,
        });
      }
      return delegateDashboardFetch(baseFetch, url);
    });

    const metrics = await adapter.getOperationalMetrics(context);
    const sandboxMetric = metrics.metrics.find(
      (metric) => metric.id === "provisioned-sandboxes",
    );

    expect(sandboxMetric).toEqual({
      hourlyTrend: {
        points: [{ label: "2026-09-15T12:00", value: 8 }],
      },
      id: "provisioned-sandboxes",
      trend: {
        points: [{ label: "2026-09-01", value: 5 }],
      },
      value: "4",
    });
  });

  it("maps sandbox attention counts onto provisioned-sandboxes", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);
    const baseFetch = getDashboardFetchMock();
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/gateway-sandboxes") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              active_sandboxes: 214,
              orphaned_sandboxes: 3,
              expiring_sandboxes: 12,
              idle_sandboxes: 7,
            }),
          ok: true,
        });
      }
      return delegateDashboardFetch(baseFetch, url);
    });

    const metrics = await adapter.getOperationalMetrics(context);
    const sandboxMetric = metrics.metrics.find(
      (metric) => metric.id === "provisioned-sandboxes",
    );

    expect(sandboxMetric).toEqual({
      expiringSandboxes: 12,
      id: "provisioned-sandboxes",
      idleSandboxes: 7,
      orphanedSandboxes: 3,
      value: "214",
    });
  });

  it("omits absent sandbox attention fields without inventing zeros", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);
    const baseFetch = getDashboardFetchMock();
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/gateway-sandboxes") {
        return Promise.resolve({
          json: () => Promise.resolve({ active_sandboxes: 214 }),
          ok: true,
        });
      }
      return delegateDashboardFetch(baseFetch, url);
    });

    const metrics = await adapter.getOperationalMetrics(context);
    const sandboxMetric = metrics.metrics.find(
      (metric) => metric.id === "provisioned-sandboxes",
    );

    expect(sandboxMetric).toEqual({
      id: "provisioned-sandboxes",
      value: "214",
    });
    expect(sandboxMetric).not.toHaveProperty("orphanedSandboxes");
    expect(sandboxMetric).not.toHaveProperty("expiringSandboxes");
    expect(sandboxMetric).not.toHaveProperty("idleSandboxes");
  });

  it("maps cluster daily_used into memory, cpu, and pods trends", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);
    const baseFetch = getDashboardFetchMock();
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/cluster-memory") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              available_bytes: 17 * 1024 ** 3,
              capacity_bytes: 237 * 1024 ** 3,
              daily_used: [{ date: "2026-09-01", value: 218 }],
              used_bytes: 220 * 1024 ** 3,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-cpu") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              available_cores: 12,
              capacity_cores: 60,
              daily_used: [{ date: "2026-09-01", value: 49 }],
              used_cores: 48,
            }),
          ok: true,
        });
      }
      if (url === "/api/metrics/cluster-pods") {
        return Promise.resolve({
          json: () =>
            Promise.resolve({
              ...mockClusterPodsResponse,
              daily_used: [{ date: "2026-09-01", value: 548 }],
            }),
          ok: true,
        });
      }
      return delegateDashboardFetch(baseFetch, url);
    });

    const metrics = await adapter.getOperationalMetrics(context);

    expect(
      metrics.metrics.find((metric) => metric.id === "memory")?.trend,
    ).toEqual({
      points: [{ label: "2026-09-01", value: 218 }],
    });
    expect(
      metrics.metrics.find((metric) => metric.id === "cpu")?.trend,
    ).toEqual({
      points: [{ label: "2026-09-01", value: 49 }],
    });
    expect(
      metrics.metrics.find((metric) => metric.id === "pods")?.trend,
    ).toEqual({
      points: [{ label: "2026-09-01", value: 548 }],
    });
  });

  it("omits memory, cpu, and pods trends when daily_used is absent", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);

    const metrics = await adapter.getOperationalMetrics(context);

    expect(
      metrics.metrics.find((metric) => metric.id === "memory")?.trend,
    ).toBeUndefined();
    expect(
      metrics.metrics.find((metric) => metric.id === "cpu")?.trend,
    ).toBeUndefined();
    expect(
      metrics.metrics.find((metric) => metric.id === "pods")?.trend,
    ).toBeUndefined();
    expect(metrics.metrics.find((metric) => metric.id === "memory")).toEqual({
      id: "memory",
      total: "1",
      unit: "GiB",
      value: "1",
    });
  });

  it("omits optional registered user adoption fields when degraded", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2, undefined, {
      registeredUsers: {
        created_last_30_days: 12,
        total_registered: 450,
      },
    });

    const metrics = await adapter.getOperationalMetrics(context);
    const registeredUsersMetric = metrics.metrics.find(
      (metric) => metric.id === "registered-users",
    );

    expect(registeredUsersMetric).toEqual({
      createdLast30Days: "12",
      id: "registered-users",
      value: "450",
    });
  });

  describe("getReliabilityMetrics", () => {
    const hourlyRequestRate = [
      { hour: "2026-09-15T12:00", value: 10.2 },
      { hour: "2026-09-15T13:00", value: 11.1 },
    ];
    const hourlyErrorRate = [
      { hour: "2026-09-15T12:00", value: 0.5 },
      { hour: "2026-09-15T13:00", value: 0.8 },
    ];
    const hourlyLatency = [
      { hour: "2026-09-15T12:00", value: 0.09 },
      { hour: "2026-09-15T13:00", value: 0.085 },
    ];

    it("maps a full API reliability payload into three metrics with hourly trends", async () => {
      fetchMock.mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({
            error_rate_percent: 1.25,
            hourly_error_rate_percent: hourlyErrorRate,
            hourly_latency_p50_seconds: hourlyLatency,
            hourly_request_rate: hourlyRequestRate,
            latency_p50_seconds: 0.084,
            request_rate: 12.5,
          }),
      });

      const metrics = await adapter.getReliabilityMetrics(context);

      expect(fetchMock).toHaveBeenCalledWith("/api/metrics/api-reliability", {
        credentials: "same-origin",
        signal: undefined,
      });
      expect(metrics.metrics).toEqual([
        {
          hourlyTrend: {
            points: [
              { label: "2026-09-15T12:00", value: 10.2 },
              { label: "2026-09-15T13:00", value: 11.1 },
            ],
          },
          id: "api-request-rate",
          unit: "req/s",
          value: "12.50",
        },
        {
          hourlyTrend: {
            points: [
              { label: "2026-09-15T12:00", value: 0.5 },
              { label: "2026-09-15T13:00", value: 0.8 },
            ],
          },
          id: "api-error-rate",
          unit: "%",
          value: "1.25",
        },
        {
          hourlyTrend: {
            points: [
              { label: "2026-09-15T12:00", value: 0.09 },
              { label: "2026-09-15T13:00", value: 0.085 },
            ],
          },
          id: "api-latency",
          unit: "sec",
          value: "0.084",
        },
      ]);
    });

    it("omits only the missing hourly trend when a historical series is absent", async () => {
      fetchMock.mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({
            error_rate_percent: 1.25,
            hourly_error_rate_percent: hourlyErrorRate,
            hourly_request_rate: hourlyRequestRate,
            latency_p50_seconds: 0.084,
            request_rate: 12.5,
          }),
      });

      const metrics = await adapter.getReliabilityMetrics(context);
      const latencyMetric = metrics.metrics.find(
        (metric) => metric.id === "api-latency",
      );
      const requestRateMetric = metrics.metrics.find(
        (metric) => metric.id === "api-request-rate",
      );

      expect(latencyMetric).toEqual({
        id: "api-latency",
        unit: "sec",
        value: "0.084",
      });
      expect(requestRateMetric?.hourlyTrend).toEqual({
        points: [
          { label: "2026-09-15T12:00", value: 10.2 },
          { label: "2026-09-15T13:00", value: 11.1 },
        ],
      });
    });

    it("returns failedSources when the BFF response is not ok", async () => {
      fetchMock.mockResolvedValue({
        ok: false,
        status: 502,
      });

      const metrics = await adapter.getReliabilityMetrics(context);

      expect(metrics.failedSources).toEqual(["api-reliability"]);
      expect(metrics.metrics).toEqual([]);
      expect(metrics.lastSuccessfulRefresh).toBeInstanceOf(Date);
    });

    it("returns failedSources when required instant fields are missing", async () => {
      fetchMock.mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({
            error_rate_percent: 1.25,
            request_rate: 12.5,
          }),
      });

      const metrics = await adapter.getReliabilityMetrics(context);

      expect(metrics.failedSources).toEqual(["api-reliability"]);
      expect(metrics.metrics).toEqual([]);
      expect(metrics.lastSuccessfulRefresh).toBeInstanceOf(Date);
    });

    it("rethrows abort errors instead of soft-failing", async () => {
      const abortError = new DOMException("Aborted", "AbortError");
      fetchMock.mockRejectedValue(abortError);

      await expect(adapter.getReliabilityMetrics(context)).rejects.toBe(
        abortError,
      );
    });

    it("forwards abort signals to the api-reliability route", async () => {
      const controller = new AbortController();
      fetchMock.mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({
            error_rate_percent: 0,
            latency_p50_seconds: 0.01,
            request_rate: 1,
          }),
      });

      await adapter.getReliabilityMetrics({
        ...context,
        signal: controller.signal,
      });

      expect(fetchMock).toHaveBeenCalledWith("/api/metrics/api-reliability", {
        credentials: "same-origin",
        signal: controller.signal,
      });
    });
  });
});
