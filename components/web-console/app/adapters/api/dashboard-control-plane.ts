import {
  fetchGatewayMetrics,
  gatewayPhaseCountsToDisplayStatusCounts,
  type GatewayDisplayStatusCounts,
} from "@openshift-online/hypershell-gateway-management-ui";
import type {
  DashboardControlPlane,
  DashboardInvocationContext,
  DashboardMetricSourceId,
  OperationalDashboardMetrics,
  OperationalMetric,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import type { SDKClient } from "@openshift-online/hypershell-sdk";

import {
  platformInventoryMetricsResponseToMetrics,
  type PlatformInventoryMetricsResponse,
} from "./platform-inventory-aggregation";

type DashboardApiFactory = (correlationId: string) => SDKClient;

const gibibyteDivisor = 1024 ** 3;
const secondsPerMinute = 60;

interface ClusterMemoryResponse {
  available_bytes: number;
  capacity_bytes: number;
  used_bytes: number;
}

interface ClusterCpuResponse {
  available_cores: number;
  capacity_cores: number;
  used_cores: number;
}

interface ClusterPodsResponse {
  available_pods: number;
  capacity_pods: number;
  phase_failed_pods: number;
  phase_pending_pods: number;
  phase_running_pods: number;
  phase_succeeded_pods: number;
  phase_unknown_pods: number;
  used_pods: number;
}

interface ClusterNodesResponse {
  not_ready_nodes: number;
  ready_nodes: number;
  total_nodes: number;
}

interface GatewayProvisionDurationResponse {
  mean_seconds: number;
  observation_count: number;
  p50_seconds: number;
  p95_seconds: number;
}

interface GatewaySandboxesResponse {
  active_sandboxes: number;
}

interface RegisteredUsersResponse {
  total_registered: number;
}

function bytesToRoundedGib(bytes: number): string {
  return String(Math.round(bytes / gibibyteDivisor));
}

function coresToRoundedString(cores: number): string {
  return String(Math.round(cores));
}

async function fetchClusterMemoryMetric(
  signal?: AbortSignal,
): Promise<OperationalMetric> {
  const response = await fetch("/api/metrics/cluster-memory", {
    credentials: "same-origin",
    signal,
  });
  if (!response.ok) {
    throw new Error(
      `Failed to fetch cluster memory metrics: ${String(response.status)}`,
    );
  }

  const body = (await response.json()) as ClusterMemoryResponse;

  return {
    id: "memory",
    total: bytesToRoundedGib(body.capacity_bytes),
    unit: "GiB",
    value: bytesToRoundedGib(body.used_bytes),
  };
}

async function fetchClusterCpuMetric(
  signal?: AbortSignal,
): Promise<OperationalMetric> {
  const response = await fetch("/api/metrics/cluster-cpu", {
    credentials: "same-origin",
    signal,
  });
  if (!response.ok) {
    throw new Error(
      `Failed to fetch cluster CPU metrics: ${String(response.status)}`,
    );
  }

  const body = (await response.json()) as ClusterCpuResponse;

  return {
    id: "cpu",
    total: coresToRoundedString(body.capacity_cores),
    unit: "cores",
    value: coresToRoundedString(body.used_cores),
  };
}

async function fetchClusterPodsMetric(
  signal?: AbortSignal,
): Promise<OperationalMetric> {
  const response = await fetch("/api/metrics/cluster-pods", {
    credentials: "same-origin",
    signal,
  });
  if (!response.ok) {
    throw new Error(
      `Failed to fetch cluster pods metrics: ${String(response.status)}`,
    );
  }

  const body = (await response.json()) as ClusterPodsResponse;

  return {
    id: "pods",
    podPhases: {
      failed: body.phase_failed_pods,
      pending: body.phase_pending_pods,
      running: body.phase_running_pods,
      succeeded: body.phase_succeeded_pods,
      unknown: body.phase_unknown_pods,
    },
    total: String(body.capacity_pods),
    unit: "pods",
    value: String(body.used_pods),
  };
}

async function fetchClusterNodesMetric(
  signal?: AbortSignal,
): Promise<OperationalMetric> {
  const response = await fetch("/api/metrics/cluster-nodes", {
    credentials: "same-origin",
    signal,
  });
  if (!response.ok) {
    throw new Error(
      `Failed to fetch cluster nodes metrics: ${String(response.status)}`,
    );
  }

  const body = (await response.json()) as ClusterNodesResponse;

  return {
    id: "nodes",
    status: {
      failed: body.not_ready_nodes,
      healthy: body.ready_nodes,
    },
    value: String(body.total_nodes),
  };
}

function formatProvisionMinutesFromSeconds(seconds: number): string {
  return (seconds / secondsPerMinute).toFixed(2);
}

async function fetchGatewayProvisionDurationMetric(
  signal?: AbortSignal,
): Promise<OperationalMetric | undefined> {
  try {
    const response = await fetch("/api/metrics/gateway-provision-duration", {
      credentials: "same-origin",
      signal,
    });
    if (!response.ok) {
      return undefined;
    }

    const body = (await response.json()) as GatewayProvisionDurationResponse;
    const mean = formatProvisionMinutesFromSeconds(body.mean_seconds);
    const p50 = formatProvisionMinutesFromSeconds(body.p50_seconds);
    const p95 = formatProvisionMinutesFromSeconds(body.p95_seconds);

    return {
      id: "provision-time",
      provisionDuration: {
        mean,
        p50,
        p95,
      },
      unit: "minutes",
      value: mean,
    };
  } catch {
    return undefined;
  }
}

function gatewayDisplayCountsToMetric(
  total: number,
  counts: GatewayDisplayStatusCounts,
): OperationalMetric {
  return {
    id: "provisioned-gateways",
    status: {
      degraded: counts.degraded,
      failed: counts.failed,
      healthy: counts.healthy,
      provisioning: counts.provisioning,
    },
    value: String(total),
  };
}

async function fetchGatewaySandboxesMetric(
  signal?: AbortSignal,
): Promise<OperationalMetric> {
  const response = await fetch("/api/metrics/gateway-sandboxes", {
    credentials: "same-origin",
    signal,
  });
  if (!response.ok) {
    throw new Error(
      `Failed to fetch gateway sandbox metrics: ${String(response.status)}`,
    );
  }

  const body = (await response.json()) as GatewaySandboxesResponse;

  return {
    id: "provisioned-sandboxes",
    value: String(body.active_sandboxes),
  };
}

function isAbortError(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "name" in error &&
    error.name === "AbortError"
  );
}

