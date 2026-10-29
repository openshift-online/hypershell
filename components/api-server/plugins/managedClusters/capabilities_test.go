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

// The reserved managed-cluster namespace is rejected as a self-declared capability
// (registration heartbeat / generic create input).
// Verifies: PLAT-CLUS-14.02
func TestNormalizeCapabilitiesRejectsReservedIdentityNamespace(t *testing.T) {
	for _, requireNonEmpty := range []bool{true, false} {
		if _, svcErr := NormalizeCapabilities([]string{"managed-cluster/hyp0-spoke1"}, requireNonEmpty); svcErr == nil {
			t.Fatalf("NormalizeCapabilities must reject a managed-cluster/* label (requireNonEmpty=%v)", requireNonEmpty)
		}
	}
	// A reserved label mixed with a valid one is still rejected as a whole.
	if _, svcErr := NormalizeCapabilities([]string{"provider/aws", "managed-cluster/x"}, true); svcErr == nil {
		t.Fatal("NormalizeCapabilities must reject a set containing any managed-cluster/* label")
	}
}

// A placement selector MAY reference the reserved identity namespace (its use is
// gated on platform:admin elsewhere).
// Verifies: PLAT-CLUS-14.03
func TestNormalizeSelectorAllowsIdentityNamespace(t *testing.T) {
	got, svcErr := NormalizeSelector([]string{"managed-cluster/hyp0-spoke1"})
	if svcErr != nil {
		t.Fatalf("NormalizeSelector must allow a managed-cluster/* identity selector: %v", svcErr)
	}
	if want := []string{"managed-cluster/hyp0-spoke1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selector = %v, want %v", got, want)
	}
}

func TestIdentityCapabilityHelpers(t *testing.T) {
	if got := IdentityCapability("hyp0-spoke1"); got != "managed-cluster/hyp0-spoke1" {
		t.Fatalf("IdentityCapability = %q, want managed-cluster/hyp0-spoke1", got)
	}
	if !HasReservedCapability([]string{"provider/aws", "managed-cluster/x"}) {
		t.Fatal("HasReservedCapability must detect a managed-cluster/* label")
	}
	if HasReservedCapability([]string{"provider/aws", "network/public"}) {
		t.Fatal("HasReservedCapability must not flag a non-reserved set")
	}
	if !SelectorReferencesIdentity([]string{"managed-cluster/a"}) {
		t.Fatal("SelectorReferencesIdentity must detect an identity selector")
	}
	if SelectorReferencesIdentity([]string{"provider/aws"}) {
		t.Fatal("SelectorReferencesIdentity must not flag a requirement selector")
	}
}

// The identity capability is derived server-side from the unique name and exposed
// only to a platform:admin caller (admin sees it; non-admin does not).
// Verifies: PLAT-CLUS-14.01, PLAT-CLUS-14.05
func TestPresentWithIdentity(t *testing.T) {
	cluster := &ManagedCluster{Name: "hyp0-spoke1", Capabilities: []string{"network/public", "provider/aws"}}

	admin := presentWithIdentity(cluster, true)
	if !contains(admin.Capabilities, "managed-cluster/hyp0-spoke1") {
		t.Fatalf("admin read must include the identity capability, got %v", admin.Capabilities)
	}
	if !contains(admin.Capabilities, "provider/aws") || !contains(admin.Capabilities, "network/public") {
		t.Fatalf("admin read must keep declared capabilities, got %v", admin.Capabilities)
	}

	nonAdmin := presentWithIdentity(cluster, false)
	for _, label := range nonAdmin.Capabilities {
		if label == "managed-cluster/hyp0-spoke1" {
			t.Fatalf("non-admin read must not include the identity capability, got %v", nonAdmin.Capabilities)
		}
	}
	if !contains(nonAdmin.Capabilities, "provider/aws") {
		t.Fatalf("non-admin read must keep declared capabilities, got %v", nonAdmin.Capabilities)
	}
}

func contains(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}
