// Package dberr classifies PostgreSQL driver errors independently of which driver
// returned them. The GORM session uses the pgx driver (gorm.io/driver/postgres) in
// production and lib/pq in the test session, so the two drivers surface the same
// SQLSTATE as different concrete error types (*pgconn.PgError vs *pq.Error).
// Detecting a condition by SQLSTATE through both types - never by substring-matching
// the message text, which is fragile against locale and wording changes - is the
// only way to classify it uniformly across drivers (SDLC-CONF-13).
package dberr

import (
	stderrors "errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// uniqueViolation is the PostgreSQL SQLSTATE for unique_violation.
const uniqueViolation = "23505"

// IsUniqueViolation reports whether err (or any error it wraps) is a PostgreSQL
// unique_violation (SQLSTATE 23505), for both the pgx and lib/pq driver error types.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if stderrors.As(err, &pgErr) {
		return pgErr.Code == uniqueViolation
	}
	var pqErr *pq.Error
	if stderrors.As(err, &pqErr) {
		return string(pqErr.Code) == uniqueViolation
	}
	return false
}

// ConstraintName returns the name of the violated constraint (or index) carried by
// err, for both the pgx and lib/pq driver error types, or "" when err is neither
// driver type or carries no constraint name. Callers use it to attribute a unique
// violation to the specific field whose index was violated. pgx exposes it as
// *pgconn.PgError.ConstraintName and lib/pq as *pq.Error.Constraint.
func ConstraintName(err error) string {
	var pgErr *pgconn.PgError
	if stderrors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	var pqErr *pq.Error
	if stderrors.As(err, &pqErr) {
		return pqErr.Constraint
	}
	return ""
}