async function fetchGatewayPrometheusMetric(
  signal?: AbortSignal,
): Promise<OperationalMetric> {
  const phaseCounts = await fetchGatewayMetrics(signal);
  const displayStatusCounts =
    gatewayPhaseCountsToDisplayStatusCounts(phaseCounts);
  const total = Object.values(phaseCounts).reduce(
    (sum, count) => sum + count,
    0,
  );

  return gatewayDisplayCountsToMetric(total, displayStatusCounts);
}

async function fetchGatewayPrometheusMetrics(
  context: DashboardInvocationContext,
): Promise<OperationalMetric[]> {
  const [gatewayMetric, sandboxMetric] = await Promise.all([
    fetchGatewayPrometheusMetric(context.signal),
    fetchGatewaySandboxesMetric(context.signal),
  ]);

  const metrics: OperationalMetric[] = [gatewayMetric, sandboxMetric];

  const provisionTimeMetric = await fetchGatewayProvisionDurationMetric(
    context.signal,
  );
  if (provisionTimeMetric !== undefined) {
    metrics.push(provisionTimeMetric);
  }

  return metrics;
}

async function fetchRegisteredUsersMetric(
  context: DashboardInvocationContext,
): Promise<OperationalMetric[]> {
  const response = await fetch("/api/metrics/registered-users", {
    credentials: "same-origin",
    signal: context.signal,
  });
  if (!response.ok) {
    throw new Error(
      `Failed to fetch registered user metrics: ${String(response.status)}`,
    );
  }

  const body = (await response.json()) as RegisteredUsersResponse;

  return [
    {
      id: "registered-users",
      value: String(body.total_registered),
    },
  ];
}

async function fetchPlatformInventoryMetrics(
  context: DashboardInvocationContext,
): Promise<OperationalMetric[]> {
  const response = await fetch("/api/metrics/platform-inventory", {
    credentials: "same-origin",
    signal: context.signal,
  });
  if (!response.ok) {
    throw new Error(
      `Failed to fetch platform inventory metrics: ${String(response.status)}`,
    );
  }

  const body = (await response.json()) as PlatformInventoryMetricsResponse;
  return platformInventoryMetricsResponseToMetrics(body);
}

interface MetricSourceDefinition {
  fetch: (
    context: DashboardInvocationContext,
    apiFactory: DashboardApiFactory,
  ) => Promise<OperationalMetric[]>;
  id: DashboardMetricSourceId;
}

const metricSources: readonly MetricSourceDefinition[] = [
  {
    id: "gateway-metrics",
    fetch: async (context) => fetchGatewayPrometheusMetrics(context),
  },
  {
    id: "registered-users",
    fetch: async (context) => fetchRegisteredUsersMetric(context),
  },
  {
    id: "platform-inventory",
    fetch: async (context) => fetchPlatformInventoryMetrics(context),
  },
  {
    id: "cluster-memory",
    fetch: async (context) => [await fetchClusterMemoryMetric(context.signal)],
  },
  {
    id: "cluster-cpu",
    fetch: async (context) => [await fetchClusterCpuMetric(context.signal)],
  },
  {
    id: "cluster-pods",
    fetch: async (context) => [await fetchClusterPodsMetric(context.signal)],
  },
  {
    id: "cluster-nodes",
    fetch: async (context) => [await fetchClusterNodesMetric(context.signal)],
  },
];

export function createDashboardControlPlaneAdapter(
  apiFactory: DashboardApiFactory,
): DashboardControlPlane {
  return {
    async getOperationalMetrics(
      context: DashboardInvocationContext,
    ): Promise<OperationalDashboardMetrics> {
      context.signal?.throwIfAborted();

      const settled = await Promise.allSettled(
        metricSources.map((source) =>
          source.fetch(context, apiFactory).then((metrics) => ({
            id: source.id,
            metrics,
          })),
        ),
      );

      const metrics: OperationalMetric[] = [];
      const failedSources: DashboardMetricSourceId[] = [];

      for (const [index, result] of settled.entries()) {
        const source = metricSources[index];
        if (source === undefined) {
          continue;
        }

        if (result.status === "fulfilled") {
          metrics.push(...result.value.metrics);
          continue;
        }

        if (isAbortError(result.reason)) {
          throw result.reason;
        }

        failedSources.push(source.id);
      }

      if (metrics.length === 0) {
        throw new Error("All operational dashboard metric sources failed");
      }

      return {
        ...(failedSources.length > 0 ? { failedSources } : {}),
        lastSuccessfulRefresh: new Date(),
        metrics,
      };
    },
  };
}
