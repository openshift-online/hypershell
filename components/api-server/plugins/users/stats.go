package users

import "time"

const activityLookbackDays = 30

type DailyCount struct {
	Date  string
	Count int64
}

type ActivityStats struct {
	TotalRegistered      int64
	RegisteredLast7Days  int64
	RegisteredLast30Days int64
	ActiveLast7Days      int64
	ActiveLast30Days     int64
	RegistrationDaily    []DailyCount
	ActiveDaily          []DailyCount
}

func utcDayStart(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func formatUTCDate(value time.Time) string {
	return utcDayStart(value).Format("2006-01-02")
}

func dailySeriesStart(evaluationTime time.Time) time.Time {
	endDay := utcDayStart(evaluationTime)
	return endDay.AddDate(0, 0, -(activityLookbackDays - 1))
}

func registration7DayWindowStart(evaluationTime time.Time) time.Time {
	return utcDayStart(evaluationTime).AddDate(0, 0, -6)
}

func registration30DayWindowStart(evaluationTime time.Time) time.Time {
	return dailySeriesStart(evaluationTime)
}

func buildDailySeries(
	startDay time.Time,
	endDay time.Time,
	counts map[string]int64,
) []DailyCount {
	series := make([]DailyCount, 0, activityLookbackDays)
	for day := startDay; !day.After(endDay); day = day.AddDate(0, 0, 1) {
		date := formatUTCDate(day)
		series = append(series, DailyCount{
			Date:  date,
			Count: counts[date],
		})
	}
	return series
}
