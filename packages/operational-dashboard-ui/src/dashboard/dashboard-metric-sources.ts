import type {
  DashboardMetricSourceId,
  OperationalDashboardMetrics,
  ReliabilityMetricSourceId,
} from "../application/dashboard-types";

export type { DashboardMetricSourceId };

export const DASHBOARD_METRIC_SOURCE_METRIC_IDS: Readonly<
  Record<DashboardMetricSourceId, readonly string[]>
> = {
  "gateway-metrics": [
    "provisioned-gateways",
    "provisioned-sandboxes",
    "provision-time",
    "provision-reliability",
  ],
  "gateway-release-distribution": ["gateway-releases"],
  "registered-users": ["registered-users"],
  "platform-inventory": ["managed-clusters"],
  "cluster-memory": ["memory"],
  "cluster-cpu": ["cpu"],
  "cluster-pods": ["pods"],
  "cluster-nodes": ["nodes"],
};

export const RELIABILITY_METRIC_SOURCE_METRIC_IDS: Readonly<
  Record<ReliabilityMetricSourceId, readonly string[]>
> = {
  "api-reliability": ["api-request-rate", "api-error-rate", "api-latency"],
};

function mergeDashboardMetricsBySourceMap(
  previous: OperationalDashboardMetrics | undefined,
  next: OperationalDashboardMetrics,
  sourceMetricIds: Readonly<Record<string, readonly string[]>>,
): OperationalDashboardMetrics {
  if (
    previous === undefined ||
    next.failedSources === undefined ||
    next.failedSources.length === 0
  ) {
    return next;
  }

  const mergedById = new Map(next.metrics.map((metric) => [metric.id, metric]));
  const staleMetricIds = new Set(
    next.failedSources.flatMap((sourceId) => sourceMetricIds[sourceId] ?? []),
  );

  for (const metric of previous.metrics) {
    if (staleMetricIds.has(metric.id) && !mergedById.has(metric.id)) {
      mergedById.set(metric.id, metric);
    }
  }

  return {
    ...next,
    metrics: [...mergedById.values()],
  };
}

export function mergeOperationalDashboardMetrics(
  previous: OperationalDashboardMetrics | undefined,
  next: OperationalDashboardMetrics,
): OperationalDashboardMetrics {
  return mergeDashboardMetricsBySourceMap(
    previous,
    next,
    DASHBOARD_METRIC_SOURCE_METRIC_IDS,
  );
}

export function mergeReliabilityDashboardMetrics(
  previous: OperationalDashboardMetrics | undefined,
  next: OperationalDashboardMetrics,
): OperationalDashboardMetrics {
  return mergeDashboardMetricsBySourceMap(
    previous,
    next,
    RELIABILITY_METRIC_SOURCE_METRIC_IDS,
  );
}
