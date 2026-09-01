package gatewayProfiles

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
)

// noopEventService satisfies services.EventService for unit tests that do not
// exercise event delivery. It counts Create calls so tests can assert the
// service emitted the expected lifecycle event.
type noopEventService struct{ createCount int }

func (n *noopEventService) Get(_ context.Context, _ string) (*api.Event, *errors.ServiceError) {
	return nil, nil
}
func (n *noopEventService) Create(_ context.Context, ev *api.Event) (*api.Event, *errors.ServiceError) {
	n.createCount++
	return ev, nil
}
func (n *noopEventService) Replace(_ context.Context, ev *api.Event) (*api.Event, *errors.ServiceError) {
	return ev, nil
}
func (n *noopEventService) Delete(_ context.Context, _ string) *errors.ServiceError { return nil }
func (n *noopEventService) All(_ context.Context) (api.EventList, *errors.ServiceError) {
	return nil, nil
}
func (n *noopEventService) FindByIDs(_ context.Context, _ []string) (api.EventList, *errors.ServiceError) {
	return nil, nil
}
func (n *noopEventService) FindUnreconciled(_ context.Context, _ time.Duration) (api.EventList, *errors.ServiceError) {
	return nil, nil
}
func (n *noopEventService) FindBySourceAndType(_ context.Context, _ string, _ api.EventType) (api.EventList, *errors.ServiceError) {
	return nil, nil
}

// stubLockFactory satisfies db.LockFactory without a database so the Replace
// advisory-lock path can be exercised in a unit test.
type stubLockFactory struct{}

func (stubLockFactory) NewAdvisoryLock(_ context.Context, _ string, _ db.LockType) (string, error) {
	return "owner", nil
}
func (stubLockFactory) NewNonBlockingLock(_ context.Context, _ string, _ db.LockType) (string, bool, error) {
	return "owner", true, nil
}
func (stubLockFactory) Unlock(_ context.Context, _ string) {}

// testProfileDao wraps the shipped mock_dao.go so unit tests reuse its
// Get/Create/All behaviour, while supplying working Delete/Replace
// implementations (the shipped mock returns NotImplemented for those) and
// configurable reference facts for deletion-protection tests.
type testProfileDao struct {
	*gatewayProfileDaoMock
	clusterReferenced bool
	gatewayReferenced bool
}

func newTestProfileDao() *testProfileDao {
	return &testProfileDao{gatewayProfileDaoMock: NewMockGatewayProfileDao()}
}

func (d *testProfileDao) Delete(_ context.Context, id string) error {
	kept := GatewayProfileList{}
	for _, p := range d.gatewayProfiles {
		if p.ID != id {
			kept = append(kept, p)
		}
	}
	d.gatewayProfiles = kept
	return nil
}

func (d *testProfileDao) Replace(_ context.Context, p *GatewayProfile) (*GatewayProfile, error) {
	for i, existing := range d.gatewayProfiles {
		if existing.ID == p.ID {
			d.gatewayProfiles[i] = p
			return p, nil
		}
	}
	d.gatewayProfiles = append(d.gatewayProfiles, p)
	return p, nil
}

func (d *testProfileDao) ExistsByClusterProfileID(_ context.Context, _ string) (bool, error) {
	return d.clusterReferenced, nil
}

func (d *testProfileDao) ExistsByGatewayProfileID(_ context.Context, _ string) (bool, error) {
	return d.gatewayReferenced, nil
}

func newTestService(dao GatewayProfileDao, events *noopEventService) GatewayProfileService {
	return NewGatewayProfileService(stubLockFactory{}, dao, events)
}

// seed inserts a profile with a fixed ID directly through the mock so lookups
// (which match on ID) resolve; the mock does not run the BeforeCreate hook.
func seed(dao *testProfileDao, id, name string) *GatewayProfile {
	p := &GatewayProfile{Name: name}
	p.ID = id
	dao.gatewayProfiles = append(dao.gatewayProfiles, p)
	return p
}

func TestGatewayProfileService_Create(t *testing.T) {
	t.Run("valid profile is persisted and emits a create event", func(t *testing.T) {
		dao := newTestProfileDao()
		events := &noopEventService{}
		svc := newTestService(dao, events)

		profile := &GatewayProfile{Name: "small", CpuRequestTotal: strPtr("2"), MemoryLimitTotal: strPtr("8Gi")}
		profile.ID = "p-create"

		created, svcErr := svc.Create(context.Background(), profile)
		if svcErr != nil {
			t.Fatalf("Create() unexpected error: %v", svcErr)
		}
		if created.Name != "small" {
			t.Fatalf("Create() name = %q, want %q", created.Name, "small")
		}
		if events.createCount != 1 {
			t.Fatalf("Create() emitted %d events, want 1", events.createCount)
		}

		got, getErr := svc.Get(context.Background(), "p-create")
		if getErr != nil {
			t.Fatalf("Get() after Create() unexpected error: %v", getErr)
		}
		if got.Name != "small" {
			t.Fatalf("Get() name = %q, want %q", got.Name, "small")
		}
	})

	t.Run("invalid quantity is rejected before persistence", func(t *testing.T) {
		dao := newTestProfileDao()
		events := &noopEventService{}
		svc := newTestService(dao, events)

		_, svcErr := svc.Create(context.Background(), &GatewayProfile{Name: "bad", MemoryLimitTotal: strPtr("not-a-quantity")})
		if svcErr == nil {
			t.Fatal("Create() = nil error, want HTTP 400 for invalid quantity")
		}
		if svcErr.HttpCode != http.StatusBadRequest {
			t.Fatalf("Create() HttpCode = %d, want 400", svcErr.HttpCode)
		}
		all, _ := svc.All(context.Background())
		if len(all) != 0 {
			t.Fatalf("invalid profile persisted; All() len = %d, want 0", len(all))
		}
		if events.createCount != 0 {
			t.Fatalf("invalid profile emitted %d events, want 0", events.createCount)
		}
	})
}

