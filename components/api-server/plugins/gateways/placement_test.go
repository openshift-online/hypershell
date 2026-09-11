package gateways

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/plugins/managedDatabases"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
)

type fakeDatabaseLookup struct {
	sole    string
	soleErr error
}

func (f *fakeDatabaseLookup) FindSole(ctx context.Context) (string, error) {
	if f.soleErr != nil {
		return "", f.soleErr
	}
	return f.sole, nil
}

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

type fakeDatabaseCreator struct {
	created  string
	err      error
	calls    int
	lastName string
}

func (f *fakeDatabaseCreator) CreateForGateway(ctx context.Context, gatewayName string) (string, error) {
	f.calls++
	f.lastName = gatewayName
	if f.err != nil {
		return "", f.err
	}
	return f.created, nil
}

// Deployment placement owns database_id: a caller-supplied value is discarded
// and a new dedicated ManagedDatabase is always created.
func TestDeploymentPlacementIgnoresExplicitDatabaseID(t *testing.T) {
	creator := &fakeDatabaseCreator{created: "server-created-db-id"}
	placement := NewDeploymentPlacement(creator)

	gw := &Gateway{Name: "gw1", DatabaseId: "client-supplied-db-id"}
	if err := placement.Resolve(context.Background(), gw); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if gw.DatabaseId != "server-created-db-id" {
		t.Fatalf("DatabaseId = %q, want server-created ID", gw.DatabaseId)
	}
	if creator.calls != 1 {
		t.Fatalf("CreateForGateway called %d times, want 1", creator.calls)
	}
}

// TestDeploymentPlacementAutoCreatesPerGatewayDatabase covers the
// deployment-mode default: a blank database_id causes the API server to
// auto-create a dedicated ManagedDatabase for the gateway, requiring no
// pre-existing ManagedDatabase and no CNPG APIs.
func TestDeploymentPlacementAutoCreatesPerGatewayDatabase(t *testing.T) {
	creator := &fakeDatabaseCreator{created: "new-db-id"}
	placement := NewDeploymentPlacement(creator)

	gw := &Gateway{Name: "gw1"}
	if err := placement.Resolve(context.Background(), gw); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if gw.DatabaseId != "new-db-id" {
		t.Fatalf("DatabaseId = %q, want %q", gw.DatabaseId, "new-db-id")
	}
	if creator.calls != 1 {
		t.Fatalf("CreateForGateway called %d times, want 1", creator.calls)
	}
	if creator.lastName != "gw1" {
		t.Fatalf("CreateForGateway called with %q, want %q", creator.lastName, "gw1")
	}
}

// CNPG placement also owns database_id. It resolves the sole database globally
// and never trusts a caller-supplied relationship ID.
func TestCNPGPlacementIgnoresExplicitDatabaseID(t *testing.T) {
	dbs := &fakeDatabaseLookup{sole: "resolved-db-id"}
	placement := NewCNPGPlacement(dbs)

	gw := &Gateway{Name: "gw1", DatabaseId: "client-supplied-db-id"}
	if err := placement.Resolve(context.Background(), gw); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if gw.DatabaseId != "resolved-db-id" {
		t.Fatalf("DatabaseId = %q, want resolved database", gw.DatabaseId)
	}
}

func TestCNPGPlacementResolvesSoleDatabase(t *testing.T) {
	dbs := &fakeDatabaseLookup{sole: "db-1"}
	placement := NewCNPGPlacement(dbs)

	gw := &Gateway{Name: "gw1", ClusterId: "cluster-1"}
	if err := placement.Resolve(context.Background(), gw); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if gw.DatabaseId != "db-1" {
		t.Fatalf("DatabaseId = %q, want %q", gw.DatabaseId, "db-1")
	}
}

// CNPG placement rejects zero or multiple global ManagedDatabases.
func TestCNPGPlacementRejectsAmbiguousDatabases(t *testing.T) {
	dbs := &fakeDatabaseLookup{}
	placement := NewCNPGPlacement(dbs)

	gw := &Gateway{Name: "gw1"}
	err := placement.Resolve(context.Background(), gw)
	if err == nil {
		t.Fatal("Resolve() = nil error, want an error when zero or multiple ManagedDatabases exist")
	}
}

func TestPlacementValidationErrorClassification(t *testing.T) {
	placement := NewCNPGPlacement(&fakeDatabaseLookup{})
	err := placement.Resolve(context.Background(), &Gateway{Name: "gw1"})
	if err == nil {
		t.Fatal("Resolve() = nil, want ambiguous-database validation error")
	}
	if !IsPlacementValidationError(err) {
		t.Fatalf("Resolve() error = %v, want validation classification", err)
	}
}

func TestPlacementDatabaseCreationErrorClassification(t *testing.T) {
	placement := NewDeploymentPlacement(&fakeDatabaseCreator{err: errors.New("database unavailable")})
	err := placement.Resolve(context.Background(), &Gateway{Name: "gw1"})
	if err == nil {
		t.Fatal("Resolve() = nil, want creation error")
	}
	if IsPlacementValidationError(err) {
		t.Fatalf("Resolve() error = %v, want dependency classification", err)
	}
}

