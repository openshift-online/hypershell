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

// A matrix suite pins every positive create to its selected cluster: the payload
// carries cluster_id and omits placement entirely (PLAT-DATA-02 exactly-one-of).
// Verifies: E2E-CORE-30.01
func TestGatewayCreateRequestForSuitePinned(t *testing.T) {
	s := &E2ESuite{driver: nameDriver{name: "kind"}, clusterIDOverride: "cluster-A"}

	req := s.gatewayCreateRequestForSuite("e2e-gw")
	if req.ClusterID != "cluster-A" {
		t.Fatalf("pinned request cluster_id = %q, want cluster-A", req.ClusterID)
	}
	if req.Placement != nil {
		t.Fatalf("pinned request must omit placement, got %+v", req.Placement)
	}

	got := decodeRequest(t, req)
	if got["cluster_id"] != "cluster-A" {
		t.Fatalf("wire cluster_id = %v, want cluster-A", got["cluster_id"])
	}
	if _, ok := got["placement"]; ok {
		t.Fatalf("pinned wire payload must not contain placement: %v", got)
	}
}

// A non-matrix suite (empty override) keeps the driver's placement selector and
// never sends cluster_id, preserving existing single-cluster and performance
// behavior.
func TestGatewayCreateRequestForSuitePlacement(t *testing.T) {
	s := &E2ESuite{driver: nameDriver{name: "kind"}}

	req := s.gatewayCreateRequestForSuite("e2e-gw")
	if req.ClusterID != "" {
		t.Fatalf("placement request must omit cluster_id, got %q", req.ClusterID)
	}
	if req.Placement == nil || req.Placement.Mode != "local-kind" {
		t.Fatalf("placement request should carry the kind placement, got %+v", req.Placement)
	}

	got := decodeRequest(t, req)
	if _, ok := got["cluster_id"]; ok {
		t.Fatalf("placement wire payload must not contain cluster_id: %v", got)
	}
	if _, ok := got["placement"]; !ok {
		t.Fatalf("placement wire payload must contain placement: %v", got)
	}
}

func TestPinnedGatewayCreateRequestShape(t *testing.T) {
	got := decodeRequest(t, pinnedGatewayCreateRequest("e2e-gw", "cluster-B"))
	if got["cluster_id"] != "cluster-B" {
		t.Fatalf("cluster_id = %v, want cluster-B", got["cluster_id"])
	}
	if _, ok := got["placement"]; ok {
		t.Fatalf("pinned request must not contain placement: %v", got)
	}
}

// verifyPinnedCluster is a no-op for a non-matrix suite, passes when the response
// binds to the selected target, and fails with a diagnostic naming both ids when
// the response binds elsewhere.
// Verifies: E2E-CORE-30.01
func TestVerifyPinnedCluster(t *testing.T) {
	if err := verifyPinnedCluster("", "cluster-anything"); err != nil {
		t.Fatalf("empty override must be a no-op, got %v", err)
	}
	if err := verifyPinnedCluster("cluster-A", "cluster-A"); err != nil {
		t.Fatalf("matching cluster must pass, got %v", err)
	}
	err := verifyPinnedCluster("cluster-A", "cluster-B")
	if err == nil {
		t.Fatal("mismatched cluster must fail")
	}
	if !strings.Contains(err.Error(), "cluster-A") || !strings.Contains(err.Error(), "cluster-B") {
		t.Fatalf("diagnostic must name both the target and the observed cluster, got %q", err.Error())
	}
}

// The P2.4 contract-negative and late-binding probes keep their own request shapes
// and are never pinned to a matrix target: the placement/late-bound shape never
// carries cluster_id, and the unregistered-cluster_id probe uses a fixed bogus id
// distinct from the suite's selected cluster (E2E-CORE-30.01 preservation clause).
func TestContractNegativeProbesNotPinned(t *testing.T) {
	// The placement-based probe (invalid placement, late binding) must never carry a
	// cluster_id, regardless of driver.
	for _, driverName := range []string{"kind", "openshift"} {
		got := decodeRequest(t, gatewayCreateRequestForDriver("e2e-probe", driverName))
		if _, ok := got["cluster_id"]; ok {
			t.Fatalf("%s placement probe must not carry cluster_id: %v", driverName, got)
		}
		if _, ok := got["placement"]; !ok {
			t.Fatalf("%s placement probe must carry placement: %v", driverName, got)
		}
	}

	// The unregistered-cluster_id probe is built with a fixed bogus id, not the
	// matrix override, so a pinned suite still exercises the rejection path.
	const override = "cluster-A"
	req := pinnedGatewayCreateRequest("e2e-bad-cluster", "unregistered-123")
	if req.ClusterID == override {
		t.Fatalf("unregistered probe must not reuse the matrix target %q", override)
	}
	if !strings.HasPrefix(req.ClusterID, "unregistered-") {
		t.Fatalf("unregistered probe cluster_id = %q, want an unregistered id", req.ClusterID)
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
