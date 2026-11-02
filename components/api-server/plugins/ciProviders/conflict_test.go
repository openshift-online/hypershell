package ciProviders

import (
	stderrors "errors"
	"testing"

	"github.com/lib/pq"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// TestConflictForUniqueDetectsSQLSTATE is the unique-violation-detection
// regression for FINDING 5. The conflict must be detected by SQLSTATE 23505 on the
// driver's *pq.Error, not by substring-matching the error text. A prior
// implementation matched "duplicate"/"unique" in the message, which misses a
// unique violation whose message omits those words (locale, driver wording) and
// could also misfire on an unrelated error that happens to contain them. This test
// pins the SQLSTATE behavior; the message-substring cases below fail under the old
// implementation.
func TestConflictForUniqueDetectsSQLSTATE(t *testing.T) {
	cp := &CIProvider{Name: "github", Issuer: "https://issuer.example/x"}

	cases := []struct {
		name         string
		err          error
		wantConflict bool
		wantContains string // substring the 409 reason must contain, when conflict
	}{
		{
			name:         "23505 on name index -> 409 naming the name",
			err:          &pq.Error{Code: "23505", Constraint: "idx_ci_providers_name"},
			wantConflict: true,
			wantContains: "github",
		},
		{
			name:         "23505 on issuer index -> 409 naming the issuer",
			err:          &pq.Error{Code: "23505", Constraint: "idx_ci_providers_issuer"},
			wantConflict: true,
			wantContains: "issuer",
		},
		{
			// A genuine 23505 whose MESSAGE contains neither "duplicate" nor
			// "unique": the old substring matcher returned nil here (no 409); the
			// SQLSTATE check still maps it to a 409. This case fails without the fix.
			name:         "23505 with a message lacking duplicate/unique words -> 409",
			err:          &pq.Error{Code: "23505", Constraint: "idx_ci_providers_name", Message: "constraint collision"},
			wantConflict: true,
			wantContains: "github",
		},
		{
			name:         "non-unique pq error (foreign key 23503) -> not a conflict",
			err:          &pq.Error{Code: "23503", Message: "duplicate key value violates unique constraint"},
			wantConflict: false,
		},
		{
			name:         "plain non-pq error -> not a conflict",
			err:          stderrors.New("boom"),
			wantConflict: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := conflictForUnique(tc.err, cp)
			if tc.wantConflict {
				if got == nil {
					t.Fatalf("expected a 409 conflict, got nil")
				}
				if got.Code != errors.ErrorConflict {
					t.Fatalf("expected ErrorConflict (409), got code %d", got.Code)
				}
				if tc.wantContains != "" && !contains(got.Reason, tc.wantContains) {
					t.Fatalf("409 reason %q should contain %q", got.Reason, tc.wantContains)
				}
				return
			}
			if got != nil {
				t.Fatalf("expected nil (not a unique violation), got %v", got)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
