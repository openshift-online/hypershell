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

  it("uses successRateTrend for provision reliability summary arrows", () => {
    expect(
      getMetricTrendChange({
        id: "provision-reliability",
        successRateTrend: {
          points: [
            { label: "2026-08-09T12:00", value: 80 },
            { label: "2026-08-09T13:00", value: 100 },
          ],
        },
        value: "100",
      }),
    ).toEqual({
      direction: "increase",
      percent: 25,
    });
  });

  it("prefers hourlyTrend over daily trend for summary arrows", () => {
    expect(
      getMetricTrendChange({
        hourlyTrend: {
          points: [
            { label: "2026-09-15T10:00", value: 10 },
            { label: "2026-09-15T11:00", value: 20 },
          ],
        },
        id: "provisioned-sandboxes",
        trend: {
          points: [
            { label: "2026-09-09", value: 100 },
            { label: "2026-09-15", value: 90 },
          ],
        },
        value: "20",
      }),
    ).toEqual({
      direction: "increase",
      percent: 100,
    });
  });
});
