package users

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

type UserDao interface {
	Get(ctx context.Context, id string) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	Create(ctx context.Context, user *User) (*User, error)
	Replace(ctx context.Context, user *User) (*User, error)
	Delete(ctx context.Context, id string) error
	Upsert(ctx context.Context, user *User) (*User, error)
	FindByIDs(ctx context.Context, ids []string) (UserList, error)
	All(ctx context.Context) (UserList, error)
	RecordLogin(ctx context.Context, userID string, loginTime time.Time) error
	GetActivityStats(ctx context.Context, evaluationTime time.Time) (*ActivityStats, error)
}

var _ UserDao = &sqlUserDao{}

type sqlUserDao struct {
	sessionFactory *db.SessionFactory
}

func NewUserDao(sessionFactory *db.SessionFactory) UserDao {
	return &sqlUserDao{sessionFactory: sessionFactory}
}

func (d *sqlUserDao) Get(ctx context.Context, id string) (*User, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var user User
	if err := g2.Take(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (d *sqlUserDao) GetByUsername(ctx context.Context, username string) (*User, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var user User
	if err := g2.Take(&user, "username = ?", username).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (d *sqlUserDao) Create(ctx context.Context, user *User) (*User, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(user).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return user, nil
}

func (d *sqlUserDao) Replace(ctx context.Context, user *User) (*User, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(user).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return user, nil
}

func (d *sqlUserDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&User{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlUserDao) Upsert(ctx context.Context, user *User) (*User, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var existing User
	result := g2.Where("username = ?", user.Username).Take(&existing)
	if result.Error == nil {
		existing.Email = user.Email
		existing.Name = user.Name
		if err := g2.Omit(clause.Associations).Save(&existing).Error; err != nil {
			db.MarkForRollback(ctx, err)
			return nil, err
		}
		return &existing, nil
	}
	if err := g2.Omit(clause.Associations).Create(user).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return user, nil
}

func (d *sqlUserDao) FindByIDs(ctx context.Context, ids []string) (UserList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	users := UserList{}
	if err := g2.Where("id in (?)", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (d *sqlUserDao) All(ctx context.Context) (UserList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	users := UserList{}
	if err := g2.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (d *sqlUserDao) RecordLogin(ctx context.Context, userID string, loginTime time.Time) error {
	_ = ctx
	// Best-effort telemetry on an independent session (see seedUserWithCreatedAt).
	// Request-scoped transactions abort on any statement error in PostgreSQL, so
	// tracking must not share the provisioning transaction even when the service
	// logs and continues on failure.
	g2 := (*d.sessionFactory).New(context.Background())
	loginAt := loginTime.UTC()
	loginDay := utcDayStart(loginAt)

	loginRecord := UserLoginDay{
		UserID:    userID,
		LoginDate: loginDay,
	}
	result := g2.Clauses(clause.OnConflict{DoNothing: true}).Create(&loginRecord)
	if result.Error != nil {
		return fmt.Errorf("record login day: %w", result.Error)
	}

	// Bump last_login_at only on the first authenticated request of each UTC day.
	// Daily activity metrics come from user_login_days; skipping repeat updates
	// within the same day avoids an unconditional write on every request.
	if result.RowsAffected > 0 {
		if err := g2.Model(&User{}).Where("id = ?", userID).Update("last_login_at", loginAt).Error; err != nil {
			return fmt.Errorf("record login: %w", err)
		}
	}

	return nil
}

type registrationDailyRow struct {
	Date  string
	Count int64
}

type activeDailyRow struct {
	Date  string
	Count int64
}

func (d *sqlUserDao) GetActivityStats(ctx context.Context, evaluationTime time.Time) (*ActivityStats, error) {
	g2 := (*d.sessionFactory).New(ctx)
	endDay := utcDayStart(evaluationTime)
	startDay := dailySeriesStart(evaluationTime)
	last7DayStart := registration7DayWindowStart(evaluationTime)
	last30DayStart := registration30DayWindowStart(evaluationTime)

	stats := &ActivityStats{}

	if err := g2.Model(&User{}).Count(&stats.TotalRegistered).Error; err != nil {
		return nil, fmt.Errorf("get activity stats: count total registered: %w", err)
	}

	if err := g2.Model(&User{}).
		Where("created_at >= ?", last7DayStart).
		Count(&stats.RegisteredLast7Days).Error; err != nil {
		return nil, fmt.Errorf("get activity stats: count registered last 7 days: %w", err)
	}

	if err := g2.Model(&User{}).
		Where("created_at >= ?", last30DayStart).
		Count(&stats.RegisteredLast30Days).Error; err != nil {
		return nil, fmt.Errorf("get activity stats: count registered last 30 days: %w", err)
	}

	if err := g2.Raw(
		"SELECT COUNT(DISTINCT user_id) FROM user_login_days WHERE login_date >= ?",
		last7DayStart,
	).Scan(&stats.ActiveLast7Days).Error; err != nil {
		return nil, fmt.Errorf("get activity stats: count active last 7 days: %w", err)
	}

	if err := g2.Raw(
		"SELECT COUNT(DISTINCT user_id) FROM user_login_days WHERE login_date >= ?",
		last30DayStart,
	).Scan(&stats.ActiveLast30Days).Error; err != nil {
		return nil, fmt.Errorf("get activity stats: count active last 30 days: %w", err)
	}

	var registrationRows []registrationDailyRow
	if err := g2.Model(&User{}).
		Select("TO_CHAR(DATE(created_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD') AS date, COUNT(*) AS count").
		Where("created_at >= ?", last30DayStart).
		Group("DATE(created_at AT TIME ZONE 'UTC')").
		Order("DATE(created_at AT TIME ZONE 'UTC') ASC").
		Scan(&registrationRows).Error; err != nil {
		return nil, fmt.Errorf("get activity stats: registration daily series: %w", err)
	}

	var activeRows []activeDailyRow
	if err := g2.Model(&UserLoginDay{}).
		Select("TO_CHAR(login_date, 'YYYY-MM-DD') AS date, COUNT(DISTINCT user_id) AS count").
		Where("login_date >= ?", last30DayStart).
		Group("login_date").
		Order("login_date ASC").
		Scan(&activeRows).Error; err != nil {
		return nil, fmt.Errorf("get activity stats: active daily series: %w", err)
	}

	registrationCounts := make(map[string]int64, len(registrationRows))
	for _, row := range registrationRows {
		registrationCounts[row.Date] = row.Count
	}

	activeCounts := make(map[string]int64, len(activeRows))
	for _, row := range activeRows {
		activeCounts[row.Date] = row.Count
	}

	stats.RegistrationDaily = buildDailySeries(startDay, endDay, registrationCounts)
	stats.ActiveDaily = buildDailySeries(startDay, endDay, activeCounts)

	return stats, nil
}
