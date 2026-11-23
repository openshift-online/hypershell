// Package policyinterceptor implements the HyperShell policy interceptor
// sidecar (PLAT-PGP-09): an OpenShell GatewayInterceptor service that runs in
// each gateway pod, watches the per-gateway policy bundle ConfigMap, evaluates
// platform gateway policies with the shared Cedar package, and reports audit
// findings to the control plane.
package policyinterceptor

import (
	"strings"
)

// Registration constants shared with the controller, which renders the
// gateway.toml interceptor registration from the same values so Describe and the
// registration always agree under binding_policy = "exact" (PLAT-PGP-10,
// GTW-CFG-05).
const (
	// InterceptorName is the registered interceptor instance name.
	InterceptorName = "hypershell-policy"
	// DefaultAudience is the OpenShell default extension audience for
	// InterceptorName (urn:openshell:extension:interceptor:<name>).
	DefaultAudience = "urn:openshell:extension:interceptor:" + InterceptorName
	// BindingPolicy is the gateway binding policy the registration uses.
	BindingPolicy = "exact"

	// PhaseModifyOperation and PhaseValidate are the gateway.toml phase names.
	PhaseModifyOperation = "modify_operation"
	PhaseValidate        = "validate"

	// FailClosed and FailOpen are the gateway.toml failure policy values.
	FailClosed = "fail_closed"
	FailOpen   = "fail_open"

	// ServiceOpenShell and ServiceSandboxUpdates are the public OpenShell gRPC
	// services that carry interceptable RPCs.
	ServiceOpenShell      = "openshell.v1.OpenShell"
	ServiceSandboxUpdates = "openshell.v1.SandboxUpdates"
)

// Binding is one static interceptor binding: an interceptable RPC, the phases
// the interceptor evaluates, and the failure policy the gateway applies when the
// interceptor cannot return a valid result (PLAT-PGP-11).
type Binding struct {
	// ID is the stable manifest binding id.
	ID string
	// RPC is the full selector, for example "openshell.v1.OpenShell/CreateSandbox".
	RPC string
	// Phases are gateway.toml phase names (PhaseModifyOperation, PhaseValidate).
	Phases []string
	// FailurePolicy is FailClosed or FailOpen.
	FailurePolicy string
}

// Service returns the gRPC service part of RPC.
func (b Binding) Service() string {
	service, _, _ := strings.Cut(b.RPC, "/")
	return service
}

// Method returns the gRPC method part of RPC.
func (b Binding) Method() string {
	_, method, _ := strings.Cut(b.RPC, "/")
	return method
}

// HasPhase reports whether the binding declares phase.
func (b Binding) HasPhase(phase string) bool {
	for _, p := range b.Phases {
		if p == phase {
			return true
		}
	}
	return false
}

type bindingSpec struct {
	service string
	method  string
	failure string
}

