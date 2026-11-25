package gateways

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The placement rewrite swaps retired labels in both selectors and keeps every
// other key. A placement with no retired label is left byte-for-byte as stored.
// Verifies: PLAT-CLUS-06.06
func TestReplaceRetiredPlacementJSON(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      string
		want    map[string]any
		changed bool
	}{
		{
			name:    "required selector",
			in:      `{"selector": ["network/public", "provider/ibm", "runtime/release-check"]}`,
			want:    map[string]any{"selector": []any{"network/public", "provider/ibm", "purpose/release-check"}},
			changed: true,
		},
		{
			name: "excluded selector keeps the required one",
			in:   `{"selector": ["provider/ibm", "network/public"], "excludedSelector": ["runtime/release-check"]}`,
			want: map[string]any{
				"selector":         []any{"provider/ibm", "network/public"},
				"excludedSelector": []any{"purpose/release-check"},
			},
			changed: true,
		},
		{
			name:    "matrix label",
			in:      `{"selector": ["runtime/release-matrix"]}`,
			want:    map[string]any{"selector": []any{"purpose/release-matrix"}},
			changed: true,
		},
		{
			name:    "unrelated runtime label",
			in:      `{"selector": ["runtime/release-candidate"]}`,
			changed: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, changed, err := replaceRetiredPlacementJSON([]byte(tc.in))
			if err != nil {
				t.Fatal(err)
			}
			if changed != tc.changed {
				t.Fatalf("changed = %v, want %v", changed, tc.changed)
			}
			if !changed {
				if string(out) != tc.in {
					t.Fatalf("unchanged placement rewritten: %s", out)
				}
				return
			}
			var got map[string]any
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("placement = %v, want %v", got, tc.want)
			}
		})
	}
	if _, _, err := replaceRetiredPlacementJSON([]byte(`{"selector": "runtime/release-check"}`)); err == nil {
		t.Fatal("a selector that is not an array must be reported, not skipped")
	}
}
