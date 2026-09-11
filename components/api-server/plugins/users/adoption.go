package users

import "time"

const (
	createdLookback7Days  = 7 * 24 * time.Hour
	createdLookback30Days = 30 * 24 * time.Hour
)

// UserAdoptionDailyLogin is one UTC calendar day in the unique-login sparkline.
type UserAdoptionDailyLogin struct {
	Date  string
	Count int64
}

// UserAdoptionSnapshot is the fleet-wide registered-user adoption aggregate
// computed on each metrics scrape.
type UserAdoptionSnapshot struct {
	TotalRegistered         int64
	CreatedLast7Days        int64
	CreatedLast30Days       int64
	UniqueLoginsLast7Days   int64
	UniqueLoginsLast30Days  int64
	DailyUniqueLogins       []UserAdoptionDailyLogin
}

func utcDayStart(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func buildUserAdoptionSnapshot(
	totalRegistered int64,
	createdLast7Days int64,
	createdLast30Days int64,
	dailyCounts map[string]int64,
	evaluationTime time.Time,
) *UserAdoptionSnapshot {
	today := utcDayStart(evaluationTime)
	startDate := today.AddDate(0, 0, -(activityRetentionDays - 1))

	dailyUniqueLogins := make([]UserAdoptionDailyLogin, 0, activityRetentionDays)
	var uniqueLoginsLast7Days int64
	var uniqueLoginsLast30Days int64

	for dayOffset := 0; dayOffset < activityRetentionDays; dayOffset++ {
		day := startDate.AddDate(0, 0, dayOffset)
		dateLabel := day.Format("2006-01-02")
		count := dailyCounts[dateLabel]
		dailyUniqueLogins = append(dailyUniqueLogins, UserAdoptionDailyLogin{
			Date:  dateLabel,
			Count: count,
		})

		daysFromToday := activityRetentionDays - 1 - dayOffset
		if daysFromToday < 7 {
			uniqueLoginsLast7Days += count
		}
		uniqueLoginsLast30Days += count
	}

	return &UserAdoptionSnapshot{
		TotalRegistered:        totalRegistered,
		CreatedLast7Days:       createdLast7Days,
		CreatedLast30Days:      createdLast30Days,
		UniqueLoginsLast7Days:  uniqueLoginsLast7Days,
		UniqueLoginsLast30Days: uniqueLoginsLast30Days,
		DailyUniqueLogins:      dailyUniqueLogins,
	}
}

// BuildUserAdoptionSnapshotForTest exposes adoption snapshot construction for tests.
func BuildUserAdoptionSnapshotForTest(
	totalRegistered int64,
	createdLast7Days int64,
	createdLast30Days int64,
	dailyCounts map[string]int64,
	evaluationTime time.Time,
) *UserAdoptionSnapshot {
	return buildUserAdoptionSnapshot(
		totalRegistered,
		createdLast7Days,
		createdLast30Days,
		dailyCounts,
		evaluationTime,
	)
}
