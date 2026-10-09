package gatewayAccess

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

// accessRow is one enriched access grant, assembled server-side by joining
// role_bindings to users and roles (GAM-03: no per-row user lookup).
type accessRow struct {
	RoleBindingID string    `gorm:"column:role_binding_id"`
	UserID        string    `gorm:"column:user_id"`
	Username      string    `gorm:"column:username"`
	Name          *string   `gorm:"column:name"`
	Email         *string   `gorm:"column:email"`
	RoleName      string    `gorm:"column:role_name"`
	GrantedAt     time.Time `gorm:"column:granted_at"`
}

type dao struct {
	sessionFactory *db.SessionFactory
}

func newDao(sf *db.SessionFactory) *dao { return &dao{sessionFactory: sf} }

var sortColumns = map[string]string{
	"granted_at": "rb.created_at",
	"username":   "u.username",
	"name":       "u.name",
	"role":       "r.name",
}

// filtered builds a fresh query for the gateway's access grants with the role
// filter and literal search applied. A new session is returned on every call so
// the count and page queries do not share mutated statement state.
func (d *dao) filtered(ctx context.Context, gatewayID string, opts ListOptions) *gorm.DB {
	q := (*d.sessionFactory).New(ctx).
		Table("role_bindings AS rb").
		Joins("JOIN roles r ON r.id = rb.role_id AND r.deleted_at IS NULL").
		Joins("JOIN users u ON u.id = rb.user_id AND u.deleted_at IS NULL").
		Where("rb.gateway_id = ? AND rb.scope = ? AND rb.deleted_at IS NULL", gatewayID, roleBindings.ScopeGateway)

	if roleName := tierToRoleName(opts.Role); roleName != "" {
		q = q.Where("r.name = ?", roleName)
	}
	if s := strings.TrimSpace(opts.Search); s != "" {
		// Treat the search term as a literal: escape LIKE metacharacters so a
		// user typing "%" or "_" matches those characters, not wildcards.
		pattern := "%" + escapeLike(strings.ToLower(s)) + "%"
		q = q.Where(`(LOWER(u.username) LIKE ? ESCAPE '\' OR LOWER(COALESCE(u.name, '')) LIKE ? ESCAPE '\')`, pattern, pattern)
	}
	return q
}

func (d *dao) list(ctx context.Context, gatewayID string, opts ListOptions) ([]accessRow, int64, error) {
	var total int64
	if err := d.filtered(ctx, gatewayID, opts).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	sortCol := sortColumns[opts.Sort]
	if sortCol == "" {
		sortCol = "rb.created_at"
	}
	order := "DESC"
	if strings.EqualFold(opts.Order, "asc") {
		order = "ASC"
	}

	rows := []accessRow{}
	q := d.filtered(ctx, gatewayID, opts).
		Select("rb.id AS role_binding_id, rb.user_id AS user_id, u.username AS username, u.name AS name, u.email AS email, r.name AS role_name, rb.created_at AS granted_at").
		Order(sortCol + " " + order).
		Order("rb.id ASC")
	if opts.Size > 0 {
		q = q.Limit(opts.Size).Offset((opts.Page - 1) * opts.Size)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// getForUser returns the enriched grant row for a single user on the gateway
// (used to shape the response after a grant/change write).
func (d *dao) getForUser(ctx context.Context, gatewayID, userID string) (accessRow, bool, error) {
	rows := []accessRow{}
	err := (*d.sessionFactory).New(ctx).
		Table("role_bindings AS rb").
		Joins("JOIN roles r ON r.id = rb.role_id AND r.deleted_at IS NULL").
		Joins("JOIN users u ON u.id = rb.user_id AND u.deleted_at IS NULL").
		Where("rb.gateway_id = ? AND rb.scope = ? AND rb.user_id = ? AND rb.deleted_at IS NULL", gatewayID, roleBindings.ScopeGateway, userID).
		Select("rb.id AS role_binding_id, rb.user_id AS user_id, u.username AS username, u.name AS name, u.email AS email, r.name AS role_name, rb.created_at AS granted_at").
		Order("rb.created_at ASC").
		Order("rb.id ASC").
		Limit(1).
		Find(&rows).Error
	if err != nil {
		return accessRow{}, false, err
	}
	if len(rows) == 0 {
		return accessRow{}, false, nil
	}
	return rows[0], true, nil
}

// ownerUserIDs returns the distinct user_ids holding gateway:owner on the
// gateway. Used for last-owner protection (GAM-07).
func (d *dao) ownerUserIDs(ctx context.Context, gatewayID string) ([]string, error) {
	var ids []string
	err := (*d.sessionFactory).New(ctx).
		Table("role_bindings AS rb").
		Joins("JOIN roles r ON r.id = rb.role_id AND r.deleted_at IS NULL").
		Where("rb.gateway_id = ? AND rb.scope = ? AND rb.deleted_at IS NULL AND r.name = ?", gatewayID, roleBindings.ScopeGateway, roles.RoleGatewayOwner).
		Distinct("rb.user_id").
		Pluck("rb.user_id", &ids).Error
	return ids, err
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
