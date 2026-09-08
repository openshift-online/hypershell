import type { OperationalMetric } from "../application/dashboard-types";

export const TREND_CHANGE_THRESHOLD_PERCENT = 5;

export type MetricTrendDirection = "increase" | "decrease";

export interface MetricTrendChange {
  direction: MetricTrendDirection;
  percent: number;
}

export function getMetricTrendChange(
  metric: OperationalMetric,
  thresholdPercent = TREND_CHANGE_THRESHOLD_PERCENT,
): MetricTrendChange | undefined {
  const trendPoints = metric.trend?.points;
  const firstPoint = trendPoints?.[0];
  const lastPoint = trendPoints?.at(-1);
  if (!firstPoint || !lastPoint) {
    return undefined;
  }

  const startValue = firstPoint.value;
  const currentValue = lastPoint.value;
  if (startValue === 0) {
    return undefined;
  }

  const percentChange = ((currentValue - startValue) / startValue) * 100;

  if (percentChange >= thresholdPercent) {
    return {
      direction: "increase",
      percent: Math.round(percentChange),
    };
  }

  if (percentChange <= -thresholdPercent) {
    return {
      direction: "decrease",
      percent: Math.round(Math.abs(percentChange)),
    };
  }

  return undefined;
}
