import type { OperationalDashboardMetrics } from "../application/dashboard-types";

function buildHourlyTrendPoints(
  baseValue: number,
  step: number,
): { label: string; value: number }[] {
  const points: { label: string; value: number }[] = [];
  const end = new Date();
  const endHour = Date.UTC(
    end.getUTCFullYear(),
    end.getUTCMonth(),
    end.getUTCDate(),
    end.getUTCHours(),
  );

  for (let offset = 23; offset >= 0; offset -= 1) {
    const hour = new Date(endHour - offset * 60 * 60 * 1000);
    const year = hour.getUTCFullYear();
    const month = String(hour.getUTCMonth() + 1).padStart(2, "0");
    const day = String(hour.getUTCDate()).padStart(2, "0");
    const hourLabel = String(hour.getUTCHours()).padStart(2, "0");
    points.push({
      label: `${String(year)}-${month}-${day}T${hourLabel}:00`,
      value: Number((baseValue + (23 - offset) * step).toFixed(3)),
    });
  }

  return points;
}

/**
 * Storybook and local-dev fixture shaped like `getReliabilityMetrics` output,
 * including 24-hour hourly trends for all three API reliability metrics.
 */
export const mockReliabilityDashboardMetrics: OperationalDashboardMetrics =
  Object.freeze({
    lastSuccessfulRefresh: new Date("2026-09-15T14:00:00.000Z"),
    metrics: Object.freeze([
      Object.freeze({
        hourlyTrend: Object.freeze({
          points: Object.freeze(buildHourlyTrendPoints(10.2, 0.1)),
        }),
        id: "api-request-rate",
        unit: "req/s",
        value: "12.50",
      }),
      Object.freeze({
        hourlyTrend: Object.freeze({
          points: Object.freeze(buildHourlyTrendPoints(0.5, 0.03)),
        }),
        id: "api-error-rate",
        unit: "%",
        value: "1.25",
      }),
      Object.freeze({
        hourlyTrend: Object.freeze({
          points: Object.freeze(buildHourlyTrendPoints(0.09, 0.001)),
        }),
        id: "api-latency",
        unit: "sec",
        value: "0.084",
      }),
    ]),
  });
