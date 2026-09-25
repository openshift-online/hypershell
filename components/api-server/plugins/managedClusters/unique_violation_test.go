package managedClusters

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// The registration race fallback keys on SQLSTATE 23505 (unique_violation),
// never on the driver's message text, for both drivers the GORM session can
// return errors from (pgx in production, lib/pq in the test session).
func TestIsUniqueViolation(t *testing.T) {
	cases := map[string]struct {
		err  error
		want bool
	}{
		"nil":                          {err: nil, want: false},
		"plain error with the message": {err: errors.New(`duplicate key value violates unique constraint "idx"`), want: false},
		"pgx unique violation":         {err: &pgconn.PgError{Code: "23505"}, want: true},
		"pgx wrapped unique violation": {err: fmt.Errorf("create: %w", &pgconn.PgError{Code: "23505"}), want: true},
		"pgx other code":               {err: &pgconn.PgError{Code: "23503"}, want: false},
		"pq unique violation":          {err: &pq.Error{Code: "23505"}, want: true},
		"pq wrapped unique violation":  {err: fmt.Errorf("create: %w", &pq.Error{Code: "23505"}), want: true},
		"pq other code":                {err: &pq.Error{Code: "42704"}, want: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isUniqueViolation(tc.err); got != tc.want {
				t.Fatalf("isUniqueViolation(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
