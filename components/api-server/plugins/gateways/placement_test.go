package gateways

import (
	"context"
	"errors"
	"testing"
)

type fakeFleetLookup struct {
	fleetForCluster map[string]string
	soleFleet       string
	soleFleetErr    error
	clusterErr      error
}

func (f *fakeFleetLookup) FleetIDForCluster(ctx context.Context, clusterID string) (string, error) {
	if f.clusterErr != nil {
		return "", f.clusterErr
	}
	return f.fleetForCluster[clusterID], nil
}

func (f *fakeFleetLookup) FindSoleFleet(ctx context.Context) (string, error) {
	return f.soleFleet, f.soleFleetErr
}

type fakeDatabaseLookup struct {
	soleInFleet map[string]string
	sole        string
	soleFleetID string
	soleErr     error
}

func (f *fakeDatabaseLookup) FindSoleInFleet(ctx context.Context, fleetID string) (string, error) {
	return f.soleInFleet[fleetID], nil
}

func (f *fakeDatabaseLookup) FindSole(ctx context.Context) (string, string, error) {
	if f.soleErr != nil {
		return "", "", f.soleErr
	}
	return f.sole, f.soleFleetID, nil
}

type fakePlacementResolver struct {
	err error
}

func (f fakePlacementResolver) Resolve(context.Context, *Gateway) error {
	return f.err
}

type fakeDatabaseCreator struct {
	created   string
	err       error
	calls     int
	lastName  string
	lastFleet string
}

func (f *fakeDatabaseCreator) CreateForGateway(ctx context.Context, gatewayName, fleetID string) (string, error) {
	f.calls++
	f.lastName = gatewayName
	f.lastFleet = fleetID
	if f.err != nil {
		return "", f.err
	}
	return f.created, nil
}

// Deployment placement owns database_id: a caller-supplied value is discarded
// and a new dedicated ManagedDatabase is always created.
func TestDeploymentPlacementIgnoresExplicitDatabaseID(t *testing.T) {
	creator := &fakeDatabaseCreator{created: "server-created-db-id"}
	placement := NewDeploymentPlacement(&fakeFleetLookup{}, creator)

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
}

// TestDeploymentPlacementAutoCreatesPerGatewayDatabase covers the
// deployment-mode default: a blank database_id causes the API server to
// auto-create a dedicated ManagedDatabase for the gateway, requiring no
// pre-existing ManagedDatabase and no CNPG APIs.
func TestDeploymentPlacementAutoCreatesPerGatewayDatabase(t *testing.T) {
	creator := &fakeDatabaseCreator{created: "new-db-id"}
	placement := NewDeploymentPlacement(&fakeFleetLookup{}, creator)

	gw := &Gateway{Name: "gw1", FleetId: "fleet-1"}
	if err := placement.Resolve(context.Background(), gw); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if gw.DatabaseId != "new-db-id" {
		t.Fatalf("DatabaseId = %q, want %q", gw.DatabaseId, "new-db-id")
	}
	if creator.calls != 1 {
		t.Fatalf("CreateForGateway called %d times, want 1", creator.calls)
	}
	if creator.lastName != "gw1" || creator.lastFleet != "fleet-1" {
		t.Fatalf("CreateForGateway called with (%q, %q), want (%q, %q)", creator.lastName, creator.lastFleet, "gw1", "fleet-1")
	}
}

// TestDeploymentPlacementRequiresFleet covers the deployment provider error
// path: per-gateway database creation needs a fleet to attach the
// ManagedDatabase to, so an unresolvable fleet is rejected rather than
// creating an orphaned database.
func TestDeploymentPlacementRequiresFleet(t *testing.T) {
	creator := &fakeDatabaseCreator{created: "unused"}
	placement := NewDeploymentPlacement(&fakeFleetLookup{}, creator)

	gw := &Gateway{Name: "gw1"}
	err := placement.Resolve(context.Background(), gw)
	if err == nil {
		t.Fatal("Resolve() = nil error, want an error when fleet_id cannot be resolved")
	}
	if creator.calls != 0 {
		t.Fatalf("CreateForGateway called %d times, want 0 on fleet resolution failure", creator.calls)
	}
}

