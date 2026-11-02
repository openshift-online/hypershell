package extsearch

import (
	"testing"

	"github.com/yaacov/tree-search-language/pkg/tsl"
	"github.com/yaacov/tree-search-language/pkg/walkers/ident"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
)

// model mirrors the shape of an ext Kind: an embedded Meta (id/created_at/...) plus
// a queryable name column and a column-tag-overridden identity column.
type model struct {
	api.Meta
	Name         string `json:"name"`
	CIProviderID string `json:"ci_provider_id" gorm:"column:ci_provider_id"`
}

func (model) TableName() string { return "models" }

func TestValidateSearchColumns(t *testing.T) {
	cases := []struct {
		name      string
		search    string
		wantError bool
		contains  string
	}{
		{name: "empty search is valid", search: "", wantError: false},
		{name: "known name column", search: "name = 'x'", wantError: false},
		{name: "column-tag override resolves", search: "ci_provider_id = 'x'", wantError: false},
		{name: "embedded meta column resolves", search: "id = 'x'", wantError: false},
		{name: "unknown column is rejected", search: "definitely_not_a_column = 'x'", wantError: true, contains: "definitely_not_a_column"},
		{name: "malformed search is a clean 400", search: "name = = =", wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			serr := ValidateSearchColumns(tc.search, &model{})
			if tc.wantError && serr == nil {
				t.Fatalf("expected an error for %q, got nil", tc.search)
			}
			if !tc.wantError && serr != nil {
				t.Fatalf("expected no error for %q, got %v", tc.search, serr)
			}
			if tc.contains != "" && serr != nil && !contains(serr.Error(), tc.contains) {
				t.Fatalf("error %q should name %q", serr.Error(), tc.contains)
			}
		})
	}
}

// TestCheckNodeEnumeratesArrayOperands is the fail-closed enumeration test
// (FINDING 5). The upstream ident.Walk does NOT descend into an IN clause's
// $array operand ("we assume that all are leafs, no need to walk on them"), so a
// column identifier hidden in an array would reach SQL unchecked and abort the
// request transaction. Our checkNode walks the array too, so every identifier is
// enumerated and can be verified. This builds the parsed shape directly because
// the current TSL grammar refuses to parse a bare identifier inside an IN list;
// the guard is defensive against a grammar/parser that does emit one.
func TestCheckNodeEnumeratesArrayOperands(t *testing.T) {
	tree := tsl.Node{
		Func: tsl.InOp,
		Left: tsl.Node{Func: tsl.IdentOp, Left: "name"},
		Right: tsl.Node{
			Func: tsl.ArrayOp,
			Right: []tsl.Node{
				{Func: tsl.IdentOp, Left: "bogus_col"},
				{Func: tsl.StringOp, Left: "x"},
			},
		},
	}

	// Our fail-closed walker visits the array element identifier.
	var seen []string
	if err := checkNode(tree, func(f string) { seen = append(seen, f) }); err != nil {
		t.Fatalf("checkNode returned an unexpected error: %v", err)
	}
	if !containsStr(seen, "bogus_col") {
		t.Fatalf("checkNode did not enumerate the array-operand column; visited=%v", seen)
	}

	// Demonstrate the gap: the upstream ident.Walk misses the same column, which
	// is exactly why validation cannot rely on it.
	var walked []string
	_, _ = ident.Walk(tree, func(f string) (string, error) { walked = append(walked, f); return f, nil })
	if containsStr(walked, "bogus_col") {
		t.Fatalf("ident.Walk unexpectedly visited the array column; the fail-closed walker would be unnecessary. walked=%v", walked)
	}
}

// TestCheckNodeFailsClosedOnUnknownOperand asserts that an operand shape the
// walker cannot classify is rejected (returns an error) rather than silently
// passed through, so an unenumerable column can never slip to SQL (FINDING 5).
func TestCheckNodeFailsClosedOnUnknownOperand(t *testing.T) {
	tree := tsl.Node{
		Func:  tsl.EqOp,
		Left:  tsl.Node{Func: tsl.IdentOp, Left: "name"},
		Right: map[string]int{"surprise": 1}, // not a Node, []Node, or literal
	}
	if err := checkNode(tree, func(string) {}); err == nil {
		t.Fatalf("checkNode should fail closed on an unsupported operand type, got nil")
	}
}

func containsStr(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
