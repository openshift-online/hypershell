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

// gatewayRouteName is the name of a tenant's single GRPCRoute, matching the
// reconciler's resource creation
// (specs/platform/openshell-gateway-routing.spec.md § GRPCRoute Resource
// Specification, OSG-ROUT-05). It is duplicated here rather than imported from
// the gateway package to avoid an import cycle between the exposure port and
// its provisioning-side resource-creation code; the two cannot drift apart
// because both are exercised by the OSG-ROUT-05/OSG-ROUT-08 scenario tests.
const gatewayRouteName = "openshell-gateway"

// ObserveReadiness reports whether the gateway's traffic is being served. Ready
// requires all of:
//
//  1. the relevant parent Gateway API `Gateway` reports condition
//     Programmed=True with a non-empty .status.addresses;
//  2. this gateway's own GRPCRoute (gatewayRouteName, in the tenant namespace)
//     is accepted by that parent: any of its status.parents entries matching
//     the parentRef's name, namespace, and sectionName reports both
//     Accepted=True and ResolvedRefs=True, with an observedGeneration equal to
//     the GRPCRoute's own metadata.generation (OSG-ROUT-08.06, OSG-ROUT-08.07);
//  3. req.RouteAddress (the Gateway resource's already-published routeAddress)
//     is non-empty.
//
// A Programmed parent alone is not sufficient: the shared Gateway reports
// Programmed=True for every tenant as soon as any one tenant's GRPCRoute
// attaches to it, independent of this gateway's own route (HYPERSHELL-504).
// Any state short of all three is reported as not Ready with a reason.
//
// The parent-Gateway lookup is two-step, and deliberately discovery-based:
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
		return g.readinessForParent(ctx, req, dedicated, req.Namespace, dedicatedName)
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

	return g.readinessForParent(ctx, req, gw, ns, name)
}

// readinessForParent evaluates the parent Gateway's own readiness and, only
// once it is Programmed with an address, this gateway's own GRPCRoute
// acceptance by that parent and the published routeAddress. parentNamespace
// and parentName identify the parent Gateway (dedicated or shared) that
// gatewayReadiness(parent) already judged, so the GRPCRoute's status.parents
// entry is matched against that exact parentRef.
func (g *GatewayAPIExposure) readinessForParent(ctx context.Context, req Request, parent *gatewayv1.Gateway, parentNamespace, parentName string) (Readiness, error) {
	if r := gatewayReadiness(parent); !r.Ready {
		return r, nil
	}

	route, err := g.client.GatewayV1().GRPCRoutes(req.Namespace).Get(ctx, gatewayRouteName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return Readiness{Ready: false, Reason: "GRPCRoute not accepted"}, nil
		}
		return Readiness{}, fmt.Errorf("get GRPCRoute %s/%s: %w", req.Namespace, gatewayRouteName, err)
	}
	if !routeAcceptedByParent(route, parentName, parentNamespace, req.Namespace) {
		return Readiness{Ready: false, Reason: "GRPCRoute not accepted"}, nil
	}

	if req.RouteAddress == "" {
		return Readiness{Ready: false, Reason: "route address not yet published"}, nil
	}

	return Readiness{Ready: true}, nil
}

// routeAcceptedByParent reports whether route's status.parents carries any
// entry for the (parentName, parentNamespace, sectionName) parentRef reporting
// both Accepted=True and ResolvedRefs=True at the route's current
// metadata.generation (OSG-ROUT-08). A GRPCRoute MAY carry several
// status.parents entries for the same parent name and namespace, for example
// reported by distinct controllers or listeners; every matching entry is
// checked, and a non-matching or False entry evaluated first SHALL NOT
// suppress a later matching entry that reports True (OSG-ROUT-08.06). A
// parentRef with no explicit namespace defaults to the route's own namespace,
// per the Gateway API spec.
//
// sectionName is read from the route's own spec.parentRefs for this parent
// rather than hardcoded, so the match is correct whether the parent is the
// dedicated per-tenant Gateway (sectionName "grpc") or the shared Gateway
// (sectionName from GATEWAY_API_HTTP_LISTENER_NAME); a parentRef with no
// sectionName imposes no constraint.
func routeAcceptedByParent(route *gatewayv1.GRPCRoute, parentName, parentNamespace, routeNamespace string) bool {
	wantSection := expectedSectionName(route, parentName, parentNamespace, routeNamespace)
	for _, p := range route.Status.Parents {
		if !parentRefMatches(p.ParentRef, parentName, parentNamespace, routeNamespace) {
			continue
		}
		if wantSection != "" && parentRefSectionName(p.ParentRef) != wantSection {
			continue
		}
		if conditionTrue(p.Conditions, string(gatewayv1.RouteConditionAccepted), route.Generation) &&
			conditionTrue(p.Conditions, string(gatewayv1.RouteConditionResolvedRefs), route.Generation) {
			return true
		}
	}
	return false
}

// expectedSectionName returns the sectionName the route's own spec.parentRefs
// declares for the (parentName, parentNamespace) parent, or "" when that
// parentRef has no sectionName set (imposing no constraint on the match).
func expectedSectionName(route *gatewayv1.GRPCRoute, parentName, parentNamespace, routeNamespace string) string {
	for _, ref := range route.Spec.ParentRefs {
		if parentRefMatches(ref, parentName, parentNamespace, routeNamespace) {
			if section := parentRefSectionName(ref); section != "" {
				return section
			}
		}
	}
	return ""
}

// parentRefMatches reports whether ref identifies the (parentName,
// parentNamespace) parent. A parentRef with no explicit namespace defaults to
// routeNamespace, per the Gateway API spec. Shared by routeAcceptedByParent
// (matching a GRPCRoute's status.parents entries) and expectedSectionName
// (matching its own spec.parentRefs) so the one namespace-defaulting rule
// cannot drift between the two.
func parentRefMatches(ref gatewayv1.ParentReference, parentName, parentNamespace, routeNamespace string) bool {
	ns := routeNamespace
	if ref.Namespace != nil && *ref.Namespace != "" {
		ns = string(*ref.Namespace)
	}
	return string(ref.Name) == parentName && ns == parentNamespace
}

// parentRefSectionName returns ref's sectionName, or "" when unset.
func parentRefSectionName(ref gatewayv1.ParentReference) string {
	if ref.SectionName != nil {
		return string(*ref.SectionName)
	}
	return ""
}

// conditionTrue reports whether conds contains condType with status True and
// an observedGeneration equal to wantGeneration. A condition's True status is
// trusted only at the generation it was observed at: a stale True left over
// from before a Helm retry re-applied the route (advancing the route's
// generation without yet re-stamping the condition) is treated as not yet
// accepted, not as a carried-over True from the prior generation
// (OSG-ROUT-08.07).
func conditionTrue(conds []metav1.Condition, condType string, wantGeneration int64) bool {
	for _, c := range conds {
		if c.Type == condType {
			return c.Status == metav1.ConditionTrue && c.ObservedGeneration == wantGeneration
		}
	}
	return false
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
