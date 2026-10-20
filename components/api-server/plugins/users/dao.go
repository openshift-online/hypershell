package users

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
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
	CountRegistered(ctx context.Context) (int64, error)
	CountCreatedSince(ctx context.Context, since time.Time) (int64, error)
	CountCreatedBetweenByDay(
		ctx context.Context,
		startDate time.Time,
		endDate time.Time,
	) (map[string]int64, error)
	CountInactiveSinceWithoutActivityAfter(
		ctx context.Context,
		createdBefore time.Time,
		activityAfter time.Time,
	) (int64, error)
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

// Upsert resolves a User row by username, writing a stored profile field
// (email, name) only when the caller supplied a value for it and that value
// differs from what is stored. A call whose claims already match the stored
// row takes no UPDATE and so no row lock; a nil field (e.g. a service-account
// token with no email claim) never clears a stored value. This matters
// because auto-provisioning runs on every authenticated request inside the
// request's own DB transaction, alongside any other write that request
// makes (e.g. granting another user gateway access pre-provisions that
// user's row in the same transaction) -- an unconditional UPDATE here would
// lock the caller's row on every request and risk a lock-ordering deadlock
// when two callers concurrently provision each other's row.
//
// For a brand-new username, insert-or-ignore via INSERT ... ON CONFLICT DO
// NOTHING avoids the check-then-act race of a separate SELECT + Create: two
// concurrent first-time provisioning calls for the same brand-new username
// (e.g. a JIT-provisioned OIDC identity's first API request) would otherwise
// both find no existing row, both attempt Create, and the loser would get a
// raw unique-constraint error instead of the persisted user.
//
// The user's ID is client-generated (BeforeCreate, not DB-autoincrement), so
// GORM does not reliably scan the server's post-conflict row back into it:
// on a losing INSERT it can leave the caller's own generated ID in place
// instead of the row that actually persisted. Re-read by username so every
// caller resolves to the one canonical row regardless of who won the race.
func (d *sqlUserDao) Upsert(ctx context.Context, user *User) (*User, error) {
	existing, err := d.GetByUsername(ctx, user.Username)
	switch {
	case err == nil:
		return d.updateChangedFields(ctx, existing, user)
	case errors.Is(err, gorm.ErrRecordNotFound):
		return d.insertIfAbsent(ctx, user)
	default:
		return nil, err
	}
}

// updateChangedFields writes only the profile fields incoming carries that
// differ from existing, taking no UPDATE (and so no row lock) when nothing
// changed.
func (d *sqlUserDao) updateChangedFields(ctx context.Context, existing, incoming *User) (*User, error) {
	updates := map[string]interface{}{}
	if incoming.Email != nil && (existing.Email == nil || *existing.Email != *incoming.Email) {
		updates["email"] = *incoming.Email
	}
	if incoming.Name != nil && (existing.Name == nil || *existing.Name != *incoming.Name) {
		updates["name"] = *incoming.Name
	}
	if len(updates) == 0 {
		return existing, nil
	}

	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Model(&User{}).Where("username = ?", existing.Username).Updates(updates).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return d.GetByUsername(ctx, existing.Username)
}

// insertIfAbsent inserts user, leaving any row a concurrent caller already
// created in place, then re-reads by username so every racing caller
// resolves to the same canonical row.
func (d *sqlUserDao) insertIfAbsent(ctx context.Context, user *User) (*User, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "username"}},
		DoNothing: true,
	}).Omit(clause.Associations).Create(user).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}

	return d.GetByUsername(ctx, user.Username)
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

func (d *sqlUserDao) CountRegistered(ctx context.Context) (int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	if err := g2.Model(&User{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count registered users: %w", err)
	}
	return count, nil
}

func (d *sqlUserDao) CountCreatedSince(ctx context.Context, since time.Time) (int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	if err := g2.Model(&User{}).Where("created_at >= ?", since.UTC()).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count users created since: %w", err)
	}
	return count, nil
}

func (d *sqlUserDao) CountCreatedBetweenByDay(
	ctx context.Context,
	startDate time.Time,
	endDate time.Time,
) (map[string]int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	type dailyCountRow struct {
		CreatedDate string
		Count       int64
	}
	rows := []dailyCountRow{}
	err := g2.Model(&User{}).
		Select("TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS created_date, COUNT(*) AS count").
		Where("created_at >= ? AND created_at < ?", utcDayStart(startDate), utcDayStart(endDate).AddDate(0, 0, 1)).
		Group("TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD')").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("count users created by day: %w", err)
	}

	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.CreatedDate] = row.Count
	}
	return counts, nil
}

func (d *sqlUserDao) CountInactiveSinceWithoutActivityAfter(
	ctx context.Context,
	createdBefore time.Time,
	activityAfter time.Time,
) (int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	err := g2.Model(&User{}).
		Where("created_at < ?", createdBefore.UTC()).
		Where(
			"NOT EXISTS (SELECT 1 FROM user_daily_activities activity WHERE activity.user_id = users.id AND activity.activity_date >= ?)",
			utcDayStart(activityAfter),
		).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count inactive users: %w", err)
	}
	return count, nil
}
