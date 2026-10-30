package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/tests/e2e/driver"
)

// nameDriver is a minimal E2EInfraDriver whose only live method is Name(); any
// other call reaches the nil embedded interface and panics, proving the request
// builders under test consult only the driver name.
type nameDriver struct {
	driver.E2EInfraDriver
	name string
}

func (d nameDriver) Name() string { return d.name }

// decodeRequest marshals a create request and decodes it into a generic map so a
// test can assert exactly which wire fields are present.
func decodeRequest(t *testing.T, req gatewayCreateRequest) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal gateway create request: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal gateway create request: %v", err)
	}
	return got
}

// placementSelector pulls the placement.selector array out of a decoded create
// request, or nil when placement/selector is absent.
func placementSelector(t *testing.T, got map[string]any) []string {
	t.Helper()
	placement, ok := got["placement"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := placement["selector"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// A matrix suite places every positive create by its one capability selector: the
// payload carries placement.selector = [<capability>] and never cluster_id
// (E2E-CORE-30).
// Verifies: E2E-CORE-30.01
func TestGatewayCreateRequestForSuiteCapability(t *testing.T) {
	s := &E2ESuite{driver: nameDriver{name: "kind"}, matrixCapability: "provider/aws"}

	req := s.gatewayCreateRequestForSuite("e2e-gw")
	if req.Placement == nil || len(req.Placement.Selector) != 1 || req.Placement.Selector[0] != "provider/aws" {
		t.Fatalf("matrix request must place by its capability selector, got %+v", req.Placement)
	}

	got := decodeRequest(t, req)
	if _, ok := got["cluster_id"]; ok {
		t.Fatalf("matrix wire payload must not contain cluster_id: %v", got)
	}
	if sel := placementSelector(t, got); len(sel) != 1 || sel[0] != "provider/aws" {
		t.Fatalf("matrix wire selector = %v, want [provider/aws]", sel)
	}
}

// A non-matrix suite (empty matrixCapability) keeps the driver's requirement selector
// and never sends cluster_id, preserving existing single-cluster and performance
// behavior.
func TestGatewayCreateRequestForSuiteRequirement(t *testing.T) {
	s := &E2ESuite{driver: nameDriver{name: "kind"}}

	req := s.gatewayCreateRequestForSuite("e2e-gw")
	if req.Placement == nil || req.Placement.Mode != "local-kind" {
		t.Fatalf("non-matrix request should carry the kind placement, got %+v", req.Placement)
	}

	got := decodeRequest(t, req)
	if _, ok := got["cluster_id"]; ok {
		t.Fatalf("placement wire payload must not contain cluster_id: %v", got)
	}
	if _, ok := got["placement"]; !ok {
		t.Fatalf("placement wire payload must contain placement: %v", got)
	}
}

// capabilityGatewayCreateRequest carries only name + placement.selector with the one
// capability, never cluster_id.
func TestCapabilityGatewayCreateRequestShape(t *testing.T) {
	got := decodeRequest(t, capabilityGatewayCreateRequest("e2e-gw", "network/public"))
	if _, ok := got["cluster_id"]; ok {
		t.Fatalf("capability request must not contain cluster_id: %v", got)
	}
	if sel := placementSelector(t, got); len(sel) != 1 || sel[0] != "network/public" {
		t.Fatalf("capability selector = %v, want [network/public]", sel)
	}
}

// assertLandedOnEligibleCluster is a no-op for a non-matrix suite, passes when the
// gateway landed on a cluster that advertises the capability, fails when it landed on
// no cluster or an ineligible one, and records only eligible landings for the matrix's
// observational landed-cluster union.
// Verifies: E2E-CORE-30.01, E2E-CORE-30.02
func TestAssertLandedOnEligibleCluster(t *testing.T) {
	nonMatrix := &E2ESuite{}
	if err := nonMatrix.assertLandedOnEligibleCluster("cluster-anything"); err != nil {
		t.Fatalf("non-matrix suite must be a no-op, got %v", err)
	}

	s := &E2ESuite{
		matrixCapability:       "provider/aws",
		matrixEligibleClusters: map[string]bool{"cluster-A": true, "cluster-B": true},
		landed:                 &landedTracker{},
	}
	if err := s.assertLandedOnEligibleCluster("cluster-A"); err != nil {
		t.Fatalf("landing on an eligible cluster must pass, got %v", err)
	}
	if err := s.assertLandedOnEligibleCluster(""); err == nil {
		t.Fatal("resolving to no cluster must fail")
	}
	err := s.assertLandedOnEligibleCluster("cluster-C")
	if err == nil {
		t.Fatal("landing on an ineligible cluster must fail")
	}
	if !strings.Contains(err.Error(), "cluster-C") || !strings.Contains(err.Error(), "provider/aws") {
		t.Fatalf("diagnostic must name the cluster and the capability, got %q", err.Error())
	}
	if got := s.landed.list(); len(got) != 1 || got[0] != "cluster-A" {
		t.Fatalf("landed union records only eligible landings, got %v, want [cluster-A]", got)
	}
}

// The P2.4 contract-negative and late-binding probes keep their own request shapes
// and never carry cluster_id, so they exercise the placement-validation path
// regardless of which capability a matrix suite targets.
func TestContractNegativeProbesNotTargeted(t *testing.T) {
	for _, driverName := range []string{"kind", "openshift"} {
		got := decodeRequest(t, gatewayCreateRequestForDriver("e2e-probe", driverName))
		if _, ok := got["cluster_id"]; ok {
			t.Fatalf("%s placement probe must not carry cluster_id: %v", driverName, got)
		}
	}
}

// sanitizeCapability makes a capability label safe as a gateway-name segment.
func TestSanitizeCapability(t *testing.T) {
	if got := sanitizeCapability("provider/aws"); got != "provider-aws" {
		t.Fatalf("sanitizeCapability(provider/aws) = %q, want provider-aws", got)
	}
}
