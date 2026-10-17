import {
  fetchGatewayMetrics,
  gatewayPhaseCountsToDisplayStatusCounts,
  type GatewayDisplayStatusCounts,
  type GatewayMetrics,
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

function rangePath(path: string, timeRange: 7 | 30 | undefined): string {
  return timeRange === 30 ? `${path}?range=30` : path;
}

function clusterMetricPath(path: string, timeRange: 7 | 30 | undefined) {
  return `${path}?range=${String(timeRange ?? 7)}`;
}

interface DailyTrendPoint {
  date: string;
  value: number | null;
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
  daily_p50?: { date: string; p50_seconds: number }[];
  daily_p95?: { date: string; p95_seconds: number }[];
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
  daily_success_rate?: { date: string; success_rate_percent: number }[];
}

interface GatewaySandboxesResponse {
  active_sandboxes: number;
  orphaned_sandboxes?: number;
  expiring_sandboxes?: number;
  idle_sandboxes?: number;
  daily_active_sandboxes?: { count: number; date: string }[];
  hourly_active_sandboxes?: { count: number; hour: string }[];
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

interface AdoptionResponse {
  registered_users: number;
  created_last_7_days?: number;
  created_last_30_days?: number;
  unique_logins_last_7_days?: number;
  unique_logins_last_30_days?: number;
  inactive_users_last_30_days?: number;
  users_with_gateway?: number;
  gateway_conversion_percent?: number;
  daily_created_users?: RegisteredUsersDailyLogin[];
  daily_unique_logins?: RegisteredUsersDailyLogin[];
  retention_7_days_percent?: number;
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

function adoptionResponseToMetric(body: AdoptionResponse): OperationalMetric {
  const dailyActiveUsers = body.daily_unique_logins?.at(-1)?.count;

  return {
    id: "adoption",
    value: String(body.registered_users),
    ...(body.created_last_7_days === undefined
      ? {}
      : { createdLast7Days: String(body.created_last_7_days) }),
    ...(body.created_last_30_days === undefined
      ? {}
      : { createdLast30Days: String(body.created_last_30_days) }),
    ...(dailyActiveUsers === undefined
      ? {}
      : { dailyActiveUsers: String(dailyActiveUsers) }),
    ...(body.unique_logins_last_7_days === undefined
      ? {}
      : {
          uniqueLoginsLast7Days: String(body.unique_logins_last_7_days),
        }),
    ...(body.unique_logins_last_30_days === undefined
      ? {}
      : {
          uniqueLoginsLast30Days: String(body.unique_logins_last_30_days),
        }),
    ...(body.inactive_users_last_30_days === undefined
      ? {}
      : {
          inactiveUsersLast30Days: String(body.inactive_users_last_30_days),
        }),
    ...(body.users_with_gateway === undefined
      ? {}
      : { usersWithGateway: String(body.users_with_gateway) }),
    ...(body.gateway_conversion_percent === undefined
      ? {}
      : {
          gatewayConversionPercent: body.gateway_conversion_percent.toFixed(1),
        }),
    ...(body.daily_created_users === undefined
      ? {}
      : {
          createdTrend: {
            points: body.daily_created_users.map((point) => ({
              label: point.date,
              value: point.count,
            })),
          },
        }),
    ...(body.daily_unique_logins === undefined
      ? {}
      : {
          trend: {
            points: body.daily_unique_logins.map((point) => ({
              label: point.date,
              value: point.count,
            })),
          },
        }),
    ...(body.retention_7_days_percent === undefined
      ? {}
      : {
          retention7DaysPercent: body.retention_7_days_percent.toFixed(1),
        }),
  };
}

function mapDailyTrend(
  dailySeries: readonly { date: string; value: number | null }[] | undefined,
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

function bytesToRoundedGib(bytes: number): string {
  return String(Math.round(bytes / gibibyteDivisor));
}

function coresToRoundedString(cores: number): string {
  return String(Math.round(cores));
}

async function fetchClusterMemoryMetric(
  context: DashboardInvocationContext,
): Promise<OperationalMetric> {
  const response = await fetch(
    clusterMetricPath("/api/metrics/cluster-memory", context.timeRange),
    {
      credentials: "same-origin",
      signal: context.signal,
    },
  );
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
  context: DashboardInvocationContext,
): Promise<OperationalMetric> {
  const response = await fetch(
    clusterMetricPath("/api/metrics/cluster-cpu", context.timeRange),
    {
      credentials: "same-origin",
      signal: context.signal,
    },
  );
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
  context: DashboardInvocationContext,
): Promise<OperationalMetric | undefined> {
  try {
    const response = await fetch(
      rangePath("/api/metrics/gateway-provision-outcomes", context.timeRange),
      {
        credentials: "same-origin",
        signal: context.signal,
      },
    );
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
    const dailyPoints = (body.daily_success_rate ?? []).map((point) => ({
      label: point.date,
      value: point.success_rate_percent,
    }));

    return {
      id: "provision-reliability",
      provisionOutcomes: {
        failureCount24h: String(body.failure_count_24h),
        successCount24h: String(body.success_count_24h),
        successRatePercent,
        ...(dailyPoints.length >= 2
          ? { successRateTrend: { points: dailyPoints } }
          : {}),
      },
      ...(dailyPoints.length >= 2
        ? {
            successRateTrend: {
              points: dailyPoints,
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
  context: DashboardInvocationContext,
): Promise<OperationalMetric | undefined> {
  try {
    const response = await fetch(
      rangePath("/api/metrics/gateway-provision-duration", context.timeRange),
      {
        credentials: "same-origin",
        signal: context.signal,
      },
    );
    if (!response.ok) {
      return undefined;
    }

    const body = (await response.json()) as GatewayProvisionDurationResponse;
    const mean = formatProvisionSeconds(body.mean_seconds);
    const p50 = formatProvisionSeconds(body.p50_seconds);
    const p95 = formatProvisionSeconds(body.p95_seconds);
    const p50Trend = (body.daily_p50 ?? []).map((point) => ({
      label: point.date,
      value: point.p50_seconds,
    }));
    const p95Trend = (body.daily_p95 ?? []).map((point) => ({
      label: point.date,
      value: point.p95_seconds,
    }));

    return {
      id: "provision-time",
      provisionDuration: {
        mean,
        p50,
        p95,
        ...(p50Trend.length >= 2 ? { p50Trend: { points: p50Trend } } : {}),
        ...(p95Trend.length >= 2 ? { p95Trend: { points: p95Trend } } : {}),
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
  trend?: { points: { label: string; value: number }[] },
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
  context: DashboardInvocationContext,
): Promise<OperationalMetric> {
  const response = await fetch(
    rangePath("/api/metrics/gateway-sandboxes", context.timeRange),
    {
      credentials: "same-origin",
      signal: context.signal,
    },
  );
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
  context: DashboardInvocationContext,
): Promise<OperationalMetric> {
  const body: GatewayMetrics = await fetchGatewayMetrics(
    context.signal,
    context.timeRange ?? 7,
  );
  const displayStatusCounts: GatewayDisplayStatusCounts =
    gatewayPhaseCountsToDisplayStatusCounts(body);
  const total =
    displayStatusCounts.healthy +
    displayStatusCounts.provisioning +
    displayStatusCounts.degraded +
    displayStatusCounts.failed;
  const trend =
    body.dailyFleetTotals === undefined
      ? undefined
      : {
          points: body.dailyFleetTotals.map((point) => ({
            label: point.date,
            value: point.total,
          })),
        };

  return gatewayDisplayCountsToMetric(total, displayStatusCounts, trend);
}

async function fetchGatewayPrometheusMetrics(
  context: DashboardInvocationContext,
): Promise<OperationalMetric[]> {
  const [gatewayMetric, sandboxMetric] = await Promise.all([
    fetchGatewayPrometheusMetric(context),
    fetchGatewaySandboxesMetric(context),
  ]);

  const metrics: OperationalMetric[] = [gatewayMetric, sandboxMetric];

  if (context.isAdmin === false) {
    return metrics;
  }

  const [provisionTimeMetric, provisionReliabilityMetric] = await Promise.all([
    fetchGatewayProvisionDurationMetric(context),
    fetchGatewayProvisionOutcomesMetric(context),
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

async function fetchAdoptionMetrics(
  context: DashboardInvocationContext,
): Promise<OperationalMetric[]> {
  const response = await fetch(
    rangePath("/api/metrics/adoption", context.timeRange),
    {
      credentials: "same-origin",
      signal: context.signal,
    },
  );
  if (!response.ok) {
    throw new Error(
      `Failed to fetch adoption metrics: ${String(response.status)}`,
    );
  }

  const body = (await response.json()) as AdoptionResponse;
  return [adoptionResponseToMetric(body)];
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

interface ControlPlaneReconciliationPoint {
  hour: string;
  value: number;
}

interface ControlPlaneReconciliationResponse {
  reconciliation_failures_count: number;
  reconciliation_retries_count: number;
  reconciliation_successes_count: number;
  reconciliation_lag_p50_seconds?: number;
  stale_resource_status_count: number;
  reconciliation_failures_by_reason?: Record<string, number>;
  reconciliation_retries_by_reason?: Record<string, number>;
  hourly_reconciliation_failures_count?: ControlPlaneReconciliationPoint[];
  hourly_reconciliation_retries_count?: ControlPlaneReconciliationPoint[];
  hourly_reconciliation_successes_count?: ControlPlaneReconciliationPoint[];
  hourly_reconciliation_lag_p50_seconds?: ControlPlaneReconciliationPoint[];
  hourly_stale_resource_status_count?: ControlPlaneReconciliationPoint[];
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
      unit: "requests/sec",
      value: body.request_rate.toFixed(3),
      ...(requestRateTrend ? { hourlyTrend: requestRateTrend } : {}),
    },
    {
      id: "api-error-rate",
      unit: "%",
      value: body.error_rate_percent.toFixed(3),
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

export function mapControlPlaneReconciliationResponse(
  body: ControlPlaneReconciliationResponse,
): OperationalMetric[] {
  const values = [
    body.reconciliation_failures_count,
    body.reconciliation_retries_count,
    body.reconciliation_successes_count,
    body.stale_resource_status_count,
  ];
  if (values.some((value) => !isFiniteNumber(value))) {
    throw new Error(
      "Control-plane reconciliation response is missing required fields",
    );
  }

  const failuresTrend = mapHourlyTrend(
    body.hourly_reconciliation_failures_count,
  );
  const retriesTrend = mapHourlyTrend(body.hourly_reconciliation_retries_count);
  const successesTrend = mapHourlyTrend(
    body.hourly_reconciliation_successes_count,
  );
  const lagTrend = mapHourlyTrend(body.hourly_reconciliation_lag_p50_seconds);
  const staleTrend = mapHourlyTrend(body.hourly_stale_resource_status_count);
  const metrics: OperationalMetric[] = [
    {
      id: "reconciliation-failures",
      unit: "count",
      value: body.reconciliation_failures_count.toFixed(0),
      ...(body.reconciliation_failures_by_reason
        ? { reasonBreakdown: body.reconciliation_failures_by_reason }
        : {}),
      ...(failuresTrend ? { hourlyTrend: failuresTrend } : {}),
    },
    {
      id: "reconciliation-retries",
      unit: "count",
      value: body.reconciliation_retries_count.toFixed(0),
      ...(body.reconciliation_retries_by_reason
        ? { reasonBreakdown: body.reconciliation_retries_by_reason }
        : {}),
      ...(retriesTrend ? { hourlyTrend: retriesTrend } : {}),
    },
    {
      id: "reconciliation-successes",
      unit: "count",
      value: body.reconciliation_successes_count.toFixed(0),
      ...(successesTrend ? { hourlyTrend: successesTrend } : {}),
    },
    ...(isFiniteNumber(body.reconciliation_lag_p50_seconds)
      ? [
          {
            id: "reconciliation-lag",
            unit: "sec",
            value: body.reconciliation_lag_p50_seconds.toFixed(3),
            ...(lagTrend ? { hourlyTrend: lagTrend } : {}),
          },
        ]
      : []),
    {
      id: "stale-resource-status-count",
      unit: "count",
      value: String(Math.round(body.stale_resource_status_count)),
      ...(staleTrend ? { hourlyTrend: staleTrend } : {}),
    },
  ];
  return metrics;
}

async function fetchControlPlaneReconciliationMetrics(
  context: DashboardInvocationContext,
): Promise<OperationalMetric[]> {
  const response = await fetch("/api/metrics/control-plane-reconciliation", {
    credentials: "same-origin",
    signal: context.signal,
  });
  if (!response.ok) {
    throw new Error(
      `Failed to fetch control-plane reconciliation metrics: ${String(response.status)}`,
    );
  }
  return mapControlPlaneReconciliationResponse(
    (await response.json()) as ControlPlaneReconciliationResponse,
  );
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
    id: "gateway-release-distribution",
    fetch: async (context, apiFactory) => {
      const client = apiFactory(context.correlationId);
      const aggregate = await aggregateGatewayReleaseDistribution(
        client,
        context.signal,
      );
      return [buildGatewayReleasesMetric(aggregate)];
    },
  },
  {
    id: "platform-inventory",
    fetch: async (context) => fetchPlatformInventoryMetrics(context),
  },
  {
    id: "cluster-memory",
    fetch: async (context) => [await fetchClusterMemoryMetric(context)],
  },
  {
    id: "cluster-cpu",
    fetch: async (context) => [await fetchClusterCpuMetric(context)],
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

      const activeSources =
        context.isAdmin === false
          ? metricSources.filter((source) =>
              ["gateway-metrics", "gateway-release-distribution"].includes(
                source.id,
              ),
            )
          : metricSources;
      const settled = await Promise.allSettled(
        activeSources.map((source) =>
          source.fetch(context, apiFactory).then((metrics) => ({
            id: source.id,
            metrics,
          })),
        ),
      );

      const metrics: OperationalMetric[] = [];
      const failedSources: DashboardMetricSourceId[] = [];

      for (const [index, result] of settled.entries()) {
        const source = activeSources[index];
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
        timeRange: context.timeRange ?? 7,
      };
    },

    async getReliabilityMetrics(
      context: DashboardInvocationContext,
    ): Promise<OperationalDashboardMetrics> {
      context.signal?.throwIfAborted();

      try {
        const results = await Promise.allSettled([
          fetchApiReliabilityMetrics(context),
          fetchControlPlaneReconciliationMetrics(context),
        ]);
        const metrics: OperationalMetric[] = [];
        const failedSources: (
          "api-reliability" | "control-plane-reconciliation"
        )[] = [];
        for (const [index, result] of results.entries()) {
          const source =
            index === 0 ? "api-reliability" : "control-plane-reconciliation";
          if (result.status === "fulfilled") metrics.push(...result.value);
          else if (isAbortError(result.reason)) throw result.reason;
          else failedSources.push(source);
        }
        if (metrics.length === 0)
          throw new Error("All reliability dashboard metric sources failed");
        return {
          ...(failedSources.length > 0 ? { failedSources } : {}),
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
          failedSources: ["api-reliability", "control-plane-reconciliation"],
          lastSuccessfulRefresh: new Date(),
          metrics: [],
        };
      }
    },

    async getAdoptionMetrics(
      context: DashboardInvocationContext,
    ): Promise<OperationalDashboardMetrics> {
      context.signal?.throwIfAborted();

      const metrics = await fetchAdoptionMetrics(context);

      return {
        lastSuccessfulRefresh: new Date(),
        metrics,
        timeRange: context.timeRange ?? 7,
      };
    },
  };
}
