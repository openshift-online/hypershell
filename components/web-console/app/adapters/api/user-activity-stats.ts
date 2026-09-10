import type {
  OperationalMetric,
  OperationalMetricTrend,
} from "@openshift-online/hypershell-operational-dashboard-ui";
import type {
  UserActivityStats,
  UserDailyCount,
} from "@openshift-online/hypershell-sdk";

function dailyCountsToTrend(
  daily: readonly UserDailyCount[],
): OperationalMetricTrend {
  return {
    points: daily.map((point) => ({
      label: point.date,
      value: point.count,
    })),
  };
}

export function userActivityStatsToMetric(
  stats: UserActivityStats,
): OperationalMetric {
  return {
    activeLast7Days: String(stats.active_last_7_days),
    activeLast30Days: String(stats.active_last_30_days),
    activeTrend: dailyCountsToTrend(stats.active_daily),
    createdLast7Days: String(stats.registered_last_7_days),
    createdLast30Days: String(stats.registered_last_30_days),
    id: "registered-users",
    trend: dailyCountsToTrend(stats.registration_daily),
    value: String(stats.total_registered),
  };
}