// CNPG placement also owns database_id: it resolves the fleet database and
// never trusts a caller-supplied relationship ID.
func TestCNPGPlacementIgnoresExplicitDatabaseID(t *testing.T) {
	dbs := &fakeDatabaseLookup{soleInFleet: map[string]string{"fleet-1": "resolved-db-id"}}
	placement := NewCNPGPlacement(&fakeFleetLookup{}, dbs)

	gw := &Gateway{Name: "gw1", FleetId: "fleet-1", DatabaseId: "client-supplied-db-id"}
	if err := placement.Resolve(context.Background(), gw); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if gw.DatabaseId != "resolved-db-id" {
		t.Fatalf("DatabaseId = %q, want resolved fleet database", gw.DatabaseId)
	}
}

// TestCNPGPlacementResolvesSoleFleetDatabase covers unchanged cnpg-mode
// auto-resolution: a blank database_id with a known fleet resolves against
// the fleet ManagedDatabases rather than creating a new one.
func TestCNPGPlacementResolvesSoleFleetDatabase(t *testing.T) {
	dbs := &fakeDatabaseLookup{soleInFleet: map[string]string{"fleet-1": "db-1"}}
	placement := NewCNPGPlacement(&fakeFleetLookup{}, dbs)

	gw := &Gateway{Name: "gw1", FleetId: "fleet-1"}
	if err := placement.Resolve(context.Background(), gw); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if gw.DatabaseId != "db-1" {
		t.Fatalf("DatabaseId = %q, want %q", gw.DatabaseId, "db-1")
	}
}

// TestCNPGPlacementRejectsAmbiguousFleetDatabases covers unchanged cnpg-mode
// error handling: zero or multiple ManagedDatabases in the resolved fleet is
// rejected rather than guessing.
func TestCNPGPlacementRejectsAmbiguousFleetDatabases(t *testing.T) {
	dbs := &fakeDatabaseLookup{}
	placement := NewCNPGPlacement(&fakeFleetLookup{}, dbs)

	gw := &Gateway{Name: "gw1", FleetId: "fleet-1"}
	err := placement.Resolve(context.Background(), gw)
	if err == nil {
		t.Fatal("Resolve() = nil error, want an error when the fleet has zero ManagedDatabases")
	}
}

func TestResolveFleetFromCluster(t *testing.T) {
	fleets := &fakeFleetLookup{fleetForCluster: map[string]string{"cluster-1": "fleet-1"}}
	dbs := &fakeDatabaseLookup{soleInFleet: map[string]string{"fleet-1": "db-1"}}
	placement := NewCNPGPlacement(fleets, dbs)

	gw := &Gateway{Name: "gw1", ClusterId: "cluster-1"}
	if err := placement.Resolve(context.Background(), gw); err != nil {
		t.Fatalf("Resolve() unexpected error: %v", err)
	}
	if gw.FleetId != "fleet-1" {
		t.Fatalf("FleetId = %q, want %q", gw.FleetId, "fleet-1")
	}
	if gw.DatabaseId != "db-1" {
		t.Fatalf("DatabaseId = %q, want %q", gw.DatabaseId, "db-1")
	}
}

func TestResolveFleetFromClusterPropagatesDependencyError(t *testing.T) {
	fleets := &fakeFleetLookup{clusterErr: errors.New("boom")}
	placement := NewCNPGPlacement(fleets, &fakeDatabaseLookup{})

	gw := &Gateway{Name: "gw1", ClusterId: "cluster-1"}
	err := placement.Resolve(context.Background(), gw)
	if err == nil {
		t.Fatal("Resolve() = nil error, want the underlying fleet lookup error propagated")
	}
	if IsPlacementValidationError(err) {
		t.Fatalf("Resolve() error = %v, want dependency classification", err)
	}
}

func TestPlacementValidationErrorClassification(t *testing.T) {
	placement := NewDeploymentPlacement(&fakeFleetLookup{}, &fakeDatabaseCreator{})
	err := placement.Resolve(context.Background(), &Gateway{Name: "gw1"})
	if err == nil {
		t.Fatal("Resolve() = nil, want unresolved-fleet validation error")
	}
	if !IsPlacementValidationError(err) {
		t.Fatalf("Resolve() error = %v, want validation classification", err)
	}
}

func TestPlacementDatabaseCreationErrorClassification(t *testing.T) {
	placement := NewDeploymentPlacement(&fakeFleetLookup{}, &fakeDatabaseCreator{err: errors.New("database unavailable")})
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
		svc := &sqlGatewayService{placement: fakePlacementResolver{err: newPlacementValidationError("fleet is ambiguous")}}
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
