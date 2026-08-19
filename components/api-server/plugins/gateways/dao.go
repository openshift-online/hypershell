package gateways

import (
	"context"

	"gorm.io/gorm"
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
	// the result at zero and treating a NULL count as zero, and returns the
	// resulting count (zero when no live gateway backs the namespace). When the
	// stored value actually changes it also emits the Gateway update Event in the
	// SAME transaction as the count mutation (transactional outbox), so a
	// committed change always has its notification and a rolled-back one never
	// emits.
	AdjustActiveSandboxCount(ctx context.Context, namespace string, delta int) (count int, err error)

	// SetActiveSandboxCount atomically sets the active_sandbox_count of the live
	// gateway in the given namespace to an absolute value, floored at zero. Its
	// return and event-emission contract matches AdjustActiveSandboxCount.
	SetActiveSandboxCount(ctx context.Context, namespace string, count int) (resulting int, err error)
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

func (d *sqlGatewayDao) AdjustActiveSandboxCount(ctx context.Context, namespace string, delta int) (int, error) {
	// A single UPDATE ... RETURNING is atomic at the row level, so concurrent
	// deltas serialize on the row lock and never lose an increment. GREATEST/
	// COALESCE floor the result at zero and treat an unset (NULL) count as zero.
	// The IS DISTINCT FROM guard skips the write (and thus the returned row) when
	// the value would not change - e.g. a decrement already at zero - so no
	// redundant event is emitted.
	const stmt = `
UPDATE gateways
SET active_sandbox_count = GREATEST(0, COALESCE(active_sandbox_count, 0) + ?)
WHERE namespace = ? AND deleted_at IS NULL
  AND active_sandbox_count IS DISTINCT FROM GREATEST(0, COALESCE(active_sandbox_count, 0) + ?)
RETURNING id, active_sandbox_count`
	return d.execSandboxCount(ctx, namespace, stmt, delta, namespace, delta)
}

func (d *sqlGatewayDao) SetActiveSandboxCount(ctx context.Context, namespace string, count int) (int, error) {
	const stmt = `
UPDATE gateways
SET active_sandbox_count = GREATEST(0, ?)
WHERE namespace = ? AND deleted_at IS NULL
  AND active_sandbox_count IS DISTINCT FROM GREATEST(0, ?)
RETURNING id, active_sandbox_count`
	return d.execSandboxCount(ctx, namespace, stmt, count, namespace, count)
}

// execSandboxCount runs a guarded sandbox-count UPDATE ... RETURNING inside a
// single transaction. When the UPDATE changes the stored value it emits the
// Gateway update Event in that same transaction (transactional outbox), so the
// count mutation and its notification commit atomically - the framework's
// EventBroker fans the event out to gRPC watchers. When no row is updated
// (either the guard excluded an unchanged value or no live gateway backs the
// namespace) it falls back to a read to report the current stored value and
// emits nothing, because nothing changed.
func (d *sqlGatewayDao) execSandboxCount(ctx context.Context, namespace, stmt string, args ...interface{}) (int, error) {
	g2 := (*d.sessionFactory).New(ctx)

	var count int
	txErr := g2.Transaction(func(tx *gorm.DB) error {
		var row sandboxCountRow
		if err := tx.Raw(stmt, args...).Scan(&row).Error; err != nil {
			return err
		}
		if row.ID != "" {
			count = derefCount(row.ActiveSandboxCount)
			return emitGatewayEventTx(tx, row.ID)
		}

		// No row updated: either the guard excluded an unchanged value or the
		// namespace has no live gateway. Resolve the current value without
		// mutating anything, and emit no event.
		var current sandboxCountRow
		if err := tx.Raw(
			`SELECT id, active_sandbox_count FROM gateways WHERE namespace = ? AND deleted_at IS NULL`,
			namespace,
		).Scan(&current).Error; err != nil {
			return err
		}
		count = derefCount(current.ActiveSandboxCount)
		return nil
	})
	if txErr != nil {
		db.MarkForRollback(ctx, txErr)
		return 0, txErr
	}
	return count, nil
}

// emitGatewayEventTx inserts a Gateway update Event and fires its pg_notify
// within the given transaction, mirroring the framework EventDao.Create but
// bound to tx so the event and the state change that produced it commit
// atomically. The events table plus NOTIFY is the outbox the EventBroker
// consumes; Postgres buffers pg_notify until commit, so a rolled-back
// transaction emits nothing.
func emitGatewayEventTx(tx *gorm.DB, gatewayID string) error {
	event := &api.Event{
		Source:    "Gateways",
		SourceID:  gatewayID,
		EventType: api.UpdateEventType,
	}
	if err := tx.Omit(clause.Associations).Create(event).Error; err != nil {
		return err
	}
	return tx.Exec("select pg_notify('events', ?)", event.ID).Error
}

func derefCount(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
