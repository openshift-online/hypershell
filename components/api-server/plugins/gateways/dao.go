package gateways

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
)

type GatewayDao interface {
	Get(ctx context.Context, id string) (*Gateway, error)
	GetUnscoped(ctx context.Context, id string) (*Gateway, error)
	Create(ctx context.Context, gateway *Gateway) (*Gateway, error)
	Replace(ctx context.Context, gateway *Gateway) (*Gateway, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (GatewayList, error)
	All(ctx context.Context) (GatewayList, error)

	// AdjustActiveSandboxCount atomically applies delta to the
	// active_sandbox_count of the live gateway in the given namespace, flooring
	// the result at zero and treating a NULL count as zero. It returns the
	// resolved gateway ID (empty when no live gateway backs the namespace), the
	// resulting count, and whether the stored value actually changed (so the
	// caller can skip emitting a redundant event).
	AdjustActiveSandboxCount(ctx context.Context, namespace string, delta int) (gatewayID string, count int, changed bool, err error)

	// SetActiveSandboxCount atomically sets the active_sandbox_count of the live
	// gateway in the given namespace to an absolute value, floored at zero. Its
	// return contract matches AdjustActiveSandboxCount.
	SetActiveSandboxCount(ctx context.Context, namespace string, count int) (gatewayID string, resulting int, changed bool, err error)
}

// sandboxCountRow captures the gateway identity and count returned by the
// atomic sandbox-count updates.
type sandboxCountRow struct {
	ID                 string
	ActiveSandboxCount *int
}

var _ GatewayDao = &sqlGatewayDao{}

type sqlGatewayDao struct {
	sessionFactory *db.SessionFactory
}

func NewGatewayDao(sessionFactory *db.SessionFactory) GatewayDao {
	return &sqlGatewayDao{sessionFactory: sessionFactory}
}

func (d *sqlGatewayDao) Get(ctx context.Context, id string) (*Gateway, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var gateway Gateway
	if err := g2.Take(&gateway, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &gateway, nil
}

func (d *sqlGatewayDao) GetUnscoped(ctx context.Context, id string) (*Gateway, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var gateway Gateway
	if err := g2.Unscoped().Take(&gateway, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &gateway, nil
}

func (d *sqlGatewayDao) Create(ctx context.Context, gateway *Gateway) (*Gateway, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(gateway).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return gateway, nil
}

func (d *sqlGatewayDao) Replace(ctx context.Context, gateway *Gateway) (*Gateway, error) {
	g2 := (*d.sessionFactory).New(ctx)
	// Omit active_sandbox_count: it is owned exclusively by the atomic
	// AdjustActiveSandboxCount / SetActiveSandboxCount path. Saving it here would
	// write back the value read into `gateway`, clobbering any concurrent
	// count adjustment with a stale number.
	if err := g2.Omit(clause.Associations, "ActiveSandboxCount").Save(gateway).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return gateway, nil
}

func (d *sqlGatewayDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&Gateway{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlGatewayDao) FindByIDs(ctx context.Context, ids []string) (GatewayList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	gateways := GatewayList{}
	if err := g2.Where("id in (?)", ids).Find(&gateways).Error; err != nil {
		return nil, err
	}
	return gateways, nil
}

func (d *sqlGatewayDao) All(ctx context.Context) (GatewayList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	gateways := GatewayList{}
	if err := g2.Find(&gateways).Error; err != nil {
		return nil, err
	}
	return gateways, nil
}

func (d *sqlGatewayDao) AdjustActiveSandboxCount(ctx context.Context, namespace string, delta int) (string, int, bool, error) {
	// A single UPDATE ... RETURNING is atomic at the row level, so concurrent
	// deltas serialize on the row lock and never lose an increment. GREATEST/
	// COALESCE floor the result at zero and treat an unset (NULL) count as zero.
	// The IS DISTINCT FROM guard skips the write (and thus the returned row) when
	// the value would not change - e.g. a decrement already at zero - so the
	// caller emits no redundant event.
	const stmt = `
UPDATE gateways
SET active_sandbox_count = GREATEST(0, COALESCE(active_sandbox_count, 0) + ?)
WHERE namespace = ? AND deleted_at IS NULL
  AND active_sandbox_count IS DISTINCT FROM GREATEST(0, COALESCE(active_sandbox_count, 0) + ?)
RETURNING id, active_sandbox_count`
	return d.execSandboxCount(ctx, namespace, stmt, delta, namespace, delta)
}

func (d *sqlGatewayDao) SetActiveSandboxCount(ctx context.Context, namespace string, count int) (string, int, bool, error) {
	const stmt = `
UPDATE gateways
SET active_sandbox_count = GREATEST(0, ?)
WHERE namespace = ? AND deleted_at IS NULL
  AND active_sandbox_count IS DISTINCT FROM GREATEST(0, ?)
RETURNING id, active_sandbox_count`
	return d.execSandboxCount(ctx, namespace, stmt, count, namespace, count)
}

// execSandboxCount runs a guarded sandbox-count UPDATE ... RETURNING and, when
// it matched no row (either the value was unchanged or no live gateway exists),
// falls back to a lookup by namespace to distinguish "unchanged" from
// "not found" and to report the current stored value.
func (d *sqlGatewayDao) execSandboxCount(ctx context.Context, namespace, stmt string, args ...interface{}) (string, int, bool, error) {
	g2 := (*d.sessionFactory).New(ctx)

	var row sandboxCountRow
	if err := g2.Raw(stmt, args...).Scan(&row).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return "", 0, false, err
	}
	if row.ID != "" {
		return row.ID, derefCount(row.ActiveSandboxCount), true, nil
	}

	// No row updated: either the guard excluded an unchanged value or the
	// namespace has no live gateway. Resolve which without mutating anything.
	var current sandboxCountRow
	if err := g2.Raw(
		`SELECT id, active_sandbox_count FROM gateways WHERE namespace = ? AND deleted_at IS NULL`,
		namespace,
	).Scan(&current).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return "", 0, false, err
	}
	return current.ID, derefCount(current.ActiveSandboxCount), false, nil
}

func derefCount(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
