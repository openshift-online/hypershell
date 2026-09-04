export const operationalDashboardMetricsQueryRoot = [
  "operational-dashboard",
  "metrics",
] as const;

export const operationalDashboardRefreshMilliseconds = 15 * 60 * 1000;

export function operationalDashboardMetricsQueryKey() {
  return [...operationalDashboardMetricsQueryRoot] as const;
}
