package defaultWorkspaceMemberships

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	trexerrors "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// fakeDao is an in-memory Dao for service-layer unit tests, keyed by
// (gateway_id, subject) for the live uniqueness check (PLAT-DATA-27.01).
type fakeDao struct {
	items  map[string]*DefaultWorkspaceMembership
	audits []*AuditEvent
}

func newFakeDao() *fakeDao {
	return &fakeDao{items: map[string]*DefaultWorkspaceMembership{}}
}

func (d *fakeDao) Get(ctx context.Context, id string) (*DefaultWorkspaceMembership, error) {
	if item, ok := d.items[id]; ok {
		return item, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *fakeDao) GetUnscoped(ctx context.Context, id string) (*DefaultWorkspaceMembership, error) {
	return d.Get(ctx, id)
}

func (d *fakeDao) Create(ctx context.Context, m *DefaultWorkspaceMembership) (*DefaultWorkspaceMembership, error) {
	if m.ID == "" {
		m.ID = "id-" + m.GatewayID + "-" + m.Subject
	}
	cp := *m
	d.items[cp.ID] = &cp
	return &cp, nil
}

func (d *fakeDao) UpdateStatus(ctx context.Context, id, status string, lastErr *string) (*DefaultWorkspaceMembership, error) {
	item, ok := d.items[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	item.ReconciliationStatus = status
	item.LastReconcileError = lastErr
	return item, nil
}

func (d *fakeDao) Delete(ctx context.Context, id string) error {
	if _, ok := d.items[id]; !ok {
		return gorm.ErrRecordNotFound
	}
	delete(d.items, id)
	return nil
}

func (d *fakeDao) FindByGatewayAndSubject(ctx context.Context, gatewayID, subject string) (*DefaultWorkspaceMembership, error) {
	for _, item := range d.items {
		if item.GatewayID == gatewayID && item.Subject == subject {
			return item, nil
		}
	}
	return nil, nil
}

func (d *fakeDao) CountLiveForGateway(ctx context.Context, gatewayID string) (int64, error) {
	var count int64
	for _, item := range d.items {
		if item.GatewayID == gatewayID {
			count++
		}
	}
	return count, nil
}

func (d *fakeDao) AllForGateway(ctx context.Context, gatewayID string) (DefaultWorkspaceMembershipList, error) {
	var out DefaultWorkspaceMembershipList
	for _, item := range d.items {
		if item.GatewayID == gatewayID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (d *fakeDao) DeleteAllForGateway(ctx context.Context, gatewayID string) (int64, error) {
	var count int64
	for id, item := range d.items {
		if item.GatewayID == gatewayID {
			delete(d.items, id)
			count++
		}
	}
	return count, nil
}

func (d *fakeDao) FindByIDs(ctx context.Context, ids []string) (DefaultWorkspaceMembershipList, error) {
	var out DefaultWorkspaceMembershipList
	for _, id := range ids {
		if item, ok := d.items[id]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func (d *fakeDao) All(ctx context.Context) (DefaultWorkspaceMembershipList, error) {
	var out DefaultWorkspaceMembershipList
	for _, item := range d.items {
		out = append(out, item)
	}
	return out, nil
}

func (d *fakeDao) CountByStatus(ctx context.Context) (map[string]int64, error) {
	counts := map[string]int64{}
	for _, item := range d.items {
		counts[item.ReconciliationStatus]++
	}
	return counts, nil
}

func (d *fakeDao) CreateAudit(ctx context.Context, event *AuditEvent) error {
	d.audits = append(d.audits, event)
	return nil
}

type noopEvents struct{}

func (noopEvents) Create(ctx context.Context, e *api.Event) (*api.Event, *trexerrors.ServiceError) {
	return e, nil
}
func (noopEvents) Get(ctx context.Context, id string) (*api.Event, *trexerrors.ServiceError) {
	return nil, nil
}
func (noopEvents) Replace(ctx context.Context, e *api.Event) (*api.Event, *trexerrors.ServiceError) {
	return e, nil
}
func (noopEvents) Delete(ctx context.Context, id string) *trexerrors.ServiceError { return nil }
func (noopEvents) All(ctx context.Context) (api.EventList, *trexerrors.ServiceError) {
	return nil, nil
}
func (noopEvents) FindByIDs(ctx context.Context, ids []string) (api.EventList, *trexerrors.ServiceError) {
	return nil, nil
}
func (noopEvents) FindUnreconciled(ctx context.Context, olderThan time.Duration) (api.EventList, *trexerrors.ServiceError) {
	return nil, nil
}
func (noopEvents) FindBySourceAndType(ctx context.Context, source string, eventType api.EventType) (api.EventList, *trexerrors.ServiceError) {
	return nil, nil
}

// Verifies: PLAT-DATA-27.02
func TestCreate_InvalidSubjectIsRejected(t *testing.T) {
	svc := NewService(newFakeDao(), noopEvents{})
	_, err := svc.Create(context.Background(), "gw-1", "bad subject!", SourceGatewayAccess)
	if err == nil || err.Code != trexerrors.ErrorValidation {
		t.Fatalf("expected validation error for malformed subject, got %v", err)
	}
}

func TestCreate_InvalidSourceIsRejected(t *testing.T) {
	svc := NewService(newFakeDao(), noopEvents{})
	_, err := svc.Create(context.Background(), "gw-1", "dana", "bogus")
	if err == nil || err.Code != trexerrors.ErrorValidation {
		t.Fatalf("expected validation error for invalid source, got %v", err)
	}
}

// Verifies: PLAT-DATA-27.01
func TestCreate_IsIdempotentByGatewayAndSubject(t *testing.T) {
	dao := newFakeDao()
	svc := NewService(dao, noopEvents{})
	ctx := context.Background()

	first, err := svc.Create(ctx, "gw-1", "dana", SourceGatewayAccess)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := svc.Create(ctx, "gw-1", "dana", SourceGatewayAccess)
	if err != nil {
		t.Fatalf("unexpected error on re-create: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected idempotent create to return the same row, got %s and %s", first.ID, second.ID)
	}
	if len(dao.items) != 1 {
		t.Fatalf("expected exactly one live row, got %d", len(dao.items))
	}
	if len(dao.audits) != 1 {
		t.Fatalf("expected exactly one audit event for the single real create, got %d", len(dao.audits))
	}
}

func TestCreate_DefaultsRoleAndStatus(t *testing.T) {
	dao := newFakeDao()
	svc := NewService(dao, noopEvents{})
	item, err := svc.Create(context.Background(), "gw-1", "dana", SourceServiceAccount)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Role != RoleUser {
		t.Fatalf("expected role %q, got %q", RoleUser, item.Role)
	}
	if item.ReconciliationStatus != StatusPending {
		t.Fatalf("expected status %q, got %q", StatusPending, item.ReconciliationStatus)
	}
}

// Delete is idempotent.
// Verifies: OSG-SACC-19.05
func TestDeleteByGatewayAndSubject_AbsentIsNoop(t *testing.T) {
	svc := NewService(newFakeDao(), noopEvents{})
	if err := svc.DeleteByGatewayAndSubject(context.Background(), "gw-1", "nobody"); err != nil {
		t.Fatalf("expected no-op success deleting an absent membership, got %v", err)
	}
}

func TestDeleteByGatewayAndSubject_RemovesLiveRow(t *testing.T) {
	dao := newFakeDao()
	svc := NewService(dao, noopEvents{})
	ctx := context.Background()
	if _, err := svc.Create(ctx, "gw-1", "dana", SourceGatewayAccess); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := svc.DeleteByGatewayAndSubject(ctx, "gw-1", "dana"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found, err := svc.FindByGatewayAndSubject(ctx, "gw-1", "dana")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != nil {
		t.Fatalf("expected membership to be gone after delete, found %+v", found)
	}
}

// Count backs the member-limit check.
// Verifies: GTW-ACC-17, OSG-SACC-19
func TestCountLiveForGateway_CountsOnlyThatGateway(t *testing.T) {
	dao := newFakeDao()
	svc := NewService(dao, noopEvents{})
	ctx := context.Background()
	if _, err := svc.Create(ctx, "gw-1", "a", SourceGatewayAccess); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.Create(ctx, "gw-1", "b", SourceServiceAccount); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.Create(ctx, "gw-2", "c", SourceGatewayAccess); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count, err := svc.CountLiveForGateway(ctx, "gw-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2 for gw-1, got %d", count)
	}
}

// Verifies: PLAT-DATA-27.05
func TestDeleteAllForGateway_RemovesOnlyThatGatewaysRows(t *testing.T) {
	dao := newFakeDao()
	svc := NewService(dao, noopEvents{})
	ctx := context.Background()
	if _, err := svc.Create(ctx, "gw-1", "a", SourceGatewayAccess); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.Create(ctx, "gw-1", "b", SourceServiceAccount); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.Create(ctx, "gw-2", "c", SourceGatewayAccess); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count, err := svc.DeleteAllForGateway(ctx, "gw-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows deleted, got %d", count)
	}
	remaining, err := svc.AllForGateway(ctx, "gw-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected gw-2's row to survive, got %d rows", len(remaining))
	}
}

// CTRL-CORE-16 status write-back.
// Verifies: CTRL-CORE-16
func TestUpdateStatus_RejectsUnknownStatus(t *testing.T) {
	dao := newFakeDao()
	svc := NewService(dao, noopEvents{})
	ctx := context.Background()
	item, err := svc.Create(ctx, "gw-1", "dana", SourceGatewayAccess)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.UpdateStatus(ctx, item.ID, "bogus", nil); err == nil || err.Code != trexerrors.ErrorValidation {
		t.Fatalf("expected validation error for unknown status, got %v", err)
	}
}

func TestUpdateStatus_TransitionsPendingToReady(t *testing.T) {
	dao := newFakeDao()
	svc := NewService(dao, noopEvents{})
	ctx := context.Background()
	item, err := svc.Create(ctx, "gw-1", "dana", SourceGatewayAccess)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	updated, err := svc.UpdateStatus(ctx, item.ID, StatusReady, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.ReconciliationStatus != StatusReady {
		t.Fatalf("expected status %q, got %q", StatusReady, updated.ReconciliationStatus)
	}
}
