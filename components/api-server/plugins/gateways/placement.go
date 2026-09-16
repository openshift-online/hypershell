package gateways

import (
	"context"
	stderrors "errors"
	"fmt"
)

type PlacementResolver interface {
	Resolve(ctx context.Context, gateway *Gateway) error
}

// DatabaseSelector resolves placement across the registered ManagedDatabases.
// FindOldest returns the earliest-created registration, or "" when none exist.
type DatabaseSelector interface {
	FindOldest(ctx context.Context) (databaseID string, err error)
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

// databasePlacement assigns every new gateway to the first-created
// ManagedDatabase. More than one registration is not an error: an operator may
// register a second server ahead of a migration without intending to move where
// new gateways land. Only the empty result is rejected.
//
// Selection happens at gateway creation only. Once assigned, database_id is
// fixed for the gateway's lifetime, so a later registration never relocates an
// existing gateway.
type databasePlacement struct {
	dbs DatabaseSelector
}

func NewDatabasePlacement(dbs DatabaseSelector) PlacementResolver {
	return &databasePlacement{dbs: dbs}
}

func (p *databasePlacement) Resolve(ctx context.Context, gw *Gateway) error {
	// database_id is owned by placement. Never trust or preserve a value supplied
	// by an API client. Database placement is resolved server-side.
	gw.DatabaseId = ""

	dbID, err := p.dbs.FindOldest(ctx)
	if err != nil {
		return newPlacementDependencyError("resolve database", err)
	}
	if dbID == "" {
		return newPlacementValidationError("no ManagedDatabase is registered; register one before creating gateways")
	}
	gw.DatabaseId = dbID
	return nil
}
