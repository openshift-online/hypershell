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

function buildHourlyCountTrendPoints(
  counts: readonly number[],
): { label: string; value: number }[] {
  const end = new Date();
  const endHour = Date.UTC(
    end.getUTCFullYear(),
    end.getUTCMonth(),
    end.getUTCDate(),
    end.getUTCHours(),
  );

  return counts.map((value, index) => {
    const hour = new Date(
      endHour - (counts.length - 1 - index) * 60 * 60 * 1000,
    );
    const year = hour.getUTCFullYear();
    const month = String(hour.getUTCMonth() + 1).padStart(2, "0");
    const day = String(hour.getUTCDate()).padStart(2, "0");
    const hourLabel = String(hour.getUTCHours()).padStart(2, "0");

    return {
      label: `${String(year)}-${month}-${day}T${hourLabel}:00`,
      value,
    };
  });
}

/**
 * Storybook and local-dev fixture shaped like `getReliabilityMetrics` output,
 * including 24-hour hourly trends for API reliability and reconciliation
 * metrics.
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
        unit: "requests/sec",
        value: "12.500",
      }),
      Object.freeze({
        hourlyTrend: Object.freeze({
          points: Object.freeze(buildHourlyTrendPoints(0.5, 0.03)),
        }),
        id: "api-error-rate",
        unit: "%",
        value: "1.250",
      }),
      Object.freeze({
        hourlyTrend: Object.freeze({
          points: Object.freeze(buildHourlyTrendPoints(0.09, 0.001)),
        }),
        id: "api-latency",
        unit: "sec",
        value: "0.084",
      }),
      Object.freeze({
        hourlyTrend: Object.freeze({
          points: Object.freeze(
            buildHourlyCountTrendPoints([
              0, 1, 0, 2, 1, 0, 3, 0, 1, 2, 0, 1, 0, 0, 2, 1, 0, 4, 0, 1, 0, 2,
              1, 1,
            ]),
          ),
        }),
        id: "reconciliation-failures",
        unit: "count",
        value: "23",
      }),
      Object.freeze({
        hourlyTrend: Object.freeze({
          points: Object.freeze(
            buildHourlyCountTrendPoints([
              0, 1, 0, 1, 2, 0, 1, 0, 2, 1, 0, 1, 0, 0, 1, 1, 0, 2, 0, 1, 0, 1,
              1, 1,
            ]),
          ),
        }),
        id: "reconciliation-retries",
        unit: "count",
        value: "16",
      }),
      Object.freeze({
        hourlyTrend: Object.freeze({
          points: Object.freeze(buildHourlyTrendPoints(0.03, 0.001)),
        }),
        id: "reconciliation-lag",
        unit: "sec",
        value: "0.047",
      }),
      Object.freeze({
        hourlyTrend: Object.freeze({
          points: Object.freeze(buildHourlyTrendPoints(18, 4 / 23)),
        }),
        id: "stale-resource-status-count",
        unit: "count",
        value: "22",
      }),
    ]),
  });
