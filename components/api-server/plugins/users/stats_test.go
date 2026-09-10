package users

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestBuildDailySeriesFillsMissingDaysWithZero(t *testing.T) {
	RegisterTestingT(t)

	evaluationTime := time.Date(2026, 9, 8, 15, 30, 0, 0, time.UTC)
	startDay := dailySeriesStart(evaluationTime)
	endDay := utcDayStart(evaluationTime)

	series := buildDailySeries(startDay, endDay, map[string]int64{
		formatUTCDate(endDay): 3,
	})

	Expect(series).To(HaveLen(activityLookbackDays))
	Expect(series[0].Date).To(Equal(formatUTCDate(startDay)))
	Expect(series[0].Count).To(Equal(int64(0)))
	Expect(series[len(series)-1].Date).To(Equal(formatUTCDate(endDay)))
	Expect(series[len(series)-1].Count).To(Equal(int64(3)))
}

func TestFormatUTCDateUsesCalendarDay(t *testing.T) {
	RegisterTestingT(t)

	value := time.Date(2026, 9, 8, 23, 59, 0, 0, time.UTC)
	Expect(formatUTCDate(value)).To(Equal("2026-09-08"))
}

func TestRegistrationWindowStartsUseInclusiveUTCCalendarDays(t *testing.T) {
	RegisterTestingT(t)

	evaluationTime := time.Date(2026, 9, 8, 15, 30, 0, 0, time.UTC)
	last7DayStart := registration7DayWindowStart(evaluationTime)
	last30DayStart := registration30DayWindowStart(evaluationTime)

	Expect(last7DayStart).To(Equal(time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)))
	Expect(last30DayStart).To(Equal(time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)))
	Expect(last7DayStart).To(Equal(last30DayStart.AddDate(0, 0, 23)))
}

func registeredIn7DayWindow(createdAt time.Time, evaluationTime time.Time) bool {
	return !createdAt.Before(registration7DayWindowStart(evaluationTime))
}

func registeredIn30DayWindow(createdAt time.Time, evaluationTime time.Time) bool {
	return !createdAt.Before(registration30DayWindowStart(evaluationTime))
}

func TestRegistrationWindowBoundaryCounts(t *testing.T) {
	RegisterTestingT(t)

	evaluationTime := time.Date(2026, 9, 8, 15, 30, 0, 0, time.UTC)
	last7DayStart := registration7DayWindowStart(evaluationTime)
	last30DayStart := registration30DayWindowStart(evaluationTime)

	createdAts := []time.Time{
		last7DayStart,
		last7DayStart.Add(-time.Nanosecond),
		last30DayStart,
		last30DayStart.Add(-time.Nanosecond),
	}

	registeredLast7Days := 0
	registeredLast30Days := 0
	for _, createdAt := range createdAts {
		if registeredIn7DayWindow(createdAt, evaluationTime) {
			registeredLast7Days++
		}
		if registeredIn30DayWindow(createdAt, evaluationTime) {
			registeredLast30Days++
		}
	}

	Expect(registeredLast7Days).To(Equal(1))
	Expect(registeredLast30Days).To(Equal(3))
	Expect(registeredIn7DayWindow(last7DayStart, evaluationTime)).To(BeTrue())
	Expect(registeredIn7DayWindow(last7DayStart.Add(-time.Nanosecond), evaluationTime)).To(BeFalse())
	Expect(registeredIn30DayWindow(last30DayStart, evaluationTime)).To(BeTrue())
	Expect(registeredIn30DayWindow(last30DayStart.Add(-time.Nanosecond), evaluationTime)).To(BeFalse())
	Expect(registeredIn7DayWindow(last30DayStart, evaluationTime)).To(BeFalse())
}