// bindingSpecs lists every RPC in the gateway's interceptable allowlist at the
// OpenShell release HyperShell deploys (crates/openshell-gateway-interceptors/
// src/routes.rs, INTERCEPTABLE_METHODS plus INTERCEPTABLE_SANDBOX_UPDATE_METHODS).
// Binding all of them keeps the registration stable across policy changes
// (PLAT-PGP-10); only CreateSandbox, StartSandbox, and ResumeSandbox are
// evaluated against policies today, every other binding allows.
//
// Failure policy (PLAT-PGP-11):
//   - fail_closed: RPCs that create, start, resume, attach, or otherwise
//     establish or widen what a sandbox or provider may do (create, start,
//     resume, attach, SSH session creation, service exposure, provider creation
//     and update, provider profile import and update, refresh configuration,
//     draft-chunk approval and edit, which are user-driven policy writes, and
//     SandboxUpdates/UpdateSandbox, which can change the image of a stopped
//     sandbox).
//   - fail_open: RPCs that delete, stop, suspend, detach, revoke, reject, undo,
//     or clear, so an interceptor outage never blocks cleanup.
//   - fail_open by choice for the remaining RPCs:
//   - UpdateConfig and SubmitPolicyAnalysis are supervisor-originated on the
//     hot path of running sandboxes (configuration and policy status reports,
//     analysis proposals). The interceptor allows them unconditionally, so
//     fail_open only matters during an outage, where it keeps running
//     sandboxes working. A proposal from SubmitPolicyAnalysis changes nothing
//     until a user approves it through a fail_closed draft-chunk RPC.
//   - RotateProviderCredential replaces a credential and never widens what a
//     sandbox may do; blocking rotation during an outage would only delay a
//     security-hygiene action.
var bindingSpecs = []bindingSpec{
	{ServiceOpenShell, "CreateSandbox", FailClosed},
	{ServiceOpenShell, "AttachSandboxProvider", FailClosed},
	{ServiceOpenShell, "DetachSandboxProvider", FailOpen},
	{ServiceOpenShell, "DeleteSandbox", FailOpen},
	{ServiceOpenShell, "StopSandbox", FailOpen},
	{ServiceOpenShell, "StartSandbox", FailClosed},
	{ServiceOpenShell, "SuspendSandbox", FailOpen},
	{ServiceOpenShell, "ResumeSandbox", FailClosed},
	{ServiceOpenShell, "CreateSshSession", FailClosed},
	{ServiceOpenShell, "ExposeService", FailClosed},
	{ServiceOpenShell, "DeleteService", FailOpen},
	{ServiceOpenShell, "RevokeSshSession", FailOpen},
	{ServiceOpenShell, "CreateProvider", FailClosed},
	{ServiceOpenShell, "ImportProviderProfiles", FailClosed},
	{ServiceOpenShell, "UpdateProviderProfiles", FailClosed},
	{ServiceOpenShell, "UpdateProvider", FailClosed},
	{ServiceOpenShell, "ConfigureProviderRefresh", FailClosed},
	{ServiceOpenShell, "RotateProviderCredential", FailOpen},
	{ServiceOpenShell, "DeleteProviderRefresh", FailOpen},
	{ServiceOpenShell, "DeleteProvider", FailOpen},
	{ServiceOpenShell, "DeleteProviderProfile", FailOpen},
	{ServiceOpenShell, "UpdateConfig", FailOpen},
	{ServiceOpenShell, "SubmitPolicyAnalysis", FailOpen},
	{ServiceOpenShell, "ApproveDraftChunk", FailClosed},
	{ServiceOpenShell, "RejectDraftChunk", FailOpen},
	{ServiceOpenShell, "ApproveAllDraftChunks", FailClosed},
	{ServiceOpenShell, "EditDraftChunk", FailClosed},
	{ServiceOpenShell, "UndoDraftChunk", FailOpen},
	{ServiceOpenShell, "ClearDraftChunks", FailOpen},
	{ServiceSandboxUpdates, "UpdateSandbox", FailClosed},
}

// Bindings returns the static binding table in a stable order. The controller
// renders the gateway.toml registration from this table and Describe returns
// it, so both always match under binding_policy = "exact".
func Bindings() []Binding {
	out := make([]Binding, 0, len(bindingSpecs))
	for _, s := range bindingSpecs {
		phases := []string{PhaseValidate}
		if s.service == ServiceOpenShell && s.method == "CreateSandbox" {
			// modify_operation adds fleet defaults (PLAT-PGP-08), validate
			// evaluates the policies (PLAT-PGP-13).
			phases = []string{PhaseModifyOperation, PhaseValidate}
		}
		out = append(out, Binding{
			ID:            bindingID(s.service, s.method),
			RPC:           s.service + "/" + s.method,
			Phases:        phases,
			FailurePolicy: s.failure,
		})
	}
	return out
}

// bindingByRPC returns the binding for a service and method.
func bindingByRPC(service, method string) (Binding, bool) {
	b, ok := bindingIndex[service+"/"+method]
	return b, ok
}

var bindingIndex = func() map[string]Binding {
	out := map[string]Binding{}
	for _, b := range Bindings() {
		out[b.RPC] = b
	}
	return out
}()

// bindingID derives a stable, readable id from the method name, for example
// "create-sandbox". The SandboxUpdates service is prefixed so ids stay unique
// should OpenShell ever make a same-named method interceptable on both services.
func bindingID(service, method string) string {
	var b strings.Builder
	if service == ServiceSandboxUpdates {
		b.WriteString("sandbox-updates-")
	}
	for i, r := range method {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r - 'A' + 'a')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
