package gateways

import (
	"context"
	stderrors "errors"
	"fmt"
)

type PlacementResolver interface {
	Resolve(ctx context.Context, gateway *Gateway) error
}

type FleetLookup interface {
	FleetIDForCluster(ctx context.Context, clusterID string) (string, error)
	FindSoleFleet(ctx context.Context) (string, error)
}

type DatabaseLookup interface {
	FindSoleInFleet(ctx context.Context, fleetID string) (databaseID string, err error)
	FindSole(ctx context.Context) (databaseID string, fleetID string, err error)
}

type DatabaseCreator interface {
	CreateForGateway(ctx context.Context, gatewayName, fleetID string) (databaseID string, err error)
}

type placementErrorKind int

const (
	placementValidationError placementErrorKind = iota
	placementDependencyError
)

type placementError struct {
	kind      placementErrorKind
	operation string
	cause     error
}

func (e *placementError) Error() string {
	if e.operation == "" {
		return e.cause.Error()
	}
	return fmt.Sprintf("%s: %v", e.operation, e.cause)
}

func (e *placementError) Unwrap() error {
	return e.cause
}

func newPlacementValidationError(format string, args ...interface{}) error {
	return &placementError{
		kind:  placementValidationError,
		cause: fmt.Errorf(format, args...),
	}
}

func newPlacementDependencyError(operation string, err error) error {
	return &placementError{
		kind:      placementDependencyError,
		operation: operation,
		cause:     err,
	}
}

func IsPlacementValidationError(err error) bool {
	var placementErr *placementError
	return stderrors.As(err, &placementErr) && placementErr.kind == placementValidationError
}

type cnpgPlacement struct {
	fleets FleetLookup
	dbs    DatabaseLookup
}

func NewCNPGPlacement(fleets FleetLookup, dbs DatabaseLookup) PlacementResolver {
	return &cnpgPlacement{fleets: fleets, dbs: dbs}
}

func (p *cnpgPlacement) Resolve(ctx context.Context, gw *Gateway) error {
	// database_id is owned by placement. Never trust or preserve a value supplied
	// by an API client; CNPG placement always resolves it from server-side fleet
	// state.
	gw.DatabaseId = ""

	if err := resolveFleet(ctx, p.fleets, gw); err != nil {
		return err
	}
	if gw.FleetId != "" {
		dbID, err := p.dbs.FindSoleInFleet(ctx, gw.FleetId)
		if err != nil {
			return newPlacementDependencyError("resolve fleet database", err)
		}
		if dbID == "" {
			return newPlacementValidationError("fleet %s has zero or multiple ManagedDatabases", gw.FleetId)
		}
		gw.DatabaseId = dbID
		return nil
	}

	dbID, fleetID, err := p.dbs.FindSole(ctx)
	if err != nil {
		return newPlacementDependencyError("resolve database", err)
	}
	if dbID == "" {
		return newPlacementValidationError("zero or multiple ManagedDatabases exist")
	}
	gw.DatabaseId = dbID
	gw.FleetId = fleetID
	return nil
}

type deploymentPlacement struct {
	fleets FleetLookup
	dbs    DatabaseCreator
}

func NewDeploymentPlacement(fleets FleetLookup, dbs DatabaseCreator) PlacementResolver {
	return &deploymentPlacement{fleets: fleets, dbs: dbs}
}

func (p *deploymentPlacement) Resolve(ctx context.Context, gw *Gateway) error {
	// A deployment database is dedicated to exactly one gateway. Ignore any
	// client-provided database_id and always create the server-owned resource.
	gw.DatabaseId = ""

	if err := resolveFleet(ctx, p.fleets, gw); err != nil {
		return err
	}
	if gw.FleetId == "" {
		return newPlacementValidationError("fleet_id could not be resolved for per-gateway database provisioning")
	}

	dbID, err := p.dbs.CreateForGateway(ctx, gw.Name, gw.FleetId)
	if err != nil {
		return newPlacementDependencyError("create per-gateway database", err)
	}
	if dbID == "" {
		return newPlacementDependencyError("create per-gateway database", fmt.Errorf("ManagedDatabase service returned an empty ID"))
	}
	gw.DatabaseId = dbID
	return nil
}

func resolveFleet(ctx context.Context, fleets FleetLookup, gw *Gateway) error {
	if gw.FleetId != "" {
		return nil
	}
	if fleets == nil {
		return nil
	}
	if gw.ClusterId != "" {
		fid, err := fleets.FleetIDForCluster(ctx, gw.ClusterId)
		if err != nil {
			return newPlacementDependencyError(fmt.Sprintf("resolve fleet from cluster %s", gw.ClusterId), err)
		}
		if fid == "" {
			return newPlacementValidationError("cluster %s does not belong to a fleet", gw.ClusterId)
		}
		gw.FleetId = fid
		return nil
	}

	fid, err := fleets.FindSoleFleet(ctx)
	if err != nil {
		return newPlacementDependencyError("resolve fleet", err)
	}
	gw.FleetId = fid
	return nil
}
