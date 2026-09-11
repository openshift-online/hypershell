package gateways

import (
	"context"
	stderrors "errors"
	"fmt"
)

type PlacementResolver interface {
	Resolve(ctx context.Context, gateway *Gateway) error
}

type DatabaseLookup interface {
	FindSole(ctx context.Context) (databaseID string, err error)
}

// DatabaseSelector resolves placement when more than one candidate
// ManagedDatabase is allowed to exist. FindOldest returns the earliest-created
// candidate, or "" when none exist.
type DatabaseSelector interface {
	FindOldest(ctx context.Context) (databaseID string, err error)
}

type DatabaseCreator interface {
	CreateForGateway(ctx context.Context, gatewayName string) (databaseID string, err error)
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
	dbs DatabaseLookup
}

func NewCNPGPlacement(dbs DatabaseLookup) PlacementResolver {
	return &cnpgPlacement{dbs: dbs}
}

func (p *cnpgPlacement) Resolve(ctx context.Context, gw *Gateway) error {
	// database_id is owned by placement. Never trust or preserve a value supplied
	// by an API client. Database placement is resolved server-side.
	gw.DatabaseId = ""

	dbID, err := p.dbs.FindSole(ctx)
	if err != nil {
		return newPlacementDependencyError("resolve database", err)
	}
	if dbID == "" {
		return newPlacementValidationError("zero or multiple ManagedDatabases exist")
	}
	gw.DatabaseId = dbID
	return nil
}

type deploymentPlacement struct {
	dbs DatabaseCreator
}

func NewDeploymentPlacement(dbs DatabaseCreator) PlacementResolver {
	return &deploymentPlacement{dbs: dbs}
}

// externalPlacement assigns every new gateway to the first-created external
// ManagedDatabase. More than one registration is not an error: an operator may
// register a second external server ahead of a migration without intending to
// move where new gateways land. Only the empty result is rejected.
//
// Selection happens at gateway creation only. Once assigned, database_id is
// fixed for the gateway's lifetime, so a later registration never relocates an
// existing gateway.
type externalPlacement struct {
	dbs DatabaseSelector
}

func NewExternalPlacement(dbs DatabaseSelector) PlacementResolver {
	return &externalPlacement{dbs: dbs}
}

func (p *externalPlacement) Resolve(ctx context.Context, gw *Gateway) error {
	gw.DatabaseId = ""

	dbID, err := p.dbs.FindOldest(ctx)
	if err != nil {
		return newPlacementDependencyError("resolve external database", err)
	}
	if dbID == "" {
		return newPlacementValidationError("no external ManagedDatabase is registered; register one before creating gateways")
	}
	gw.DatabaseId = dbID
	return nil
}

func (p *deploymentPlacement) Resolve(ctx context.Context, gw *Gateway) error {
	// A deployment database is dedicated to exactly one gateway. Ignore any
	// client-provided database_id and always create the server-owned resource.
	gw.DatabaseId = ""

	dbID, err := p.dbs.CreateForGateway(ctx, gw.Name)
	if err != nil {
		return newPlacementDependencyError("create per-gateway database", err)
	}
	if dbID == "" {
		return newPlacementDependencyError("create per-gateway database", fmt.Errorf("ManagedDatabase service returned an empty ID"))
	}
	gw.DatabaseId = dbID
	return nil
}
