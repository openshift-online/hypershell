import { describe, expect, it } from "vitest";

import type { OperationalMetric } from "../application/dashboard-types";
import { getMetricTrendChange } from "./metric-trend-change";

function metricWithTrend(values: number[]): OperationalMetric {
  return {
    id: "pods",
    trend: {
      points: values.map((value, index) => ({
        label: `Day ${String(index + 1)}`,
        value,
      })),
    },
    value: String(values.at(-1) ?? 0),
  };
}

describe("getMetricTrendChange", () => {
  it("detects an increase above the default threshold", () => {
    expect(getMetricTrendChange(metricWithTrend([100, 110]))).toEqual({
      direction: "increase",
      percent: 10,
    });
  });

  it("detects a decrease above the default threshold", () => {
    expect(getMetricTrendChange(metricWithTrend([100, 90]))).toEqual({
      direction: "decrease",
      percent: 10,
    });
  });

  it("returns undefined when the change is within the threshold", () => {
    expect(getMetricTrendChange(metricWithTrend([100, 104]))).toBeUndefined();
  });

  it("returns undefined when the starting trend value is zero", () => {
    expect(getMetricTrendChange(metricWithTrend([0, 50]))).toBeUndefined();
  });
});
