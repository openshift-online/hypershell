package gateways

import "testing"

// TestAnyAccessibleUnowned is the set-difference regression for the
// "show only my gateways" toggle condition (WEB-UIEX-07, SEC-RBAC-10, FINDING 3).
// The toggle is visible iff the caller's accessible set contains a gateway the
// caller does not own. The prior implementation compared lengths
// (len(accessible) > len(owned)), which is correct only when the owned set is a
// strict subset of the accessible set. When the caller owns a gateway that is NOT
// in its accessible set, the length comparison silently returns the wrong answer;
// a true set difference does not.
func TestAnyAccessibleUnowned(t *testing.T) {
	cases := []struct {
		name       string
		accessible []string
		owned      []string
		want       bool
	}{
		{
			name:       "accessible gateway not owned -> true",
			accessible: []string{"a", "b"},
			owned:      []string{"b"},
			want:       true,
		},
		{
			name:       "all accessible owned -> false",
			accessible: []string{"a", "b"},
			owned:      []string{"a", "b"},
			want:       false,
		},
		{
			// The exact case the length comparison gets wrong: the caller owns a
			// gateway ("c") that is absent from its accessible set, so
			// len(accessible) == len(owned) even though an accessible gateway ("a")
			// is unowned. len() returns false here; the set difference returns true.
			name:       "owned id outside accessible set, accessible has an unowned id -> true",
			accessible: []string{"a", "b"},
			owned:      []string{"b", "c"},
			want:       true,
		},
		{
			// Owned not a subset AND every accessible gateway is owned: the correct
			// answer is false, which a naive len() comparison also gets wrong the
			// other direction (len(accessible)=1 < len(owned)=2 -> false, correct by
			// luck). The set difference is unconditionally correct.
			name:       "owned superset of accessible -> false",
			accessible: []string{"a"},
			owned:      []string{"a", "c"},
			want:       false,
		},
		{
			name:       "no accessible gateways -> false",
			accessible: nil,
			owned:      []string{"a"},
			want:       false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := anyAccessibleUnowned(tc.accessible, tc.owned); got != tc.want {
				t.Fatalf("anyAccessibleUnowned(%v, %v) = %v, want %v",
					tc.accessible, tc.owned, got, tc.want)
			}
		})
	}
}
