package defaultWorkspaceMemberships

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

type Dao interface {
	Get(ctx context.Context, id string) (*DefaultWorkspaceMembership, error)
	GetUnscoped(ctx context.Context, id string) (*DefaultWorkspaceMembership, error)
	Create(ctx context.Context, m *DefaultWorkspaceMembership) (*DefaultWorkspaceMembership, error)
	UpdateStatus(ctx context.Context, id, status string, lastErr *string) (*DefaultWorkspaceMembership, error)
	Delete(ctx context.Context, id string) error
	FindByGatewayAndSubject(ctx context.Context, gatewayID, subject string) (*DefaultWorkspaceMembership, error)
	CountLiveForGateway(ctx context.Context, gatewayID string) (int64, error)
	AllForGateway(ctx context.Context, gatewayID string) (DefaultWorkspaceMembershipList, error)
	DeleteAllForGateway(ctx context.Context, gatewayID string) (int64, error)
	FindByIDs(ctx context.Context, ids []string) (DefaultWorkspaceMembershipList, error)
	All(ctx context.Context) (DefaultWorkspaceMembershipList, error)
	// CountByStatus returns the live-row count for every reconciliation_status,
	// keyed by status, for the OBS-CRM-14 pending/failed gauges.
	CountByStatus(ctx context.Context) (map[string]int64, error)
	CreateAudit(ctx context.Context, event *AuditEvent) error
}

var _ Dao = &sqlDao{}

type sqlDao struct {
	sessionFactory *db.SessionFactory
}

func NewDao(sessionFactory *db.SessionFactory) Dao {
	return &sqlDao{sessionFactory: sessionFactory}
}

func (d *sqlDao) Get(ctx context.Context, id string) (*DefaultWorkspaceMembership, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item DefaultWorkspaceMembership
	if err := g2.Take(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// GetUnscoped reads a membership including a soft-deleted row. The watch
// handler's delete event needs the deleted row's own gateway_id and subject
// (a plain Get excludes soft-deleted rows, mirroring roleBindings.GetUnscoped).
func (d *sqlDao) GetUnscoped(ctx context.Context, id string) (*DefaultWorkspaceMembership, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item DefaultWorkspaceMembership
	if err := g2.Unscoped().Take(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (d *sqlDao) Create(ctx context.Context, m *DefaultWorkspaceMembership) (*DefaultWorkspaceMembership, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(m).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return m, nil
}

func (d *sqlDao) UpdateStatus(ctx context.Context, id, status string, lastErr *string) (*DefaultWorkspaceMembership, error) {
	g2 := (*d.sessionFactory).New(ctx)
	updates := map[string]interface{}{"reconciliation_status": status, "last_reconcile_error": lastErr}
	if err := g2.Model(&DefaultWorkspaceMembership{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return d.Get(ctx, id)
}

func (d *sqlDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&DefaultWorkspaceMembership{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

// FindByGatewayAndSubject returns the live membership for (gatewayID, subject),
// or (nil, nil) when none exists. This is the idempotency key the automation
// hooks (GTW-ACC-16, OSG-SACC-19) use to avoid creating a duplicate row.
func (d *sqlDao) FindByGatewayAndSubject(ctx context.Context, gatewayID, subject string) (*DefaultWorkspaceMembership, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var item DefaultWorkspaceMembership
	err := g2.Take(&item, "gateway_id = ? AND subject = ?", gatewayID, subject).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// CountLiveForGateway backs the GTW-ACC-17/OSG-SACC-19 1,000-member limit check.
func (d *sqlDao) CountLiveForGateway(ctx context.Context, gatewayID string) (int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	if err := g2.Model(&DefaultWorkspaceMembership{}).Where("gateway_id = ?", gatewayID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (d *sqlDao) AllForGateway(ctx context.Context, gatewayID string) (DefaultWorkspaceMembershipList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := DefaultWorkspaceMembershipList{}
	if err := g2.Where("gateway_id = ?", gatewayID).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// DeleteAllForGateway implements the PLAT-DATA-27.05 gateway-deletion cascade.
// DB foreign keys in this codebase are ON DELETE RESTRICT (db.CreateFK) and
// gateway deletion is a soft delete, so cascade is an explicit application-level
// bulk delete, not a DB-level cascade.
func (d *sqlDao) DeleteAllForGateway(ctx context.Context, gatewayID string) (int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	result := g2.Where("gateway_id = ?", gatewayID).Delete(&DefaultWorkspaceMembership{})
	if result.Error != nil {
		db.MarkForRollback(ctx, result.Error)
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func (d *sqlDao) FindByIDs(ctx context.Context, ids []string) (DefaultWorkspaceMembershipList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := DefaultWorkspaceMembershipList{}
	if err := g2.Where("id in (?)", ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlDao) All(ctx context.Context) (DefaultWorkspaceMembershipList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	items := DefaultWorkspaceMembershipList{}
	if err := g2.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (d *sqlDao) CountByStatus(ctx context.Context) (map[string]int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	type row struct {
		ReconciliationStatus string
		Count                int64
	}
	var rows []row
	if err := g2.Model(&DefaultWorkspaceMembership{}).
		Select("reconciliation_status, count(*) as count").
		Group("reconciliation_status").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(rows))
	for _, r := range rows {
		counts[r.ReconciliationStatus] = r.Count
	}
	return counts, nil
}

func (d *sqlDao) CreateAudit(ctx context.Context, event *AuditEvent) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(event).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}
