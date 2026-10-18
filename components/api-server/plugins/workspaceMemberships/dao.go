package workspaceMemberships

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type WorkspaceMembershipDao interface {
	Get(ctx context.Context, id string) (*WorkspaceMembership, error)
	Create(ctx context.Context, workspaceMembership *WorkspaceMembership) (*WorkspaceMembership, error)
	Replace(ctx context.Context, workspaceMembership *WorkspaceMembership) (*WorkspaceMembership, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (WorkspaceMembershipList, error)
	All(ctx context.Context) (WorkspaceMembershipList, error)
	// CountAdmins counts the admin memberships of a workspace, excluding the
	// membership with excludeID (pass "" to count all). It backs the advisory 422
	// admin-membership-required check.
	CountAdmins(ctx context.Context, workspaceID, excludeID string) (int64, error)
	// WorkspaceRequiresAdmin reports whether the workspace has any ProviderBinding
	// with a non-empty refresh_strategy, which requires an admin membership
	// (data-model.spec.md "WorkspaceMembership"). It reads the sibling
	// provider_bindings table directly to avoid a cross-plugin import cycle.
	WorkspaceRequiresAdmin(ctx context.Context, workspaceID string) (bool, error)
}

var _ WorkspaceMembershipDao = &sqlWorkspaceMembershipDao{}

type sqlWorkspaceMembershipDao struct {
	sessionFactory *db.SessionFactory
}

func NewWorkspaceMembershipDao(sessionFactory *db.SessionFactory) WorkspaceMembershipDao {
	return &sqlWorkspaceMembershipDao{sessionFactory: sessionFactory}
}

func (d *sqlWorkspaceMembershipDao) Get(ctx context.Context, id string) (*WorkspaceMembership, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item WorkspaceMembership
	if err := g2.Take(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *sqlWorkspaceMembershipDao) Create(ctx context.Context, workspaceMembership *WorkspaceMembership) (*WorkspaceMembership, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(workspaceMembership).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return workspaceMembership, nil
}

func (d *sqlWorkspaceMembershipDao) Replace(ctx context.Context, workspaceMembership *WorkspaceMembership) (*WorkspaceMembership, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(workspaceMembership).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return workspaceMembership, nil
}

func (d *sqlWorkspaceMembershipDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&WorkspaceMembership{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlWorkspaceMembershipDao) FindByIDs(ctx context.Context, ids []string) (WorkspaceMembershipList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := WorkspaceMembershipList{}
	if err := g2.Where("id in (?)", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlWorkspaceMembershipDao) All(ctx context.Context) (WorkspaceMembershipList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := WorkspaceMembershipList{}
	if err := g2.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlWorkspaceMembershipDao) CountAdmins(ctx context.Context, workspaceID, excludeID string) (int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	q := g2.Model(&WorkspaceMembership{}).Where("workspace_id = ? AND role = ?", workspaceID, RoleAdmin)
	if excludeID != "" {
		q = q.Where("id <> ?", excludeID)
	}
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (d *sqlWorkspaceMembershipDao) WorkspaceRequiresAdmin(ctx context.Context, workspaceID string) (bool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	if err := g2.Table("provider_bindings").
		Where("workspace_id = ? AND deleted_at IS NULL AND refresh_strategy IS NOT NULL AND refresh_strategy <> ''", workspaceID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
