package users

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

const activityRetentionDays = 30

type UserActivityDao interface {
	UpsertDailyActivity(ctx context.Context, userID string, activityDate time.Time) error
	DailyUniqueLoginCounts(ctx context.Context, startDate time.Time, endDate time.Time) (map[string]int64, error)
	PruneBefore(ctx context.Context, cutoffDate time.Time) error
}

var _ UserActivityDao = &sqlUserActivityDao{}

type sqlUserActivityDao struct {
	sessionFactory *db.SessionFactory
}

func NewUserActivityDao(sessionFactory *db.SessionFactory) UserActivityDao {
	return &sqlUserActivityDao{sessionFactory: sessionFactory}
}

func utcCalendarDate(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func (d *sqlUserActivityDao) UpsertDailyActivity(ctx context.Context, userID string, activityDate time.Time) error {
	g2 := (*d.sessionFactory).New(ctx)
	row := &UserDailyActivity{
		UserID:       userID,
		ActivityDate: utcCalendarDate(activityDate),
	}
	if err := g2.Clauses(clause.OnConflict{DoNothing: true}).Create(row).Error; err != nil {
		return fmt.Errorf("upsert daily activity: %w", err)
	}
	return nil
}

func (d *sqlUserActivityDao) DailyUniqueLoginCounts(
	ctx context.Context,
	startDate time.Time,
	endDate time.Time,
) (map[string]int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	type dailyCountRow struct {
		ActivityDate time.Time
		Count        int64
	}
	rows := []dailyCountRow{}
	err := g2.Model(&UserDailyActivity{}).
		Select("activity_date, COUNT(*) AS count").
		Where("activity_date >= ? AND activity_date <= ?", utcCalendarDate(startDate), utcCalendarDate(endDate)).
		Group("activity_date").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("count daily unique logins: %w", err)
	}

	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.ActivityDate.UTC().Format("2006-01-02")] = row.Count
	}
	return counts, nil
}

func (d *sqlUserActivityDao) PruneBefore(ctx context.Context, cutoffDate time.Time) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Where("activity_date < ?", utcCalendarDate(cutoffDate)).Delete(&UserDailyActivity{}).Error; err != nil {
		return fmt.Errorf("prune daily activity: %w", err)
	}
	return nil
}
