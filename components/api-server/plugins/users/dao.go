package users

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
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
	GatewayUsageByUser(ctx context.Context) ([]UserGatewayUsage, error)
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

// UserGatewayUsage is one registered user's fleet usage on one spoke cluster
// (ManagedCluster referenced by their owned gateways): the number of gateways
// they own there (gateway:owner RoleBindings) and the total active sandboxes
// across those gateways. Users with no gateways appear once with zero counts
// and an empty cluster so the caller can distinguish adoption from ownership.
type UserGatewayUsage struct {
	Username  string
	Cluster   string
	Gateways  int64
	Sandboxes int64
}

// GatewayUsageByUser returns per-user, per-spoke-cluster gateway and
// active-sandbox counts for every registered user, ordered by username then
// cluster. Ownership is the gateway:owner RoleBinding (the binding created
// with each gateway); viewer bindings do not count. The DISTINCT inner join
// dedupes a user holding multiple bindings for the same gateway so sandbox
// counts never double-count. Gateways referencing a soft-deleted cluster
// report an empty cluster name but still count toward the user's totals.
func (d *sqlUserDao) GatewayUsageByUser(ctx context.Context) ([]UserGatewayUsage, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var rows []UserGatewayUsage
	if err := g2.Raw(`
		SELECT users.username AS username,
		       COALESCE(managed_clusters.name, '') AS cluster,
		       COUNT(DISTINCT gateways.id) AS gateways,
		       COALESCE(SUM(COALESCE(gateways.active_sandbox_count, 0)), 0) AS sandboxes
		FROM users
		LEFT JOIN (
			SELECT DISTINCT role_bindings.user_id, role_bindings.gateway_id
			FROM role_bindings
			JOIN roles ON roles.id = role_bindings.role_id
				AND roles.deleted_at IS NULL
				AND roles.name = ?
			WHERE role_bindings.deleted_at IS NULL
				AND role_bindings.gateway_id IS NOT NULL
		) owned ON owned.user_id = users.id
		LEFT JOIN gateways ON gateways.id = owned.gateway_id
			AND gateways.deleted_at IS NULL
		LEFT JOIN managed_clusters ON managed_clusters.id = gateways.cluster_id
			AND managed_clusters.deleted_at IS NULL
		WHERE users.deleted_at IS NULL
		GROUP BY users.id, users.username, managed_clusters.name
		ORDER BY users.username, cluster
	`, roles.RoleGatewayOwner).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("gateway usage by user: %w", err)
	}
	return rows, nil
}

func (d *sqlUserDao) CountCreatedSince(ctx context.Context, since time.Time) (int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	if err := g2.Model(&User{}).Where("created_at >= ?", since.UTC()).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count users created since: %w", err)
	}
	return count, nil
}
