// Package gatewayhealth is the single source of truth for the vocabulary the
// HyperShell platform uses to describe a Gateway's health: its lifecycle Phase
// and the canonical status reason recorded alongside a healthy gateway.
//
// Both the API server (phase validation on writes and the per-phase metric) and
// the control plane (which writes phase/status back and gates on phase) import
// this package, so the vocabulary cannot drift between components. See
// specs/platform/gateway-phase-vocabulary.spec.md.
package gatewayhealth

// Phase is the canonical lifecycle state of a Gateway. Values are TitleCase and
// compared case-sensitively.
type Phase string

const (
	// PhasePending is accepted but not yet acted on by the reconciler.
	PhasePending Phase = "Pending"
	// PhaseProvisioning indicates manifests are being applied and the workload
	// (and, for routed gateways, its external exposure) is not yet Ready.
	PhaseProvisioning Phase = "Provisioning"
	// PhaseRunning indicates the gateway is fully serving.
	PhaseRunning Phase = "Running"
	// PhaseDegraded indicates the gateway was provisioned but is currently
	// unhealthy; it is recoverable without user action.
	PhaseDegraded Phase = "Degraded"
	// PhaseFailed indicates provisioning could not complete; recovery requires a
	// change.
	PhaseFailed Phase = "Failed"
)

// StatusHealthy is the canonical human-readable status recorded alongside
// PhaseRunning when a gateway's workload - and, for routed gateways, its
// external exposure - is fully Ready.
const StatusHealthy = "Healthy"

// canonicalPhases lists every allowed phase in lifecycle order. It is the one
// place the allowed-phase set is defined; all other consumers derive from it.
var canonicalPhases = []Phase{
	PhasePending,
	PhaseProvisioning,
	PhaseRunning,
	PhaseDegraded,
	PhaseFailed,
}

// Phases returns the canonical phase set in lifecycle order. The returned slice
// is a copy, so callers cannot mutate the canonical set.
func Phases() []Phase {
	out := make([]Phase, len(canonicalPhases))
	copy(out, canonicalPhases)
	return out
}

// PhaseStrings returns the canonical phase set as strings, in lifecycle order.
func PhaseStrings() []string {
	out := make([]string, len(canonicalPhases))
	for i, p := range canonicalPhases {
		out[i] = string(p)
	}
	return out
}

// IsValidPhase reports whether s is exactly one of the canonical phase values.
// Comparison is case-sensitive: the platform vocabulary is TitleCase.
func IsValidPhase(s string) bool {
	for _, p := range canonicalPhases {
		if string(p) == s {
			return true
		}
	}
	return false
}
