package gateways

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/plugins/managedDatabases"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
)

type fakeDatabaseSelector struct {
	oldest    string
	oldestErr error
	calls     int
}

func (f *fakeDatabaseSelector) FindOldest(ctx context.Context) (string, error) {
	f.calls++
	if f.oldestErr != nil {
		return "", f.oldestErr
	}
	return f.oldest, nil
}

type fakePlacementResolver struct {
	err error
}

func (f fakePlacementResolver) Resolve(context.Context, *Gateway) error {
	return f.err
}

// Placement owns database_id: a caller-supplied value is discarded and replaced
// with the server-side selection.
func TestPlacementIgnoresExplicitDatabaseID(t *testing.T) {
	dbs := &fakeDatabaseSelector{oldest: "oldest-db-id"}
	placement := NewDatabasePlacement(dbs)

	gw := &Gateway{Name: "gw1", DatabaseId: "client-supplied-db-id"}
	if err := placement.Resolve(context.Background(), gw); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if gw.DatabaseId != "oldest-db-id" {
		t.Fatalf("DatabaseId = %q, want the server-selected ID", gw.DatabaseId)
	}
}

// More than one registered ManagedDatabase is NOT an error: placement picks the
// first-created one rather than rejecting the gateway creation.
func TestPlacementAcceptsMultipleDatabases(t *testing.T) {
	dbs := &fakeDatabaseSelector{oldest: "first-created"}
	placement := NewDatabasePlacement(dbs)

	gw := &Gateway{Name: "gw1"}
	if err := placement.Resolve(context.Background(), gw); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if gw.DatabaseId != "first-created" {
		t.Fatalf("DatabaseId = %q, want %q", gw.DatabaseId, "first-created")
	}
	if dbs.calls != 1 {
		t.Fatalf("FindOldest called %d times, want 1", dbs.calls)
	}
}

// Zero registered ManagedDatabases is rejected as a validation error, so the API
// returns 400 rather than 500.
func TestPlacementRejectsWhenNoneRegistered(t *testing.T) {
	placement := NewDatabasePlacement(&fakeDatabaseSelector{oldest: ""})

	err := placement.Resolve(context.Background(), &Gateway{Name: "gw1"})
	if err == nil {
		t.Fatal("Resolve() = nil, want an error when no ManagedDatabase is registered")
	}
	if !IsPlacementValidationError(err) {
		t.Fatalf("Resolve() error = %v, want validation classification", err)
	}
}

func TestPlacementLookupFailureIsDependencyError(t *testing.T) {
	placement := NewDatabasePlacement(&fakeDatabaseSelector{oldestErr: errors.New("database unavailable")})

	err := placement.Resolve(context.Background(), &Gateway{Name: "gw1"})
	if err == nil {
		t.Fatal("Resolve() = nil, want lookup error")
	}
	if IsPlacementValidationError(err) {
		t.Fatalf("Resolve() error = %v, want dependency classification", err)
	}
}

func TestGatewayServiceMapsPlacementErrors(t *testing.T) {
	t.Run("validation failure is a bad request", func(t *testing.T) {
		svc := &sqlGatewayService{placement: fakePlacementResolver{err: newPlacementValidationError("no ManagedDatabase is registered")}}
		_, svcErr := svc.Create(context.Background(), &Gateway{DatabaseId: "client-value"})
		if svcErr == nil || svcErr.HttpCode != 400 {
			t.Fatalf("Create() error = %#v, want HTTP 400", svcErr)
		}
	})

	t.Run("dependency failure is an internal error", func(t *testing.T) {
		svc := &sqlGatewayService{placement: fakePlacementResolver{err: newPlacementDependencyError("resolve database", errors.New("database unavailable"))}}
		_, svcErr := svc.Create(context.Background(), &Gateway{DatabaseId: "client-value"})
		if svcErr == nil || svcErr.HttpCode != 500 {
			t.Fatalf("Create() error = %#v, want HTTP 500", svcErr)
		}
	})
}

// --- pickOldestManagedDatabase ---

func mdb(id string, createdAt time.Time) *managedDatabases.ManagedDatabase {
	return &managedDatabases.ManagedDatabase{
		Meta: api.Meta{ID: id, CreatedAt: createdAt},
	}
}

func TestPickOldestManagedDatabase(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	t2 := t0.Add(2 * time.Hour)

	t.Run("empty list returns empty", func(t *testing.T) {
		if got := pickOldestManagedDatabase(nil); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})

	t.Run("single candidate", func(t *testing.T) {
		list := managedDatabases.ManagedDatabaseList{mdb("a", t1)}
		if got := pickOldestManagedDatabase(list); got != "a" {
			t.Fatalf("got %q, want %q", got, "a")
		}
	})

	t.Run("picks earliest created regardless of list order", func(t *testing.T) {
		list := managedDatabases.ManagedDatabaseList{
			mdb("newest", t2),
			mdb("oldest", t0),
			mdb("middle", t1),
		}
		if got := pickOldestManagedDatabase(list); got != "oldest" {
			t.Fatalf("got %q, want %q", got, "oldest")
		}
	})

	t.Run("ties broken by ID ascending", func(t *testing.T) {
		list := managedDatabases.ManagedDatabaseList{
			mdb("bbb", t0),
			mdb("aaa", t0),
		}
		if got := pickOldestManagedDatabase(list); got != "aaa" {
			t.Fatalf("got %q, want %q", got, "aaa")
		}
	})
}
