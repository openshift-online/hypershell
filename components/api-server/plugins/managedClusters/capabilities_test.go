package managedClusters

import (
	"reflect"
	"strings"
	"testing"
)

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

func TestNormalizeCapabilitiesGatewayVersion(t *testing.T) {
	got, svcErr := NormalizeCapabilities([]string{"provider/aws", "gateway-version/v0.1.2-rhaiv.7"}, true)
	if svcErr != nil {
		t.Fatal(svcErr)
	}
	want := []string{"gateway-version/v0.1.2-rhaiv.7", "provider/aws"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("capabilities = %v, want %v", got, want)
	}

	for _, labels := range [][]string{
		{"gateway-version/v0.1.2-rhaiv.7", "gateway-version/v0.1.1-rhaiv.3"}, // single-valued
		{"gateway-version/V0.1.2"},  // uppercase rejected
		{"gateway-version/.v0.1.2"}, // leading dot rejected
		{"gateway-version/v0.1.2-"},                                 // trailing hyphen rejected
		{"provider/v0.1.2-rhaiv.7"},                                // relaxation does not leak to other namespaces
		{"gateway-version/v0." + strings.Repeat("1", 130) + ".2"}, // over the length bound
	} {
		if _, svcErr := NormalizeCapabilities(labels, true); svcErr == nil {
			t.Errorf("NormalizeCapabilities(%v) unexpectedly succeeded", labels)
		}
	}
}

func TestGatewayVersionCapability(t *testing.T) {
	if v := GatewayVersionCapability([]string{"gateway-version/v0.1.2-rhaiv.7", "provider/aws"}); v != "v0.1.2-rhaiv.7" {
		t.Fatalf("GatewayVersionCapability = %q, want %q", v, "v0.1.2-rhaiv.7")
	}
	if v := GatewayVersionCapability([]string{"provider/aws", "network/vpn"}); v != "" {
		t.Fatalf("GatewayVersionCapability = %q, want empty", v)
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
