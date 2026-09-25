import {
  emptyGatewayPhaseCounts,
  gatewayPhaseCountsToDisplayStatusCounts,
  gatewayPhases,
  type GatewayDisplayStatusCounts,
  type GatewayPhaseCounts,
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
  aggregateGatewayReleaseDistribution,
  buildGatewayReleasesMetric,
} from "./gateway-release-distribution-aggregation";
import {
  platformInventoryMetricsResponseToMetrics,
  type PlatformInventoryMetricsResponse,
} from "./platform-inventory-aggregation";

type DashboardApiFactory = (correlationId: string) => SDKClient;

const gibibyteDivisor = 1024 ** 3;

interface DailyTrendPoint {
  date: string;
  value: number;
}

interface ClusterMemoryResponse {
  available_bytes: number;
  capacity_bytes: number;
  daily_used?: DailyTrendPoint[];
  used_bytes: number;
}

interface ClusterCpuResponse {
  available_cores: number;
  capacity_cores: number;
  daily_used?: DailyTrendPoint[];
  used_cores: number;
}

interface ClusterPodsResponse {
  available_pods: number;
  capacity_pods: number;
  daily_used?: DailyTrendPoint[];
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

interface GatewayProvisionHourlySuccessRate {
  failure_count: number;
  hour: string;
  success_count: number;
  success_rate_percent: number;
}

interface GatewayProvisionOutcomesResponse {
  failure_count_24h: number;
  hourly_success_rate: GatewayProvisionHourlySuccessRate[];
  success_count_24h: number;
  success_rate_percent: number | null;
}

interface GatewaySandboxesResponse {
  active_sandboxes: number;
  orphaned_sandboxes?: number;
  expiring_sandboxes?: number;
  idle_sandboxes?: number;
  daily_active_sandboxes?: { count: number; date: string }[];
  hourly_active_sandboxes?: { count: number; hour: string }[];
}

interface GatewayMetricsResponse {
  counts: Record<string, number>;
  daily_fleet_totals?: { date: string; total: number }[];
}

interface RegisteredUsersDailyLogin {
  count: number;
  date: string;
}

interface RegisteredUsersResponse {
  created_last_7_days?: number;
  created_last_30_days?: number;
  daily_unique_logins?: RegisteredUsersDailyLogin[];
  total_registered: number;
  unique_logins_last_7_days?: number;
  unique_logins_last_30_days?: number;
}

function registeredUsersResponseToMetric(
  body: RegisteredUsersResponse,
): OperationalMetric {
  const metric: OperationalMetric = {
    id: "registered-users",
    value: String(body.total_registered),
  };

  if (body.created_last_7_days !== undefined) {
    metric.createdLast7Days = String(body.created_last_7_days);
  }
  if (body.created_last_30_days !== undefined) {
    metric.createdLast30Days = String(body.created_last_30_days);
  }
  if (body.unique_logins_last_7_days !== undefined) {
    metric.uniqueLoginsLast7Days = String(body.unique_logins_last_7_days);
  }
  if (body.unique_logins_last_30_days !== undefined) {
    metric.uniqueLoginsLast30Days = String(body.unique_logins_last_30_days);
  }
  if (body.daily_unique_logins !== undefined) {
    metric.trend = {
      points: body.daily_unique_logins.map((point) => ({
        label: point.date,
        value: point.count,
      })),
    };
  }

  return metric;
}

function mapDailyTrend(
  dailySeries: readonly { date: string; value: number }[] | undefined,
) {
  if (dailySeries === undefined) {
    return undefined;
  }

  return {
    points: dailySeries.map((point) => ({
      label: point.date,
      value: point.value,
    })),
  };
}

function mapFleetTotalTrend(
  dailyFleetTotals: GatewayMetricsResponse["daily_fleet_totals"],
) {
  if (dailyFleetTotals === undefined) {
    return undefined;
  }

  return {
    points: dailyFleetTotals.map((point) => ({
      label: point.date,
      value: point.total,
    })),
  };
}

function parseGatewayPhaseCounts(
  counts: Record<string, number>,
): GatewayPhaseCounts {
  const phaseCounts = emptyGatewayPhaseCounts();
  for (const phase of gatewayPhases) {
    phaseCounts[phase] = counts[phase] ?? 0;
  }
  return phaseCounts;
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

  const trend = mapDailyTrend(body.daily_used);

  return {
    id: "memory",
    total: bytesToRoundedGib(body.capacity_bytes),
    unit: "GiB",
    value: bytesToRoundedGib(body.used_bytes),
    ...(trend ? { trend } : {}),
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

  const trend = mapDailyTrend(body.daily_used);

  return {
    id: "cpu",
    total: coresToRoundedString(body.capacity_cores),
    unit: "cores",
    value: coresToRoundedString(body.used_cores),
    ...(trend ? { trend } : {}),
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

  const trend = mapDailyTrend(body.daily_used);

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
    ...(trend ? { trend } : {}),
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

function formatProvisionSeconds(seconds: number): string {
  return seconds.toFixed(2);
}

function formatSuccessRatePercent(value: number): string {
  return value.toFixed(1);
}

async function fetchGatewayProvisionOutcomesMetric(
  signal?: AbortSignal,
): Promise<OperationalMetric | undefined> {
  try {
    const response = await fetch("/api/metrics/gateway-provision-outcomes", {
      credentials: "same-origin",
      signal,
    });
    if (!response.ok) {
      return undefined;
    }

    const body = (await response.json()) as GatewayProvisionOutcomesResponse;
    if (body.success_rate_percent === null) {
      return undefined;
    }

    const successRatePercent = formatSuccessRatePercent(
      body.success_rate_percent,
    );
    const hourlyPoints = body.hourly_success_rate.map((point) => ({
      label: point.hour,
      value: point.success_rate_percent,
    }));

    return {
      id: "provision-reliability",
      provisionOutcomes: {
        failureCount24h: String(body.failure_count_24h),
        successCount24h: String(body.success_count_24h),
        successRatePercent,
      },
      ...(hourlyPoints.length >= 2
        ? {
            successRateTrend: {
              points: hourlyPoints,
            },
          }
        : {}),
      value: successRatePercent,
    };
  } catch {
    return undefined;
  }
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
    const mean = formatProvisionSeconds(body.mean_seconds);
    const p50 = formatProvisionSeconds(body.p50_seconds);
    const p95 = formatProvisionSeconds(body.p95_seconds);

    return {
      id: "provision-time",
      provisionDuration: {
        mean,
        p50,
        p95,
      },
      unit: "sec",
      value: mean,
    };
  } catch {
    return undefined;
  }
}

function gatewayDisplayCountsToMetric(
  total: number,
  counts: GatewayDisplayStatusCounts,
  trend?: ReturnType<typeof mapFleetTotalTrend>,
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
    ...(trend ? { trend } : {}),
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
  const hourlyTrend =
    body.hourly_active_sandboxes === undefined
      ? undefined
      : {
          points: body.hourly_active_sandboxes.map((point) => ({
            label: point.hour,
            value: point.count,
          })),
        };
  const trend =
    body.daily_active_sandboxes === undefined
      ? undefined
      : {
          points: body.daily_active_sandboxes.map((point) => ({
            label: point.date,
            value: point.count,
          })),
        };

  return {
    id: "provisioned-sandboxes",
    value: String(body.active_sandboxes),
    ...(body.orphaned_sandboxes === undefined
      ? {}
      : { orphanedSandboxes: body.orphaned_sandboxes }),
    ...(body.expiring_sandboxes === undefined
      ? {}
      : { expiringSandboxes: body.expiring_sandboxes }),
    ...(body.idle_sandboxes === undefined
      ? {}
      : { idleSandboxes: body.idle_sandboxes }),
    ...(hourlyTrend ? { hourlyTrend } : {}),
    ...(trend ? { trend } : {}),
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
  const response = await fetch("/api/metrics/gateways", {
    credentials: "same-origin",
    signal,
  });
  if (!response.ok) {
    throw new Error(
      `Failed to fetch gateway metrics: ${String(response.status)}`,
    );
  }

  const body = (await response.json()) as GatewayMetricsResponse;
  const phaseCounts = parseGatewayPhaseCounts(body.counts);
  const displayStatusCounts =
    gatewayPhaseCountsToDisplayStatusCounts(phaseCounts);
  const total = Object.values(phaseCounts).reduce(
    (sum, count) => sum + count,
    0,
  );
  const trend = mapFleetTotalTrend(body.daily_fleet_totals);

  return gatewayDisplayCountsToMetric(total, displayStatusCounts, trend);
}

async function fetchGatewayPrometheusMetrics(
  context: DashboardInvocationContext,
): Promise<OperationalMetric[]> {
  const [gatewayMetric, sandboxMetric] = await Promise.all([
    fetchGatewayPrometheusMetric(context.signal),
    fetchGatewaySandboxesMetric(context.signal),
  ]);

  const metrics: OperationalMetric[] = [gatewayMetric, sandboxMetric];

  const [provisionTimeMetric, provisionReliabilityMetric] = await Promise.all([
    fetchGatewayProvisionDurationMetric(context.signal),
    fetchGatewayProvisionOutcomesMetric(context.signal),
  ]);
  if (provisionTimeMetric !== undefined) {
    metrics.push(provisionTimeMetric);
  }
  if (provisionReliabilityMetric !== undefined) {
    metrics.push(provisionReliabilityMetric);
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

  return [registeredUsersResponseToMetric(body)];
}

async function fetchGatewayReleaseDistributionMetrics(
  context: DashboardInvocationContext,
  apiFactory: DashboardApiFactory,
): Promise<OperationalMetric[]> {
  const client = apiFactory(context.correlationId);
  const aggregate = await aggregateGatewayReleaseDistribution(
    client,
    context.signal,
  );

  return [buildGatewayReleasesMetric(aggregate)];
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

interface ApiReliabilityHourlyPoint {
  hour: string;
  value: number;
}

interface ApiReliabilityResponse {
  error_rate_percent: number;
  hourly_error_rate_percent?: ApiReliabilityHourlyPoint[];
  hourly_latency_p50_seconds?: ApiReliabilityHourlyPoint[];
  hourly_request_rate?: ApiReliabilityHourlyPoint[];
  latency_p50_seconds: number;
  request_rate: number;
}

function mapHourlyTrend(
  series: readonly ApiReliabilityHourlyPoint[] | undefined,
): OperationalMetric["hourlyTrend"] {
  if (series === undefined) {
    return undefined;
  }

  return {
    points: series.map((point) => ({
      label: point.hour,
      value: point.value,
    })),
  };
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value);
}

function mapApiReliabilityResponse(
  body: ApiReliabilityResponse,
): OperationalMetric[] {
  if (
    !isFiniteNumber(body.request_rate) ||
    !isFiniteNumber(body.error_rate_percent) ||
    !isFiniteNumber(body.latency_p50_seconds)
  ) {
    throw new Error(
      "API reliability metrics response is missing required fields",
    );
  }

  const requestRateTrend = mapHourlyTrend(body.hourly_request_rate);
  const errorRateTrend = mapHourlyTrend(body.hourly_error_rate_percent);
  const latencyTrend = mapHourlyTrend(body.hourly_latency_p50_seconds);

  return [
    {
      id: "api-request-rate",
      unit: "req/s",
      value: body.request_rate.toFixed(2),
      ...(requestRateTrend ? { hourlyTrend: requestRateTrend } : {}),
    },
    {
      id: "api-error-rate",
      unit: "%",
      value: body.error_rate_percent.toFixed(2),
      ...(errorRateTrend ? { hourlyTrend: errorRateTrend } : {}),
    },
    {
      id: "api-latency",
      unit: "sec",
      value: body.latency_p50_seconds.toFixed(3),
      ...(latencyTrend ? { hourlyTrend: latencyTrend } : {}),
    },
  ];
}

async function fetchApiReliabilityMetrics(
  context: DashboardInvocationContext,
): Promise<OperationalMetric[]> {
  const response = await fetch("/api/metrics/api-reliability", {
    credentials: "same-origin",
    signal: context.signal,
  });
  if (!response.ok) {
    throw new Error(
      `Failed to fetch API reliability metrics: ${String(response.status)}`,
    );
  }

  const body = (await response.json()) as ApiReliabilityResponse;
  return mapApiReliabilityResponse(body);
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
    id: "gateway-release-distribution",
    fetch: async (context, apiFactory) =>
      fetchGatewayReleaseDistributionMetrics(context, apiFactory),
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

    async getReliabilityMetrics(
      context: DashboardInvocationContext,
    ): Promise<OperationalDashboardMetrics> {
      context.signal?.throwIfAborted();

      try {
        const metrics = await fetchApiReliabilityMetrics(context);
        return {
          lastSuccessfulRefresh: new Date(),
          metrics,
        };
      } catch (error) {
        if (isAbortError(error)) {
          throw error;
        }

        // Soft-fail like getOperationalMetrics partial sources so
        // mergeReliabilityDashboardMetrics can keep stale widgets on
        // refresh and the page can show the partial-load warning.
        return {
          failedSources: ["api-reliability"],
          lastSuccessfulRefresh: new Date(),
          metrics: [],
        };
      }
    },
  };
}
