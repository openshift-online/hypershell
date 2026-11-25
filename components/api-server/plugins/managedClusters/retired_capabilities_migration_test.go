package managedClusters

import "testing"

// The capabilities rewrite swaps retired labels and leaves other sets as stored.
// Verifies: PLAT-CLUS-06.06
func TestReplaceRetiredCapabilitiesJSON(t *testing.T) {
	out, changed, err := replaceRetiredCapabilitiesJSON([]byte(`["provider/ibm","runtime/release-matrix","runtime/release-check"]`))
	if err != nil || !changed {
		t.Fatalf("changed = %v, err = %v; want a rewrite", changed, err)
	}
	if want := `["provider/ibm","purpose/release-check","purpose/release-matrix"]`; string(out) != want {
		t.Fatalf("capabilities = %s, want %s", out, want)
	}

	stored := `["provider/ibm", "runtime/kata"]`
	out, changed, err = replaceRetiredCapabilitiesJSON([]byte(stored))
	if err != nil || changed || string(out) != stored {
		t.Fatalf("an unaffected set must be left as stored: %s, %v, %v", out, changed, err)
	}

	if _, _, err := replaceRetiredCapabilitiesJSON([]byte(`{"runtime/release-check": true}`)); err == nil {
		t.Fatal("a capabilities value that is not an array must be reported, not skipped")
	}
}
