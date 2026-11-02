package extsearch

import (
	"testing"

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
