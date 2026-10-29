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

// A matrix suite targets every positive create at its selected cluster by the
// cluster's server-assigned identity capability: the payload carries
// placement.selector = [managed-cluster/<name>] and never cluster_id (PLAT-DATA-02,
// E2E-CORE-30).
// Verifies: E2E-CORE-30.01
func TestGatewayCreateRequestForSuiteIdentity(t *testing.T) {
	s := &E2ESuite{driver: nameDriver{name: "kind"}, targetClusterName: "spoke-a", targetClusterID: "cluster-A"}

	req := s.gatewayCreateRequestForSuite("e2e-gw")
	if req.Placement == nil || len(req.Placement.Selector) != 1 || req.Placement.Selector[0] != "managed-cluster/spoke-a" {
		t.Fatalf("matrix request must place by the identity selector, got %+v", req.Placement)
	}

	got := decodeRequest(t, req)
	if _, ok := got["cluster_id"]; ok {
		t.Fatalf("matrix wire payload must not contain cluster_id: %v", got)
	}
	if sel := placementSelector(t, got); len(sel) != 1 || sel[0] != "managed-cluster/spoke-a" {
		t.Fatalf("matrix wire selector = %v, want [managed-cluster/spoke-a]", sel)
	}
}

// A non-matrix suite (empty target) keeps the driver's requirement selector and
// never sends cluster_id, preserving existing single-cluster and performance
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

// identityGatewayCreateRequest carries only name + placement.selector with the
// identity capability, never cluster_id.
func TestIdentityGatewayCreateRequestShape(t *testing.T) {
	got := decodeRequest(t, identityGatewayCreateRequest("e2e-gw", "spoke-b"))
	if _, ok := got["cluster_id"]; ok {
		t.Fatalf("identity request must not contain cluster_id: %v", got)
	}
	if sel := placementSelector(t, got); len(sel) != 1 || sel[0] != "managed-cluster/spoke-b" {
		t.Fatalf("identity selector = %v, want [managed-cluster/spoke-b]", sel)
	}
}

// verifyResolvedCluster is a no-op for a non-matrix suite, passes when the gateway
// resolved to the selected target, and fails with a diagnostic naming both ids when
// it resolved elsewhere.
// Verifies: E2E-CORE-30.01
func TestVerifyResolvedCluster(t *testing.T) {
	if err := verifyResolvedCluster("", "cluster-anything"); err != nil {
		t.Fatalf("empty target must be a no-op, got %v", err)
	}
	if err := verifyResolvedCluster("cluster-A", "cluster-A"); err != nil {
		t.Fatalf("matching cluster must pass, got %v", err)
	}
	err := verifyResolvedCluster("cluster-A", "cluster-B")
	if err == nil {
		t.Fatal("mismatched cluster must fail")
	}
	if !strings.Contains(err.Error(), "cluster-A") || !strings.Contains(err.Error(), "cluster-B") {
		t.Fatalf("diagnostic must name both the target and the observed cluster, got %q", err.Error())
	}
}

// The P2.4 contract-negative and late-binding probes keep their own request shapes
// and are never given the matrix target: the requirement/late-bound shape never
// carries cluster_id or the identity capability (E2E-CORE-30.01 preservation clause).
func TestContractNegativeProbesNotTargeted(t *testing.T) {
	for _, driverName := range []string{"kind", "openshift"} {
		got := decodeRequest(t, gatewayCreateRequestForDriver("e2e-probe", driverName))
		if _, ok := got["cluster_id"]; ok {
			t.Fatalf("%s placement probe must not carry cluster_id: %v", driverName, got)
		}
		for _, label := range placementSelector(t, got) {
			if strings.HasPrefix(label, "managed-cluster/") {
				t.Fatalf("%s placement probe must not carry an identity capability: %v", driverName, got)
			}
		}
	}
}

// A selected cluster without an E2E_MANAGED_KUBECONTEXT_<name> mapping skips
// Kubernetes checks and never falls back to another cluster's context, while a
// mapped cluster runs them.
// Verifies: E2E-CORE-30.03
func TestManagedKubeContextSkip(t *testing.T) {
	if !managedKubeContextSkip("") {
		t.Fatal("missing kube context must skip Kubernetes checks")
	}
	if managedKubeContextSkip("kind-managed-b") {
		t.Fatal("present kube context must not skip Kubernetes checks")
	}
}
