import type { SDKClient } from "@openshift-online/hypershell-sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createDashboardControlPlaneAdapter } from "./dashboard-control-plane";
import type { PlatformInventoryMetricsResponse } from "./platform-inventory-aggregation";

const fetchMock = vi.fn();
const apiFactory = vi.fn(() => ({}) as unknown as SDKClient);

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
    managed_databases: {
      by_status: {},
      total: 0,
    },
  };
}

interface DashboardMetricsMockOptions {
  activeSandboxes?: number;
  omitProvisionDuration?: boolean;
  platformInventory?: PlatformInventoryMetricsResponse;
  registeredUsers?: { total_registered: number };
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
  return undefined;
}

beforeEach(() => {
  vi.stubGlobal("fetch", fetchMock);
  fetchMock.mockReset();
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
        mean: "5.25",
        p50: "4.80",
        p95: "12.10",
      },
      unit: "minutes",
      value: "5.25",
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

  it("maps gateway provision duration histogram into average, P50, and P95 minutes", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);

    const metrics = await adapter.getOperationalMetrics(context);
    const provisionTimeMetric = metrics.metrics.find(
      (metric) => metric.id === "provision-time",
    );

    expect(provisionTimeMetric).toEqual({
      id: "provision-time",
      provisionDuration: {
        mean: "5.25",
        p50: "4.80",
        p95: "12.10",
      },
      unit: "minutes",
      value: "5.25",
    });
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

    await expect(adapter.getOperationalMetrics(context)).rejects.toThrow(
      "All operational dashboard metric sources failed",
    );
  });

  it("aggregates managed cluster and database inventory into operational metrics", async () => {
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
          managed_databases: {
            by_status: {
              Ready: 1,
              unknown: 1,
            },
            total: 2,
          },
        },
      },
    );

    const metrics = await adapter.getOperationalMetrics(context);

    const clustersMetric = metrics.metrics.find(
      (metric) => metric.id === "managed-clusters",
    );
    const databasesMetric = metrics.metrics.find(
      (metric) => metric.id === "managed-databases",
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
    expect(databasesMetric).toEqual({
      id: "managed-databases",
      inventoryStatus: {
        Ready: 1,
        unknown: 1,
      },
      value: "2",
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
      metrics.metrics.find((metric) => metric.id === "managed-databases"),
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
});
