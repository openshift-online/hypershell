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
