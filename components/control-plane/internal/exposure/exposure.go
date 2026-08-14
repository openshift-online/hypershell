// Package exposure defines the application-owned port through which the control
// plane resolves a gateway's external route address and observes the readiness
// of its external exposure. The Gateway API is one adapter behind this port;
// additional adapters (e.g. OpenShift Route, passthrough Route) can be added
// without changing the reconciler or health-reconciliation logic.
//
// See specs/platform/openshell-gateway-routing.spec.md § Gateway Exposure Port.
package exposure

import "context"

// Request identifies the gateway whose exposure is being resolved or observed.
type Request struct {
	// Namespace is the tenant namespace of the openshell gateway.
	Namespace string
	// Host is an explicit route host, if configured on the Gateway. Empty means
	// the adapter derives the host from the namespace and its own configuration.
	Host string
}

// Readiness describes whether a gateway's external exposure is currently Ready
// to serve traffic. When Ready is false, Reason carries a short human-readable
// descriptor suitable for the Gateway `status` field.
type Readiness struct {
	Ready  bool
	Reason string
}

// Port is the application-owned boundary for external gateway exposure. The
// reconciler and health loop depend only on this interface, never on a concrete
// exposure backend's types.
type Port interface {
	// ResolveAddress returns the external route address for the gateway
	// (e.g. "grpcs://host:443"), or an empty string when it cannot be resolved
	// (for example when the base domain is not configured). A nil error with an
	// empty address means "not resolvable", not a failure.
	ResolveAddress(ctx context.Context, req Request) (string, error)

	// ObserveReadiness reports whether the gateway's external exposure is
	// currently Ready, with a reason when it is not.
	ObserveReadiness(ctx context.Context, req Request) (Readiness, error)
}
