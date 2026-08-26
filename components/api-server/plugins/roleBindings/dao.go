package roleBindings

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

type RoleBindingDao interface {
	Get(ctx context.Context, id string) (*RoleBinding, error)
	GetUnscoped(ctx context.Context, id string) (*RoleBinding, error)
	Create(ctx context.Context, rb *RoleBinding) (*RoleBinding, error)
	Delete(ctx context.Context, id string) error
	FindByUserID(ctx context.Context, userID string) (RoleBindingList, error)
	FindByIDs(ctx context.Context, ids []string) (RoleBindingList, error)
	FindGatewayIDsByUserID(ctx context.Context, userID string) ([]string, error)
	FindOwnerUsernamesByGatewayIDs(ctx context.Context, gatewayIDs []string) (map[string]string, error)
	All(ctx context.Context) (RoleBindingList, error)
}

var _ RoleBindingDao = &sqlRoleBindingDao{}

type sqlRoleBindingDao struct {
	sessionFactory *db.SessionFactory
}

func NewRoleBindingDao(sessionFactory *db.SessionFactory) RoleBindingDao {
	return &sqlRoleBindingDao{sessionFactory: sessionFactory}
}

func (d *sqlRoleBindingDao) Get(ctx context.Context, id string) (*RoleBinding, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var rb RoleBinding
	if err := g2.Take(&rb, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &rb, nil
}

func (d *sqlRoleBindingDao) Create(ctx context.Context, rb *RoleBinding) (*RoleBinding, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(rb).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return rb, nil
}

func (d *sqlRoleBindingDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&RoleBinding{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlRoleBindingDao) GetUnscoped(ctx context.Context, id string) (*RoleBinding, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var rb RoleBinding
	if err := g2.Unscoped().Take(&rb, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &rb, nil
}

func (d *sqlRoleBindingDao) FindByUserID(ctx context.Context, userID string) (RoleBindingList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	bindings := RoleBindingList{}
	if err := g2.Where("user_id = ?", userID).Find(&bindings).Error; err != nil {
		return nil, err
	}
	return bindings, nil
}

func (d *sqlRoleBindingDao) FindByIDs(ctx context.Context, ids []string) (RoleBindingList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	bindings := RoleBindingList{}
	if err := g2.Where("id in (?)", ids).Find(&bindings).Error; err != nil {
		return nil, err
	}
	return bindings, nil
}

func (d *sqlRoleBindingDao) FindGatewayIDsByUserID(ctx context.Context, userID string) ([]string, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var gatewayIDs []string
	if err := g2.Model(&RoleBinding{}).
		Where("user_id = ? AND gateway_id IS NOT NULL", userID).
		Distinct("gateway_id").
		Pluck("gateway_id", &gatewayIDs).Error; err != nil {
		return nil, err
	}
	return gatewayIDs, nil
}

func (d *sqlRoleBindingDao) FindOwnerUsernamesByGatewayIDs(ctx context.Context, gatewayIDs []string) (map[string]string, error) {
	if len(gatewayIDs) == 0 {
		return map[string]string{}, nil
	}
	g2 := (*d.sessionFactory).New(ctx)
	type result struct {
		GatewayID string
		Username  string
	}
	var rows []result
	if err := g2.Model(&RoleBinding{}).
		Select("DISTINCT ON (role_bindings.gateway_id) role_bindings.gateway_id, users.username").
		Joins("JOIN roles ON roles.id = role_bindings.role_id AND roles.deleted_at IS NULL").
		Joins("JOIN users ON users.id = role_bindings.user_id AND users.deleted_at IS NULL").
		Where("role_bindings.gateway_id IN ? AND roles.name = ? AND role_bindings.deleted_at IS NULL", gatewayIDs, roles.RoleGatewayOwner).
		Order("role_bindings.gateway_id, role_bindings.created_at ASC, role_bindings.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	owners := make(map[string]string, len(rows))
	for _, r := range rows {
		owners[r.GatewayID] = r.Username
	}
	return owners, nil
}

func (d *sqlRoleBindingDao) All(ctx context.Context) (RoleBindingList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	bindings := RoleBindingList{}
	if err := g2.Find(&bindings).Error; err != nil {
		return nil, err
	}
	return bindings, nil
}
