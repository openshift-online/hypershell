export const operationalDashboardMetricsQueryRoot = [
  "operational-dashboard",
  "metrics",
] as const;

export const reliabilityDashboardMetricsQueryRoot = [
  "reliability-dashboard",
  "metrics",
] as const;

export const operationalDashboardRefreshMilliseconds = 15 * 60 * 1000;

export const reliabilityDashboardRefreshMilliseconds =
  operationalDashboardRefreshMilliseconds;

export function operationalDashboardMetricsQueryKey() {
  return [...operationalDashboardMetricsQueryRoot] as const;
}

export function reliabilityDashboardMetricsQueryKey() {
  return [...reliabilityDashboardMetricsQueryRoot] as const;
}
