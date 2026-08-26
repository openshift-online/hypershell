package gateways

import (
	"context"
	"errors"
	"testing"
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

	gw := &Gateway{Name: "gw1", FleetId: "fleet-1", DatabaseId: "client-supplied-db-id"}
	if err := placement.Resolve(context.Background(), gw); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if gw.DatabaseId != "server-created-db-id" {
		t.Fatalf("DatabaseId = %q, want server-created ID", gw.DatabaseId)
	}
	if creator.calls != 1 {
		t.Fatalf("CreateForGateway called %d times, want 1", creator.calls)
	}
	if gw.FleetId != "fleet-1" {
		t.Fatalf("FleetId = %q, want API value left unchanged", gw.FleetId)
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
// and never trusts a caller-supplied relationship ID or uses fleet_id.
func TestCNPGPlacementIgnoresExplicitDatabaseID(t *testing.T) {
	dbs := &fakeDatabaseLookup{sole: "resolved-db-id"}
	placement := NewCNPGPlacement(dbs)

	gw := &Gateway{Name: "gw1", FleetId: "fleet-1", DatabaseId: "client-supplied-db-id"}
	if err := placement.Resolve(context.Background(), gw); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if gw.DatabaseId != "resolved-db-id" {
		t.Fatalf("DatabaseId = %q, want resolved database", gw.DatabaseId)
	}
	if gw.FleetId != "fleet-1" {
		t.Fatalf("FleetId = %q, want API value left unchanged", gw.FleetId)
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
	if gw.FleetId != "" {
		t.Fatalf("FleetId = %q, want no Fleet resolution from cluster_id", gw.FleetId)
	}
}

// CNPG placement rejects zero or multiple global ManagedDatabases rather than
// using fleet_id to choose one.
func TestCNPGPlacementRejectsAmbiguousDatabases(t *testing.T) {
	dbs := &fakeDatabaseLookup{}
	placement := NewCNPGPlacement(dbs)

	gw := &Gateway{Name: "gw1", FleetId: "fleet-1"}
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
	err := placement.Resolve(context.Background(), &Gateway{Name: "gw1", FleetId: "fleet-1"})
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
