// Package exposure defines the application-owned port through which the control
// plane resolves a gateway's external route address and observes the readiness
// of its external exposure. The Gateway API adapter implements this port, so
// the reconciler and health-reconciliation logic do not depend on it directly.
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
	// RouteAddress is the gateway's already-persisted routeAddress (the Gateway
	// resource's `routeAddress` field), as known by the caller. It is only
	// consulted by ObserveReadiness, never by ResolveAddress (which computes the
	// address itself). A gateway is not Ready until its own routeAddress has been
	// published, so an empty value here reports not-Ready even when the
	// underlying Gateway API resources are otherwise healthy. See
	// specs/platform/openshell-gateway-routing.spec.md § Gateway Exposure Port
	// (OSG-ROUT-08).
	RouteAddress string
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