func TestGatewayServiceMapsPlacementErrors(t *testing.T) {
	t.Run("validation failure is a bad request", func(t *testing.T) {
		svc := &sqlGatewayService{placement: fakePlacementResolver{err: newPlacementValidationError("database is ambiguous")}}
		_, svcErr := svc.Create(context.Background(), &Gateway{DatabaseId: "client-value"})
		if svcErr == nil || svcErr.HttpCode != 400 {
			t.Fatalf("Create() error = %#v, want HTTP 400", svcErr)
		}
	})

	t.Run("dependency failure is an internal error", func(t *testing.T) {
		svc := &sqlGatewayService{placement: fakePlacementResolver{err: newPlacementDependencyError("create database", errors.New("database unavailable"))}}
		_, svcErr := svc.Create(context.Background(), &Gateway{DatabaseId: "client-value"})
		if svcErr == nil || svcErr.HttpCode != 500 {
			t.Fatalf("Create() error = %#v, want HTTP 500", svcErr)
		}
	})
}

// --- external placement: oldest-wins ---

// External placement owns database_id like every other mode: a caller-supplied
// value is discarded and replaced with the server-side selection.
func TestExternalPlacementIgnoresExplicitDatabaseID(t *testing.T) {
	dbs := &fakeDatabaseSelector{oldest: "oldest-db-id"}
	placement := NewExternalPlacement(dbs)

	gw := &Gateway{Name: "gw1", DatabaseId: "client-supplied-db-id"}
	if err := placement.Resolve(context.Background(), gw); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if gw.DatabaseId != "oldest-db-id" {
		t.Fatalf("DatabaseId = %q, want the server-selected ID", gw.DatabaseId)
	}
}

// More than one registered external ManagedDatabase is NOT an error: placement
// picks the first-created one rather than rejecting the gateway creation.
func TestExternalPlacementAcceptsMultipleDatabases(t *testing.T) {
	dbs := &fakeDatabaseSelector{oldest: "first-created"}
	placement := NewExternalPlacement(dbs)

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

// Zero registered external ManagedDatabases is still rejected, as a validation
// error so the API returns 400 rather than 500.
func TestExternalPlacementRejectsWhenNoneRegistered(t *testing.T) {
	placement := NewExternalPlacement(&fakeDatabaseSelector{oldest: ""})

	err := placement.Resolve(context.Background(), &Gateway{Name: "gw1"})
	if err == nil {
		t.Fatal("Resolve() = nil, want an error when no external ManagedDatabase is registered")
	}
	if !IsPlacementValidationError(err) {
		t.Fatalf("Resolve() error = %v, want validation classification", err)
	}
}

func TestExternalPlacementLookupFailureIsDependencyError(t *testing.T) {
	placement := NewExternalPlacement(&fakeDatabaseSelector{oldestErr: errors.New("database unavailable")})

	err := placement.Resolve(context.Background(), &Gateway{Name: "gw1"})
	if err == nil {
		t.Fatal("Resolve() = nil, want lookup error")
	}
	if IsPlacementValidationError(err) {
		t.Fatalf("Resolve() error = %v, want dependency classification", err)
	}
}

// --- pickOldestManagedDatabase ---

func mdb(id, provider string, createdAt time.Time) *managedDatabases.ManagedDatabase {
	return &managedDatabases.ManagedDatabase{
		Meta:     api.Meta{ID: id, CreatedAt: createdAt},
		Provider: provider,
	}
}

func TestPickOldestManagedDatabase(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	t2 := t0.Add(2 * time.Hour)

	t.Run("empty list returns empty", func(t *testing.T) {
		if got := pickOldestManagedDatabase(nil, ProviderExternal); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})

	t.Run("single candidate", func(t *testing.T) {
		list := managedDatabases.ManagedDatabaseList{mdb("a", ProviderExternal, t1)}
		if got := pickOldestManagedDatabase(list, ProviderExternal); got != "a" {
			t.Fatalf("got %q, want %q", got, "a")
		}
	})

	t.Run("picks earliest created regardless of list order", func(t *testing.T) {
		list := managedDatabases.ManagedDatabaseList{
			mdb("newest", ProviderExternal, t2),
			mdb("oldest", ProviderExternal, t0),
			mdb("middle", ProviderExternal, t1),
		}
		if got := pickOldestManagedDatabase(list, ProviderExternal); got != "oldest" {
			t.Fatalf("got %q, want %q", got, "oldest")
		}
	})

	t.Run("ties broken by ID ascending", func(t *testing.T) {
		list := managedDatabases.ManagedDatabaseList{
			mdb("bbb", ProviderExternal, t0),
			mdb("aaa", ProviderExternal, t0),
		}
		if got := pickOldestManagedDatabase(list, ProviderExternal); got != "aaa" {
			t.Fatalf("got %q, want %q", got, "aaa")
		}
	})

	t.Run("filters by provider", func(t *testing.T) {
		list := managedDatabases.ManagedDatabaseList{
			mdb("cnpg-older", ProviderCNPG, t0),
			mdb("external-newer", ProviderExternal, t2),
		}
		if got := pickOldestManagedDatabase(list, ProviderExternal); got != "external-newer" {
			t.Fatalf("got %q, want %q", got, "external-newer")
		}
	})

	t.Run("no candidate of the requested provider", func(t *testing.T) {
		list := managedDatabases.ManagedDatabaseList{mdb("c", ProviderCNPG, t0)}
		if got := pickOldestManagedDatabase(list, ProviderExternal); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}
