import { describe, expect, it } from "vitest";

import { userActivityStatsToMetric } from "./user-activity-stats";

describe("userActivityStatsToMetric", () => {
  it("maps user activity stats into the registered-users operational metric", () => {
    expect(
      userActivityStatsToMetric({
        active_daily: [{ count: 2, date: "2026-09-01" }],
        active_last_7_days: 4,
        active_last_30_days: 10,
        registered_last_7_days: 1,
        registered_last_30_days: 3,
        registration_daily: [{ count: 1, date: "2026-09-01" }],
        total_registered: 42,
      }),
    ).toEqual({
      activeLast7Days: "4",
      activeLast30Days: "10",
      activeTrend: {
        points: [{ label: "2026-09-01", value: 2 }],
      },
      createdLast7Days: "1",
      createdLast30Days: "3",
      id: "registered-users",
      trend: {
        points: [{ label: "2026-09-01", value: 1 }],
      },
      value: "42",
    });
  });
});
