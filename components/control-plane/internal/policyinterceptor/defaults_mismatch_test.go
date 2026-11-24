package policyinterceptor

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
	"github.com/openshift-online/hypershell/components/control-plane/internal/policybundle"
)

// bundleWithDefaults builds an enforced-policy bundle that records the given
// fleet CPU request, as the controller does from PlatformPolicySettings.
func bundleWithDefaults(t *testing.T, cpuRequest string) []byte {
	t.Helper()
	n, err := cedar.ParseDefaults(cpuRequest, "2", "1Gi", "4Gi")
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := policybundle.Build(policybundle.Input{
		GatewayID:  testGatewayID,
		GlobalMode: policybundle.ModeAudit,
		Policies: []*pb.PlatformGatewayPolicy{{
			Metadata: &pb.ObjectReference{Id: "p1"}, Name: "cpu-cap", Target: &pb.PlatformGatewayPolicyTarget{All: true},
			EnforcementMode: "enforce", Cedar: cpuOver("4000"),
		}},
		Defaults:          n,
		DefaultQuantities: &policybundle.Defaults{CPURequest: cpuRequest, CPULimit: "2", MemoryRequest: "1Gi", MemoryLimit: "4Gi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := b.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func cpuRequestPatch(t *testing.T, s *Server) string {
	t.Helper()
	op := createOp("s1", map[string]any{"memory": "1Gi"}, map[string]any{"memory": "1Gi"})
	res, err := s.Evaluate(context.Background(), modifyEval(t, op))
	if err != nil || !res.GetAllowed() {
		t.Fatalf("modify: %+v, %v", res, err)
	}
	for _, p := range res.GetPatches() {
		if p.GetPath() == "/spec/template/resources/requests/cpu" {
			return p.GetValue().GetStringValue()
		}
	}
	return ""
}

// Verifies: PLAT-PGP-08.05
func TestSidecarAppliesBundleDefaultsWhenEnvHashDiffers(t *testing.T) {
	// The bundle already carries the new 700m default while the pod, not yet
	// rolled, still has the 500m environment default.
	s := newTestServer(t, bundleWithDefaults(t, "700m"))
	if got := cpuRequestPatch(t, s); got != "700m" {
		t.Fatalf("a stale sidecar must apply the bundle's default, got %q", got)
	}
	if got := mismatchCount(t, s); got != "1" {
		t.Fatalf("the mismatch must be counted, got %s", got)
	}
}

// Verifies: PLAT-PGP-08.05
func TestSidecarUsesEnvDefaultsWhenHashesMatch(t *testing.T) {
	s := newTestServer(t, bundleWithDefaults(t, "0.5"))
	if got := cpuRequestPatch(t, s); got != "500m" {
		t.Fatalf("with matching hashes the env default applies, got %q", got)
	}
	if got := mismatchCount(t, s); got != "0" {
		t.Fatalf("no mismatch may be counted, got %s", got)
	}
}

// mismatchCount scrapes the sidecar metrics endpoint for the mismatch counter.
func mismatchCount(t *testing.T, s *Server) string {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Metrics.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	body, _ := io.ReadAll(rec.Body)
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "hypershell_policy_interceptor_defaults_hash_mismatch_total ") {
			return strings.TrimPrefix(line, "hypershell_policy_interceptor_defaults_hash_mismatch_total ")
		}
	}
	t.Fatal("mismatch counter not exported")
	return ""
}
