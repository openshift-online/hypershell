import { describe, expect, it } from "vitest";

import {
  alignDailyIntegerSeries,
  dailyRangeLookbackDays,
  formatHourLabel,
  formatUtcCalendarDate,
  utcCalendarDatesInclusive,
} from "../src/prometheus-range-query.js";

describe("prometheus range helpers", () => {
  it("formats UTC calendar dates and hour labels", () => {
    expect(formatUtcCalendarDate(1_704_067_200)).toBe("2024-01-01");
    expect(
      formatHourLabel(
        Math.floor(Date.parse("2024-01-15T14:37:00.000Z") / 1000),
      ),
    ).toBe("2024-01-15T14:00");
  });

  it("aligns daily samples to a seven-day UTC grid", () => {
    const dates = utcCalendarDatesInclusive(dailyRangeLookbackDays);
    const latestDate = dates.at(-1);
    if (!latestDate) {
      throw new Error("expected latest UTC calendar date");
    }
    const latestTimestamp = Math.floor(
      Date.parse(`${latestDate}T00:00:00.000Z`) / 1000,
    );
    const samples = new Map<number, number>([[latestTimestamp, 18]]);

    expect(alignDailyIntegerSeries(samples)).toEqual(
      dates.map((date) => ({
        date,
        value: date === latestDate ? 18 : 0,
      })),
    );
  });
});
