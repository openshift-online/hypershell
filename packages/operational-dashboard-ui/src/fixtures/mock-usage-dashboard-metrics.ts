import type {
  UsageDashboardMetrics,
  UsageMetric,
} from "../application/dashboard-types";

function buildDailyTrendPoints(
  values: readonly (number | null)[],
): { label: string; value: number | null }[] {
  const end = new Date();
  const endDate = Date.UTC(
    end.getUTCFullYear(),
    end.getUTCMonth(),
    end.getUTCDate(),
  );

  return values.map((value, index) => {
    const day = new Date(
      endDate - (values.length - 1 - index) * 24 * 60 * 60 * 1000,
    );
    const year = day.getUTCFullYear();
    const month = String(day.getUTCMonth() + 1).padStart(2, "0");
    const date = String(day.getUTCDate()).padStart(2, "0");

    return {
      label: `${String(year)}-${month}-${date}`,
      value,
    };
  });
}

function buildTrend(values: readonly (number | null)[]) {
  return {
    coverageEnd: "2026-10-15",
    coverageStart: "2026-09-16",
    points: buildDailyTrendPoints(values),
  };
}

const requestVolume = Array.from({ length: 30 }, (_, index) => 420 + index * 8);
const errorRate = Array.from({ length: 30 }, (_, index) =>
  Number((1.8 - index * 0.02).toFixed(3)),
);
const sandboxLifetime = Array.from(
  { length: 30 },
  (_, index) => 18 + index * 0.4,
);

const metrics: readonly UsageMetric[] = [
  {
    id: "gateway-request-volume",
    trend: buildTrend(requestVolume),
    unit: "requests",
    value: "1240000",
  },
  {
    id: "gateway-error-rate",
    trend: buildTrend(errorRate),
    unit: "%",
    value: "1.25",
  },
  {
    activeSharePercent: "74.23",
    id: "active-gateways",
    total: "97",
    unit: "gateways",
    value: "72",
  },
  {
    id: "unused-gateways",
    unit: "gateways",
    value: "25",
  },
  {
    id: "sandbox-utilization",
    total: "340",
    unit: "sandboxes",
    value: "214",
  },
  {
    id: "sandbox-lifetime",
    sandboxLifetime: {
      mean: "67320",
      p50: "29520",
      p95: "367200",
    },
    trend: buildTrend(sandboxLifetime),
    value: "67320",
  },
];

export const mockUsageDashboardMetrics: UsageDashboardMetrics = Object.freeze({
  lastSuccessfulRefresh: new Date("2026-10-15T14:00:00.000Z"),
  metrics: Object.freeze(metrics.map((metric) => Object.freeze(metric))),
  timeRange: 30,
});
