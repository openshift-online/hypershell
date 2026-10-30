package exposure

import (
	"context"
	"fmt"
	"os"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
)

// GatewayAPIExposure is the Gateway API adapter for the Gateway Exposure port.
// It resolves the deterministic route address from the tenant namespace and the
// configured base domain, and observes readiness by reading the status of the
// Gateway API `Gateway` object that serves the tenant's traffic with the typed
// gateway-api client.
type GatewayAPIExposure struct {
	client gatewayclient.Interface
}

// NewGatewayAPIExposure constructs the Gateway API exposure adapter around a
// typed gateway-api clientset.
func NewGatewayAPIExposure(client gatewayclient.Interface) *GatewayAPIExposure {
	return &GatewayAPIExposure{client: client}
}

// ResolveAddress returns the deterministic external address for a Gateway API
// exposed gateway: grpcs://<host>:443. It returns an empty address (nil error)
// when the host cannot be derived, for example when GATEWAY_API_BASE_DOMAIN is
// unset and no explicit host was configured.
func (g *GatewayAPIExposure) ResolveAddress(ctx context.Context, req Request) (string, error) {
	host, ok := DeriveGatewayAPIHost(req.Namespace, req.Host)
	if !ok {
		return "", nil
	}
	return fmt.Sprintf("grpcs://%s:443", host), nil
}

// ObserveReadiness reports whether the gateway's traffic is being served: the
// observed Gateway API `Gateway` must report condition Programmed=True and
// carry a non-empty .status.addresses. Any other state is reported as not
// Ready with a reason.
//
// The lookup is two-step, and deliberately discovery-based:
//
//  1. Try the dedicated per-gateway `Gateway` `gw-<namespace>` in the tenant
//     namespace. Gateways that opt in to a per-gateway certificate get their
//     own Gateway there (created by the reconciler). When it exists, its
//     readiness is authoritative: the opted-in gateway's GRPCRoute/HTTPRoute
//     attach to it, not to the shared Gateway.
//  2. When it does not exist, fall back to the shared Gateway in the ingress
//     namespace (honoring GATEWAY_API_GATEWAY_NAME and
//     GATEWAY_API_GATEWAY_NAMESPACE), which is the only Gateway serving
//     tenants that did not opt in.
//
// The adapter has no opt-in flag on Request on purpose: the reconciler stays
// decoupled from the exposure port, and the presence of the per-tenant
// Gateway is itself the signal that the gateway opted in. A per-tenant Get
// error other than NotFound is returned, never silently treated as
// "no dedicated gateway" - a transient API error must not downgrade the
// observation to the shared Gateway.
func (g *GatewayAPIExposure) ObserveReadiness(ctx context.Context, req Request) (Readiness, error) {
	dedicatedName := dedicatedGatewayName(req.Namespace)
	dedicated, err := g.client.GatewayV1().Gateways(req.Namespace).Get(ctx, dedicatedName, metav1.GetOptions{})
	if err == nil {
		return gatewayReadiness(dedicated), nil
	}
	if !k8serrors.IsNotFound(err) {
		return Readiness{}, fmt.Errorf("get dedicated gateway %s/%s: %w", req.Namespace, dedicatedName, err)
	}

	// No dedicated Gateway: the tenant is served by the shared Gateway.
	ns := gatewayIngressNamespace()
	name := perTenantGatewayName(req.Namespace)

	gw, err := g.client.GatewayV1().Gateways(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return Readiness{Ready: false, Reason: "per-tenant Gateway not found"}, nil
		}
		return Readiness{}, fmt.Errorf("get gateway %s/%s: %w", ns, name, err)
	}

	return gatewayReadiness(gw), nil
}

// gatewayReadiness evaluates a Gateway's Programmed condition and assigned
// addresses into a Readiness observation. Both the dedicated per-tenant and
// the shared fallback Gateway are judged by this one helper so the criteria
// cannot drift between the two paths.
func gatewayReadiness(gw *gatewayv1.Gateway) Readiness {
	programmed := false
	programmedReason := "Programmed condition not reported"
	for _, cond := range gw.Status.Conditions {
		if cond.Type == string(gatewayv1.GatewayConditionProgrammed) {
			programmed = cond.Status == metav1.ConditionTrue
			if !programmed {
				programmedReason = fmt.Sprintf("gateway not programmed: %s", cond.Reason)
			}
			break
		}
	}
	if !programmed {
		return Readiness{Ready: false, Reason: programmedReason}
	}
	if len(gw.Status.Addresses) == 0 {
		return Readiness{Ready: false, Reason: "gateway has no assigned address"}
	}
	return Readiness{Ready: true}
}

// DeriveGatewayAPIHost returns the external hostname for a Gateway API exposed
// gateway. An explicit host is used verbatim; otherwise the host is derived as
// gw-<namespace>.<GATEWAY_API_BASE_DOMAIN>. The bool is false when no host can
// be derived (base domain unset and no explicit host).
//
// This is the single source of truth for the hostname so the address published
// through the port and the hostname used to build the Gateway/GRPCRoute
// resources cannot drift apart.
func DeriveGatewayAPIHost(namespace, explicitHost string) (string, bool) {
	if explicitHost != "" {
		return explicitHost, true
	}
	baseDomain := os.Getenv("GATEWAY_API_BASE_DOMAIN")
	if baseDomain == "" {
		return "", false
	}
	return fmt.Sprintf("gw-%s.%s", namespace, baseDomain), true
}

// dedicatedGatewayName returns the name of the dedicated per-gateway Gateway
// API Gateway created in the tenant namespace for gateways that opt in to a
// per-gateway certificate. Unlike the shared Gateway lookup, this name is
// not configurable: it is always the gw-<namespace> convention, matching the
// reconciler's resource creation for opted-in gateways.
func dedicatedGatewayName(namespace string) string {
	return "gw-" + namespace
}

// perTenantGatewayName returns the name of the shared Gateway API Gateway
// for a tenant namespace. A shared Gateway name configured via
// GATEWAY_API_GATEWAY_NAME takes precedence over the per-tenant gw-<namespace>
// convention, mirroring the reconciler's resource creation.
func perTenantGatewayName(namespace string) string {
	if shared := os.Getenv("GATEWAY_API_GATEWAY_NAME"); shared != "" {
		return shared
	}
	return "gw-" + namespace
}

// gatewayIngressNamespace returns the namespace where per-tenant Gateway API
// Gateway resources live, matching the reconciler's placement in
// openshift-ingress (configurable via GATEWAY_API_GATEWAY_NAMESPACE).
func gatewayIngressNamespace() string {
	if ns := os.Getenv("GATEWAY_API_GATEWAY_NAMESPACE"); ns != "" {
		return ns
	}
	return "openshift-ingress"
}
