package e2e

import (
	"slices"
	"testing"
)

// fleetFixture is a hyp0-like fleet: a hub and two spokes that opt in to the
// matrix, an experimental spoke that does not, and an unregistered cluster.
func fleetFixture() []matrixCluster {
	return []matrixCluster{
		{ID: "hub", OidcSubject: "sub-hub", Capabilities: []string{"provider/ibm", "network/public", "placement/release-check", "placement/release-matrix"}},
		{ID: "spoke1", OidcSubject: "sub-1", Capabilities: []string{"provider/ibm", "network/public", "placement/release-matrix"}},
		{ID: "spoke2", OidcSubject: "sub-2", Capabilities: []string{"provider/ibm", "network/public", "runtime/kata", "placement/release-matrix"}},
		{ID: "spoke3", OidcSubject: "sub-3", Capabilities: []string{"provider/ibm", "environment/experimental"}},
		{ID: "unregistered", Capabilities: []string{"provider/aws", "placement/release-matrix"}},
	}
}

// Without a required capability the matrix covers every registered cluster, as
// before: the unregistered cluster is skipped and every label is a suite.
// Verifies: E2E-CORE-30.01, E2E-CORE-30.02
func TestBuildMatrixFleetWholeFleet(t *testing.T) {
	fleet := buildMatrixFleet(fleetFixture(), "")

	want := []string{"environment/experimental", "network/public", "placement/release-check", "placement/release-matrix", "provider/ibm", "runtime/kata"}
	if got := fleet.capabilities(); !slices.Equal(got, want) {
		t.Fatalf("capabilities = %v, want %v", got, want)
	}
	if fleet.registered != 4 || fleet.optedIn != 4 {
		t.Fatalf("registered=%d optedIn=%d, want 4 and 4", fleet.registered, fleet.optedIn)
	}
	if !fleet.eligible["provider/ibm"]["spoke3"] {
		t.Fatalf("without a required capability spoke3 must be eligible for provider/ibm: %v", fleet.eligible["provider/ibm"])
	}
}

// With a required capability only opted-in clusters count: a cluster without the
// label contributes no capability and is never eligible, and the label itself is
// not a suite of its own.
// Verifies: E2E-CORE-30.05
func TestBuildMatrixFleetRequiredCapability(t *testing.T) {
	fleet := buildMatrixFleet(fleetFixture(), "placement/release-matrix")

	want := []string{"network/public", "placement/release-check", "provider/ibm", "runtime/kata"}
	if got := fleet.capabilities(); !slices.Equal(got, want) {
		t.Fatalf("capabilities = %v, want %v", got, want)
	}
	if fleet.registered != 4 || fleet.optedIn != 3 {
		t.Fatalf("registered=%d optedIn=%d, want 4 and 3", fleet.registered, fleet.optedIn)
	}
	for capability, clusters := range fleet.eligible {
		if clusters["spoke3"] {
			t.Fatalf("spoke3 did not opt in but is eligible for %s", capability)
		}
		if clusters["unregistered"] {
			t.Fatalf("an unregistered cluster is eligible for %s", capability)
		}
	}
	if got := fleet.eligible["provider/ibm"]; len(got) != 3 || !got["hub"] || !got["spoke1"] || !got["spoke2"] {
		t.Fatalf("provider/ibm eligible = %v, want hub, spoke1, spoke2", got)
	}
}

// A required capability that no registered cluster advertises leaves nothing to
// cover, so TestMatrix fails rather than passing with zero suites.
// Verifies: E2E-CORE-30.05
func TestBuildMatrixFleetNoClusterOptedIn(t *testing.T) {
	fleet := buildMatrixFleet(fleetFixture(), "runtime/absent")
	if got := fleet.capabilities(); len(got) != 0 {
		t.Fatalf("capabilities = %v, want none", got)
	}
	if fleet.registered != 4 || fleet.optedIn != 0 {
		t.Fatalf("registered=%d optedIn=%d, want 4 and 0", fleet.registered, fleet.optedIn)
	}
}

// A matrix suite with a required capability places by [capability, required], so
// the gateway can land only on an opted-in cluster.
// Verifies: E2E-CORE-30.05
func TestGatewayCreateRequestForSuiteRequiredCapability(t *testing.T) {
	s := &E2ESuite{driver: nameDriver{name: "openshift"}, matrixCapability: "provider/ibm", matrixRequiredCapability: "placement/release-matrix"}

	got := decodeRequest(t, s.gatewayCreateRequestForSuite("e2e-gw"))
	if sel := placementSelector(t, got); !slices.Equal(sel, []string{"provider/ibm", "placement/release-matrix"}) {
		t.Fatalf("selector = %v, want [provider/ibm placement/release-matrix]", sel)
	}
	if _, ok := got["cluster_id"]; ok {
		t.Fatalf("matrix wire payload must not contain cluster_id: %v", got)
	}
}

// The required capability is never duplicated when it equals the suite capability.
func TestCapabilityGatewayCreateRequestNoDuplicateRequired(t *testing.T) {
	got := decodeRequest(t, capabilityGatewayCreateRequest("e2e-gw", "placement/release-matrix", "placement/release-matrix"))
	if sel := placementSelector(t, got); !slices.Equal(sel, []string{"placement/release-matrix"}) {
		t.Fatalf("selector = %v, want [placement/release-matrix]", sel)
	}
}
