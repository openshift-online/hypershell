import type { OperationalMetric } from "../application/dashboard-types";

export interface PodCapacityMetric {
  id: string;
  podPhases: NonNullable<OperationalMetric["podPhases"]>;
  total: string;
  unit: string;
  value: string;
}

export function isPodCapacityMetric(
  metric: OperationalMetric,
): metric is PodCapacityMetric {
  return (
    typeof metric.unit === "string" &&
    typeof metric.total === "string" &&
    metric.podPhases !== undefined
  );
}
