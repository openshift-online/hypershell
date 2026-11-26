package policyinterceptor

import (
	"context"
	"sort"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gi "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/gatewayinterceptorv1"
)

// routesAllowlist is the interceptable allowlist of
// crates/openshell-gateway-interceptors/src/routes.rs at v0.1.8-rhaiv.0 (unchanged
// through v0.1.10-rhaiv.0):
// 29 openshell.v1.OpenShell methods (INTERCEPTABLE_METHODS) plus
// openshell.v1.SandboxUpdates/UpdateSandbox (INTERCEPTABLE_SANDBOX_UPDATE_METHODS).
var routesAllowlist = []string{
	"openshell.v1.OpenShell/CreateSandbox",
	"openshell.v1.OpenShell/AttachSandboxProvider",
	"openshell.v1.OpenShell/DetachSandboxProvider",
	"openshell.v1.OpenShell/DeleteSandbox",
	"openshell.v1.OpenShell/StopSandbox",
	"openshell.v1.OpenShell/StartSandbox",
	"openshell.v1.OpenShell/SuspendSandbox",
	"openshell.v1.OpenShell/ResumeSandbox",
	"openshell.v1.OpenShell/CreateSshSession",
	"openshell.v1.OpenShell/ExposeService",
	"openshell.v1.OpenShell/DeleteService",
	"openshell.v1.OpenShell/RevokeSshSession",
	"openshell.v1.OpenShell/CreateProvider",
	"openshell.v1.OpenShell/ImportProviderProfiles",
	"openshell.v1.OpenShell/UpdateProviderProfiles",
	"openshell.v1.OpenShell/UpdateProvider",
	"openshell.v1.OpenShell/ConfigureProviderRefresh",
	"openshell.v1.OpenShell/RotateProviderCredential",
	"openshell.v1.OpenShell/DeleteProviderRefresh",
	"openshell.v1.OpenShell/DeleteProvider",
	"openshell.v1.OpenShell/DeleteProviderProfile",
	"openshell.v1.OpenShell/UpdateConfig",
	"openshell.v1.OpenShell/SubmitPolicyAnalysis",
	"openshell.v1.OpenShell/ApproveDraftChunk",
	"openshell.v1.OpenShell/RejectDraftChunk",
	"openshell.v1.OpenShell/ApproveAllDraftChunks",
	"openshell.v1.OpenShell/EditDraftChunk",
	"openshell.v1.OpenShell/UndoDraftChunk",
	"openshell.v1.OpenShell/ClearDraftChunks",
	"openshell.v1.SandboxUpdates/UpdateSandbox",
}

// Verifies: PLAT-PGP-10.01
func TestBindingsMatchRoutesAllowlistExactly(t *testing.T) {
	bindings := Bindings()
	if len(bindings) != 30 || len(routesAllowlist) != 30 {
		t.Fatalf("got %d bindings and %d allowlist entries, want 30 each", len(bindings), len(routesAllowlist))
	}
	var got []string
	ids := map[string]bool{}
	for _, b := range bindings {
		got = append(got, b.RPC)
		if b.ID == "" || ids[b.ID] {
			t.Errorf("binding %s has empty or duplicate id %q", b.RPC, b.ID)
		}
		ids[b.ID] = true
		if len(b.Phases) == 0 {
			t.Errorf("binding %s has no phases", b.RPC)
		}
		seen := map[string]bool{}
		for _, p := range b.Phases {
			if (p != PhaseModifyOperation && p != PhaseValidate) || seen[p] {
				t.Errorf("binding %s has invalid or duplicate phase %q", b.RPC, p)
			}
			seen[p] = true
		}
		if b.FailurePolicy != FailClosed && b.FailurePolicy != FailOpen {
			t.Errorf("binding %s has invalid failure policy %q", b.RPC, b.FailurePolicy)
		}
		wantPhases := 1
		if b.RPC == "openshell.v1.OpenShell/CreateSandbox" {
			wantPhases = 2
			if !b.HasPhase(PhaseModifyOperation) || !b.HasPhase(PhaseValidate) {
				t.Errorf("CreateSandbox must bind modify_operation and validate, got %v", b.Phases)
			}
		} else if !b.HasPhase(PhaseValidate) {
			t.Errorf("binding %s must bind validate, got %v", b.RPC, b.Phases)
		}
		if len(b.Phases) != wantPhases {
			t.Errorf("binding %s has %d phases, want %d", b.RPC, len(b.Phases), wantPhases)
		}
	}
	want := append([]string(nil), routesAllowlist...)
	sort.Strings(got)
	sort.Strings(want)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("bindings do not match routes.rs allowlist: got %v, want %v", got, want)
		}
	}
}