func TestGatewayProfileService_Get_NotFound(t *testing.T) {
	svc := newTestService(newTestProfileDao(), &noopEventService{})

	_, svcErr := svc.Get(context.Background(), "missing")
	if svcErr == nil {
		t.Fatal("Get() = nil error, want not-found")
	}
	if svcErr.HttpCode != http.StatusNotFound {
		t.Fatalf("Get() HttpCode = %d, want 404", svcErr.HttpCode)
	}
}

func TestGatewayProfileService_Replace(t *testing.T) {
	dao := newTestProfileDao()
	events := &noopEventService{}
	svc := newTestService(dao, events)

	seed(dao, "p-1", "before")

	updated := &GatewayProfile{Name: "after", CpuRequestTotal: strPtr("4")}
	updated.ID = "p-1"

	got, svcErr := svc.Replace(context.Background(), updated)
	if svcErr != nil {
		t.Fatalf("Replace() unexpected error: %v", svcErr)
	}
	if got.Name != "after" {
		t.Fatalf("Replace() name = %q, want %q", got.Name, "after")
	}
	if events.createCount != 1 {
		t.Fatalf("Replace() emitted %d events, want 1", events.createCount)
	}

	roundTrip, _ := svc.Get(context.Background(), "p-1")
	if roundTrip.Name != "after" {
		t.Fatalf("Get() after Replace() name = %q, want %q", roundTrip.Name, "after")
	}
}

func TestGatewayProfileService_Replace_ValidatesFields(t *testing.T) {
	dao := newTestProfileDao()
	svc := newTestService(dao, &noopEventService{})
	seed(dao, "p-1", "before")

	bad := &GatewayProfile{Name: "before", MemoryLimitTotal: strPtr("2GB")}
	bad.ID = "p-1"

	_, svcErr := svc.Replace(context.Background(), bad)
	if svcErr == nil {
		t.Fatal("Replace() = nil error, want HTTP 400 for invalid quantity")
	}
	if svcErr.HttpCode != http.StatusBadRequest {
		t.Fatalf("Replace() HttpCode = %d, want 400", svcErr.HttpCode)
	}
}

func TestGatewayProfileService_Delete(t *testing.T) {
	t.Run("unreferenced profile is deleted", func(t *testing.T) {
		dao := newTestProfileDao()
		events := &noopEventService{}
		svc := newTestService(dao, events)
		seed(dao, "p-free", "unused")

		if svcErr := svc.Delete(context.Background(), "p-free"); svcErr != nil {
			t.Fatalf("Delete() unexpected error: %v", svcErr)
		}
		if events.createCount != 1 {
			t.Fatalf("Delete() emitted %d events, want 1", events.createCount)
		}
		if _, getErr := svc.Get(context.Background(), "p-free"); getErr == nil {
			t.Fatal("Get() after Delete() succeeded, want not-found")
		}
	})

	t.Run("missing profile returns not-found", func(t *testing.T) {
		svc := newTestService(newTestProfileDao(), &noopEventService{})
		svcErr := svc.Delete(context.Background(), "nope")
		if svcErr == nil {
			t.Fatal("Delete() = nil error, want not-found")
		}
		if svcErr.HttpCode != http.StatusNotFound {
			t.Fatalf("Delete() HttpCode = %d, want 404", svcErr.HttpCode)
		}
	})

	t.Run("profile referenced by a cluster default is protected", func(t *testing.T) {
		dao := newTestProfileDao()
		dao.clusterReferenced = true
		svc := newTestService(dao, &noopEventService{})
		seed(dao, "p-ref", "in-use")

		svcErr := svc.Delete(context.Background(), "p-ref")
		if svcErr == nil {
			t.Fatal("Delete() = nil error, want HTTP 409 when referenced by a cluster")
		}
		if svcErr.HttpCode != http.StatusConflict {
			t.Fatalf("Delete() HttpCode = %d, want 409", svcErr.HttpCode)
		}
	})

	t.Run("profile referenced by a gateway is protected", func(t *testing.T) {
		dao := newTestProfileDao()
		dao.gatewayReferenced = true
		svc := newTestService(dao, &noopEventService{})
		seed(dao, "p-ref", "in-use")

		svcErr := svc.Delete(context.Background(), "p-ref")
		if svcErr == nil {
			t.Fatal("Delete() = nil error, want HTTP 409 when referenced by a gateway")
		}
		if svcErr.HttpCode != http.StatusConflict {
			t.Fatalf("Delete() HttpCode = %d, want 409", svcErr.HttpCode)
		}
	})
}
