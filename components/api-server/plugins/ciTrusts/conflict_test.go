package ciTrusts

import (
	stderrors "errors"
	"testing"

	"github.com/lib/pq"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// TestConflictForDuplicateNameDetectsSQLSTATE is the duplicate-name-detection
// regression for FINDING 5, covering the single-unique-index helper shape shared by
// ci_trusts, inference_routes and workspace_memberships. The conflict must be
// detected by SQLSTATE 23505 on the driver's *pq.Error, not by substring-matching
// the error text (fragile against locale and wording). A 23505 maps to a 409; an
// unrelated error does not.
func TestConflictForDuplicateNameDetectsSQLSTATE(t *testing.T) {
	cases := []struct {
		name         string
		err          error
		wantConflict bool
	}{
		{
			name:         "23505 -> 409",
			err:          &pq.Error{Code: "23505", Constraint: "idx_ci_trusts_name"},
			wantConflict: true,
		},
		{
			// 23505 whose message omits "duplicate"/"unique": fails without the fix.
			name:         "23505 with a message lacking duplicate/unique words -> 409",
			err:          &pq.Error{Code: "23505", Constraint: "idx_ci_trusts_name", Message: "constraint collision"},
			wantConflict: true,
		},
		{
			name:         "non-unique pq error (23503) -> not a conflict",
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
			got := conflictForDuplicateName(tc.err, "trust-name")
			if tc.wantConflict {
				if got == nil || got.Code != errors.ErrorConflict {
					t.Fatalf("expected a 409 conflict, got %v", got)
				}
				return
			}
			if got != nil {
				t.Fatalf("expected nil (not a unique violation), got %v", got)
			}
		})
	}
}
