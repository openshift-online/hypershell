package managedClusters

import (
	"reflect"
	"testing"
)

// Verifies: PLAT-CLUS-06
func TestNormalizeCapabilities(t *testing.T) {
	got, svcErr := NormalizeCapabilities([]string{"runtime/kata", "provider/aws", "network/vpn"}, true)
	if svcErr != nil {
		t.Fatal(svcErr)
	}
	want := []string{"network/vpn", "provider/aws", "runtime/kata"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("capabilities = %v, want %v", got, want)
	}
	for _, labels := range [][]string{
		{},
		{"provider/AWS"},
		{"unknown/value"},
		{"provider/aws", "provider/ibm"},
		{"network/public", "network/public"},
	} {
		if _, svcErr := NormalizeCapabilities(labels, true); svcErr == nil {
			t.Errorf("NormalizeCapabilities(%v) unexpectedly succeeded", labels)
		}
	}
}

func TestNormalizeSelectorSortsAndDeduplicates(t *testing.T) {
	got, svcErr := NormalizeSelector([]string{"provider/aws", "network/public", "provider/aws"})
	if svcErr != nil {
		t.Fatal(svcErr)
	}
	want := []string{"network/public", "provider/aws"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selector = %v, want %v", got, want)
	}
}