// Verifies: PLAT-PGP-11.01
func TestBindingFailurePolicies(t *testing.T) {
	closed := []string{"CreateSandbox", "StartSandbox", "ResumeSandbox", "AttachSandboxProvider",
		"ApproveDraftChunk", "ApproveAllDraftChunks", "EditDraftChunk", "CreateProvider", "UpdateProvider"}
	open := []string{"DeleteSandbox", "StopSandbox", "SuspendSandbox", "RevokeSshSession",
		"RejectDraftChunk", "UndoDraftChunk", "ClearDraftChunks", "DeleteProvider", "DeleteService",
		"UpdateConfig", "SubmitPolicyAnalysis"}
	for _, m := range closed {
		if b, _ := bindingByRPC(ServiceOpenShell, m); b.FailurePolicy != FailClosed {
			t.Errorf("%s: failure policy %q, want fail_closed", m, b.FailurePolicy)
		}
	}
	for _, m := range open {
		if b, _ := bindingByRPC(ServiceOpenShell, m); b.FailurePolicy != FailOpen {
			t.Errorf("%s: failure policy %q, want fail_open", m, b.FailurePolicy)
		}
	}
	if b, _ := bindingByRPC(ServiceSandboxUpdates, "UpdateSandbox"); b.FailurePolicy != FailClosed {
		t.Errorf("UpdateSandbox: failure policy %q, want fail_closed", b.FailurePolicy)
	}
}

// Verifies: AGT-EXT-06.01
func TestManifestRealizesPolicyAsInterceptor(t *testing.T) {
	srv := newTestServer(t, buildBundle(t, testGatewayID, false,
		testPolicy{name: "cpu-cap", mode: "enforce", message: "Keep CPU small.", cedar: cpuOver("1000")}))
	m := srv.Manifest()
	if m.GetName() != InterceptorName || m.GetExpectedAudience() != DefaultAudience || m.GetProviderProfiles() {
		t.Fatalf("unexpected manifest header: %+v", m)
	}
	if len(m.GetBindings()) != len(routesAllowlist) {
		t.Fatalf("manifest has %d bindings, want %d", len(m.GetBindings()), len(routesAllowlist))
	}
	for _, b := range m.GetBindings() {
		for _, p := range b.GetPhases() {
			if p == gi.GatewayInterceptorPhase_GATEWAY_INTERCEPTOR_PHASE_POST_COMMIT ||
				p == gi.GatewayInterceptorPhase_GATEWAY_INTERCEPTOR_PHASE_UNSPECIFIED {
				t.Errorf("binding %s declares phase %v", b.GetSelector().GetRpc(), p)
			}
		}
	}
	ext := m.GetExtension()
	if ext.GetProtocolVersion().GetMajor() != 1 || ext.GetProtocolVersion().GetMinor() != 0 ||
		len(ext.GetSupportedCapabilities()) != 1 || ext.GetSupportedCapabilities()[0] != CapabilityContract {
		t.Fatalf("unexpected extension metadata: %+v", ext)
	}
	// Provider profiles are not vended.
	if _, err := srv.SnapshotProviderProfiles(context.Background(), &gi.ProviderProfileSnapshotRequest{}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("SnapshotProviderProfiles: got %v, want Unimplemented", err)
	}
	// Every non-create binding allows without patches: the interceptor never
	// mutates stored OpenShell workspace, provider, or template resources.
	for _, b := range Bindings() {
		if b.Method() == "CreateSandbox" {
			continue
		}
		res, err := srv.Evaluate(context.Background(), validateEval(t, b.Method(), map[string]any{"name": "x"}))
		if err != nil || !res.GetAllowed() || len(res.GetPatches()) != 0 {
			t.Errorf("%s: got %+v, %v; want allow without patches", b.RPC, res, err)
		}
	}
}
