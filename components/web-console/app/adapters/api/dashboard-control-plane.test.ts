import type { Gateway, GatewayList } from "@openshift-online/hypershell-sdk";
import type { SDKClient } from "@openshift-online/hypershell-sdk";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createDashboardControlPlaneAdapter } from "./dashboard-control-plane";

const gatewayListApi = vi.fn();
const usersListApi = vi.fn();
const fetchMock = vi.fn();
const apiFactory = vi.fn(
  () =>
    ({
      gateways: {
        list: gatewayListApi,
      },
      users: {
        list: usersListApi,
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

function mockClusterMetricsResponses(
  capacityBytes: number,
  usedBytes: number,
): void {
  fetchMock.mockImplementation((url: string) => {
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
      return Promise.resolve({
        json: () => Promise.resolve(mockGatewayProvisionDurationResponse),
        ok: true,
      });
    }
    return Promise.reject(new Error(`unexpected fetch url: ${url}`));
  });
}

beforeEach(() => {
  vi.stubGlobal("fetch", fetchMock);
  fetchMock.mockReset();
  gatewayListApi.mockReset();
  usersListApi.mockReset();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function gateway(overrides: Partial<Gateway> = {}): Gateway {
  const phase = overrides.phase ?? "Running";
  const runningTimestamps =
    phase === "Running"
      ? {
          created_at: "2026-08-01T10:00:00.000Z",
          updated_at: "2026-08-01T10:05:15.000Z",
        }
      : {
          created_at: null,
          updated_at: null,
        };

  return {
    active_sandbox_count: 2,
    cluster_id: "",
    console_address: "",
    created_by: "",
    credential_driver: "",
    database_id: "database-1",
    external_dns: "gateway.example.com",
    href: "/api/hypershell/v1/gateways/gateway-1",
    id: "gateway-1",
    image: "",
    kind: "Gateway",
    name: "Team gateway",
    namespace: "openshell",
    oidc: "",
    phase: "Running",
    release_id: "release-1",
    route: "",
    route_address: "",
    server_dns_names: "",
    service_type: "",
    status: "Healthy",
    supervisor_image: "",
    tls_mode: "",
    ...runningTimestamps,
    ...overrides,
  };
}

function gatewayList(
  items: Gateway[],
  total = items.length,
  page = 1,
): GatewayList {
  return {
    items,
    kind: "GatewayList",
    page,
    size: items.length,
    total,
  };
}

describe("createDashboardControlPlaneAdapter", () => {
  it("aggregates paginated gateway lists into operational metrics", async () => {
    mockClusterMetricsResponses(254468212736, 236223201280);

    usersListApi.mockResolvedValueOnce({
      items: [],
      kind: "UserList",
      page: 1,
      size: 1,
      total: 42,
    });

    const firstPage = Array.from({ length: 100 }, (_, index) =>
      gateway({
        active_sandbox_count: 1,
        id: `gateway-${String(index)}`,
        phase: "Running",
        status: "Healthy",
      }),
    );
    const secondPage = Array.from({ length: 50 }, (_, index) =>
      gateway({
        active_sandbox_count: 2,
        id: `gateway-${String(index + 100)}`,
        phase: "Provisioning",
        status: "route pending",
      }),
    );

    gatewayListApi
      .mockResolvedValueOnce(gatewayList(firstPage, 150, 1))
      .mockResolvedValueOnce(gatewayList(secondPage, 150, 2));

    const metrics = await adapter.getOperationalMetrics(context);

    expect(gatewayListApi).toHaveBeenCalledTimes(2);
    expect(gatewayListApi).toHaveBeenNthCalledWith(
      1,
      { orderBy: "name asc", page: 1, size: 100 },
      { signal: undefined },
    );
    expect(gatewayListApi).toHaveBeenNthCalledWith(
      2,
      { orderBy: "name asc", page: 2, size: 100 },
      { signal: undefined },
    );

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
    expect(registeredUsersMetric?.value).toBe("42");
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
    expect(usersListApi).toHaveBeenCalledWith(
      { orderBy: "username asc", page: 1, size: 1 },
      { signal: undefined },
    );
  });

  it("maps gateway lifecycle fields into display-status buckets", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);

    usersListApi.mockResolvedValueOnce({
      items: [],
      kind: "UserList",
      page: 1,
      size: 1,
      total: 0,
    });

    gatewayListApi.mockResolvedValueOnce(
      gatewayList(
        [
          gateway({ phase: "Running", status: "Healthy" }),
          gateway({ phase: "Degraded", status: "CrashLoopBackOff" }),
          gateway({ phase: "Failed", status: "apply error" }),
        ],
        3,
        1,
      ),
    );

    const metrics = await adapter.getOperationalMetrics(context);
    const gatewaysMetric = metrics.metrics.find(
      (metric) => metric.id === "provisioned-gateways",
    );

    expect(gatewaysMetric?.status).toEqual({
      degraded: 1,
      failed: 1,
      healthy: 1,
      provisioning: 0,
    });
  });

  it("treats omitted active_sandbox_count as zero when summing sandboxes", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);

    usersListApi.mockResolvedValueOnce({
      items: [],
      kind: "UserList",
      page: 1,
      size: 1,
      total: 0,
    });

    gatewayListApi.mockResolvedValueOnce(
      gatewayList(
        [
          gateway({ active_sandbox_count: 2 }),
          gateway({ active_sandbox_count: undefined }),
          gateway({ active_sandbox_count: 3 }),
        ],
        3,
        1,
      ),
    );

    const metrics = await adapter.getOperationalMetrics(context);
    const sandboxesMetric = metrics.metrics.find(
      (metric) => metric.id === "provisioned-sandboxes",
    );

    expect(sandboxesMetric?.value).toBe("5");
  });

  it("maps gateway provision duration histogram into average, P50, and P95 minutes", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);

    usersListApi.mockResolvedValueOnce({
      items: [],
      kind: "UserList",
      page: 1,
      size: 1,
      total: 0,
    });

    gatewayListApi.mockResolvedValueOnce(
      gatewayList(
        [gateway({ phase: "Provisioning", status: "route pending" })],
        1,
        1,
      ),
    );

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
    fetchMock.mockImplementation((url: string) => {
      if (url === "/api/metrics/gateway-provision-duration") {
        return Promise.resolve({
          ok: false,
          status: 502,
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
      return Promise.reject(new Error(`unexpected fetch url: ${url}`));
    });

    usersListApi.mockResolvedValueOnce({
      items: [],
      kind: "UserList",
      page: 1,
      size: 1,
      total: 0,
    });

    gatewayListApi.mockResolvedValueOnce(
      gatewayList(
        [
          gateway({ phase: "Provisioning", status: "route pending" }),
          gateway({ phase: "Failed", status: "apply error" }),
        ],
        2,
        1,
      ),
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
  });

  it("rejects inconsistent pagination responses", async () => {
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);

    usersListApi.mockResolvedValueOnce({
      items: [],
      kind: "UserList",
      page: 1,
      size: 1,
      total: 0,
    });
    gatewayListApi.mockResolvedValueOnce(gatewayList([gateway()], 1, 2));

    await expect(adapter.getOperationalMetrics(context)).rejects.toThrow(
      "Gateway list response was inconsistent",
    );
  });

  it("forwards abort signals to the gateway list client", async () => {
    const controller = new AbortController();
    mockClusterMetricsResponses(1024 ** 3, 512 * 1024 ** 2);

    usersListApi.mockResolvedValueOnce({
      items: [],
      kind: "UserList",
      page: 1,
      size: 1,
      total: 0,
    });
    gatewayListApi.mockResolvedValueOnce(gatewayList([gateway()], 1, 1));

    await adapter.getOperationalMetrics({
      ...context,
      signal: controller.signal,
    });

    expect(gatewayListApi).toHaveBeenCalledWith(
      { orderBy: "name asc", page: 1, size: 100 },
      { signal: controller.signal },
    );
    expect(usersListApi).toHaveBeenCalledWith(
      { orderBy: "username asc", page: 1, size: 1 },
      { signal: controller.signal },
    );
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
  });

  it("fails when cluster memory metrics are unavailable", async () => {
    fetchMock.mockImplementation((url: string) => {
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
      if (url === "/api/metrics/gateway-provision-duration") {
        return Promise.resolve({
          json: () => Promise.resolve(mockGatewayProvisionDurationResponse),
          ok: true,
        });
      }
      return Promise.reject(new Error(`unexpected fetch url: ${url}`));
    });
    usersListApi.mockResolvedValueOnce({
      items: [],
      kind: "UserList",
      page: 1,
      size: 1,
      total: 0,
    });
    gatewayListApi.mockResolvedValueOnce(gatewayList([gateway()], 1, 1));

    await expect(adapter.getOperationalMetrics(context)).rejects.toThrow(
      "Failed to fetch cluster memory metrics: 502",
    );
  });

  it("fails when cluster CPU metrics are unavailable", async () => {
    fetchMock.mockImplementation((url: string) => {
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
      if (url === "/api/metrics/gateway-provision-duration") {
        return Promise.resolve({
          json: () => Promise.resolve(mockGatewayProvisionDurationResponse),
          ok: true,
        });
      }
      return Promise.reject(new Error(`unexpected fetch url: ${url}`));
    });
    usersListApi.mockResolvedValueOnce({
      items: [],
      kind: "UserList",
      page: 1,
      size: 1,
      total: 0,
    });
    gatewayListApi.mockResolvedValueOnce(gatewayList([gateway()], 1, 1));

    await expect(adapter.getOperationalMetrics(context)).rejects.toThrow(
      "Failed to fetch cluster CPU metrics: 502",
    );
  });

  it("fails when cluster pods metrics are unavailable", async () => {
    fetchMock.mockImplementation((url: string) => {
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
      if (url === "/api/metrics/gateway-provision-duration") {
        return Promise.resolve({
          json: () => Promise.resolve(mockGatewayProvisionDurationResponse),
          ok: true,
        });
      }
      return Promise.reject(new Error(`unexpected fetch url: ${url}`));
    });
    usersListApi.mockResolvedValueOnce({
      items: [],
      kind: "UserList",
      page: 1,
      size: 1,
      total: 0,
    });
    gatewayListApi.mockResolvedValueOnce(gatewayList([gateway()], 1, 1));

    await expect(adapter.getOperationalMetrics(context)).rejects.toThrow(
      "Failed to fetch cluster pods metrics: 502",
    );
  });

  it("fails when cluster nodes metrics are unavailable", async () => {
    fetchMock.mockImplementation((url: string) => {
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
      return Promise.reject(new Error(`unexpected fetch url: ${url}`));
    });
    usersListApi.mockResolvedValueOnce({
      items: [],
      kind: "UserList",
      page: 1,
      size: 1,
      total: 0,
    });
    gatewayListApi.mockResolvedValueOnce(gatewayList([gateway()], 1, 1));

    await expect(adapter.getOperationalMetrics(context)).rejects.toThrow(
      "Failed to fetch cluster nodes metrics: 502",
    );
  });
});
