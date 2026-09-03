import type { OperationalDashboardMetrics } from "../application/dashboard-types";

export type DashboardMetricSourceId =
  | "cluster-cpu"
  | "cluster-memory"
  | "cluster-nodes"
  | "cluster-pods"
  | "gateway-list"
  | "platform-inventory"
  | "registered-users";

export const DASHBOARD_METRIC_SOURCE_METRIC_IDS: Readonly<
  Record<DashboardMetricSourceId, readonly string[]>
> = {
  "gateway-list": [
    "provisioned-gateways",
    "provisioned-sandboxes",
    "provision-time",
  ],
  "registered-users": ["registered-users"],
  "platform-inventory": ["managed-clusters", "managed-databases"],
  "cluster-memory": ["memory"],
  "cluster-cpu": ["cpu"],
  "cluster-pods": ["pods"],
  "cluster-nodes": ["nodes"],
};

export function mergeOperationalDashboardMetrics(
  previous: OperationalDashboardMetrics | undefined,
  next: OperationalDashboardMetrics,
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
    next.failedSources.flatMap(
      (sourceId) => DASHBOARD_METRIC_SOURCE_METRIC_IDS[sourceId],
    ),
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
