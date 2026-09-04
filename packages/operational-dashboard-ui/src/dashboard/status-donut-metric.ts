import type { OperationalMetric } from "../application/dashboard-types";

export interface StatusDonutMetric {
  id: string;
  status: NonNullable<OperationalMetric["status"]>;
  value: string;
}

export function isStatusDonutMetric(
  metric: OperationalMetric,
): metric is StatusDonutMetric {
  return metric.status !== undefined;
}
