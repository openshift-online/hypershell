package dberr

import (
	stderrors "errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// TestIsUniqueViolation pins SQLSTATE 23505 detection across BOTH driver error
// types. The pgx cases are the PRODUCTION path (the GORM session uses
// gorm.io/driver/postgres in production): a pq-only detector returns false for a
// *pgconn.PgError and turns every duplicate-key create/patch into a 500 instead of a
// 409. The lib/pq cases cover the test session. Detection keys on the code, never on
// the message text.
func TestIsUniqueViolation(t *testing.T) {
	cases := map[string]struct {
		err  error
		want bool
	}{
		"nil":                          {err: nil, want: false},
		"plain error with the message": {err: stderrors.New(`duplicate key value violates unique constraint "idx"`), want: false},
		"pgx unique violation":         {err: &pgconn.PgError{Code: "23505"}, want: true},
		"pgx wrapped unique violation": {err: fmt.Errorf("create: %w", &pgconn.PgError{Code: "23505"}), want: true},
		"pgx foreign-key violation":    {err: &pgconn.PgError{Code: "23503"}, want: false},
		"pq unique violation":          {err: &pq.Error{Code: "23505"}, want: true},
		"pq wrapped unique violation":  {err: fmt.Errorf("create: %w", &pq.Error{Code: "23505"}), want: true},
		"pq undefined-object":          {err: &pq.Error{Code: "42704"}, want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := IsUniqueViolation(tc.err); got != tc.want {
				t.Fatalf("IsUniqueViolation(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestConstraintName pins constraint-name attribution across BOTH driver error
// types, so a multi-index kind (ci_providers: name vs issuer) attributes the right
// field under production (pgx) as well as the test session (lib/pq).
func TestConstraintName(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"nil":                      {err: nil, want: ""},
		"plain error":              {err: stderrors.New("boom"), want: ""},
		"pgx name index":           {err: &pgconn.PgError{Code: "23505", ConstraintName: "idx_ci_providers_name"}, want: "idx_ci_providers_name"},
		"pgx wrapped issuer index": {err: fmt.Errorf("create: %w", &pgconn.PgError{Code: "23505", ConstraintName: "idx_ci_providers_issuer"}), want: "idx_ci_providers_issuer"},
		"pgx no constraint":        {err: &pgconn.PgError{Code: "23505"}, want: ""},
		"pq name index":            {err: &pq.Error{Code: "23505", Constraint: "idx_ci_providers_name"}, want: "idx_ci_providers_name"},
		"pq wrapped issuer index":  {err: fmt.Errorf("create: %w", &pq.Error{Code: "23505", Constraint: "idx_ci_providers_issuer"}), want: "idx_ci_providers_issuer"},
		"pq no constraint":         {err: &pq.Error{Code: "23505"}, want: ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ConstraintName(tc.err); got != tc.want {
				t.Fatalf("ConstraintName(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
