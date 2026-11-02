import type { OperationalDashboardMetrics } from "../application/dashboard-types";

function buildDailyTrendPoints(
  values: readonly number[],
): { label: string; value: number }[] {
  const today = new Date();
  const utcToday = Date.UTC(
    today.getUTCFullYear(),
    today.getUTCMonth(),
    today.getUTCDate(),
  );

  return values.map((value, index) => {
    const day = new Date(
      utcToday - (values.length - 1 - index) * 24 * 60 * 60 * 1000,
    );
    return {
      label: day.toISOString().slice(0, 10),
      value,
    };
  });
}

const registrationCounts = [
  2, 3, 2, 4, 3, 5, 4, 3, 6, 5, 4, 6, 5, 7, 4, 6, 5, 8, 6, 7, 5, 8, 6, 7, 8, 6,
  9, 7, 8, 10,
] as const;

const activeUserCounts = [
  12, 15, 14, 18, 16, 20, 18, 17, 22, 20, 19, 23, 21, 24, 20, 22, 21, 25, 23,
  24, 22, 26, 24, 25, 27, 23, 28, 26, 27, 30,
] as const;

/** Storybook and local-dev fixture shaped like `getAdoptionMetrics` output. */
export const mockAdoptionDashboardMetrics: OperationalDashboardMetrics =
  Object.freeze({
    lastSuccessfulRefresh: new Date("2026-10-16T14:00:00.000Z"),
    metrics: Object.freeze([
      Object.freeze({
        createdLast7Days: "48",
        createdLast30Days: "123",
        createdTrend: Object.freeze({
          points: Object.freeze(buildDailyTrendPoints(registrationCounts)),
        }),
        dailyActiveUsers: "186",
        gatewayConversionPercent: "68.0",
        id: "adoption",
        inactiveUsersLast30Days: "52",
        retention7DaysPercent: "78.0",
        trend: Object.freeze({
          points: Object.freeze(buildDailyTrendPoints(activeUserCounts)),
        }),
        uniqueLoginsLast7Days: "312",
        uniqueLoginsLast30Days: "398",
        usersWithGateway: "306",
        value: "450",
      }),
    ]),
  });

/** Fixture for the unavailable-state story; optional adoption fields are omitted. */
export const mockAdoptionDashboardUnavailableMetrics: OperationalDashboardMetrics =
  Object.freeze({
    lastSuccessfulRefresh: new Date("2026-10-16T14:00:00.000Z"),
    metrics: Object.freeze([
      Object.freeze({
        id: "adoption",
        value: "450",
      }),
    ]),
  });
