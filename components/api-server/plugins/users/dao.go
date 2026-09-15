package users

import (
	"context"

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
	CountRegistered(ctx context.Context) (int64, error)
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

// Upsert atomically inserts or updates by username via INSERT ... ON
// CONFLICT, avoiding the check-then-act race of a separate SELECT + Create/
// Save: two concurrent first-time provisioning calls for the same brand-new
// username (e.g. a JIT-provisioned OIDC identity's first API request) would
// otherwise both find no existing row, both attempt Create, and the loser
// would get a raw unique-constraint error instead of the persisted user.
//
// The user's ID is client-generated (BeforeCreate, not DB-autoincrement), so
// GORM does not reliably scan the server's post-conflict row back into it:
// on a losing INSERT it can leave the caller's own generated ID in place
// instead of the row that actually persisted. Re-read by username so every
// caller resolves to the one canonical row regardless of who won the race.
func (d *sqlUserDao) Upsert(ctx context.Context, user *User) (*User, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "username"}},
		DoUpdates: clause.AssignmentColumns([]string{"email", "name"}),
	}).Omit(clause.Associations).Create(user).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}

	var persisted User
	if err := g2.Take(&persisted, "username = ?", user.Username).Error; err != nil {
		return nil, err
	}
	return &persisted, nil
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
		return 0, err
	}
	return count, nil
}
