package exposure

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// routeGVR is the OpenShift Route resource the reconciler creates for a tenant
// gateway in Route ingress mode.
var routeGVR = schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}

// gatewayRouteName is the fixed name of the per-tenant Route the reconciler
// emits for a gateway (see reconcileRouteResources).
const gatewayRouteName = "openshell-gateway"

// RouteExposure is the OpenShift Route adapter for the Gateway Exposure port. It
// is selected on clusters that expose tenant gateways through an HAProxy
// passthrough Route rather than the Gateway API (e.g. IBM Cloud ROKS, where the
// Gateway API CRDs exist but the CIO-managed Istio cannot run). It resolves the
// deterministic route address from the tenant namespace and the configured base
// domain, and observes readiness by reading the per-tenant Route's `Admitted`
// ingress condition with the dynamic client (no OpenShift typed client
// dependency needed).
type RouteExposure struct {
	client dynamic.Interface
}

// NewRouteExposure constructs the Route exposure adapter around a dynamic client.
func NewRouteExposure(client dynamic.Interface) *RouteExposure {
	return &RouteExposure{client: client}
}

// ResolveAddress returns the deterministic external address for a Route-exposed
// gateway: grpcs://<host>:443, using the same host derivation as the Gateway API
// adapter so the address published through the port matches the hostname baked
// into the Route. It returns an empty address (nil error) when the host cannot
// be derived (GATEWAY_API_BASE_DOMAIN unset and no explicit host).
func (r *RouteExposure) ResolveAddress(ctx context.Context, req Request) (string, error) {
	host, ok := DeriveGatewayAPIHost(req.Namespace, req.Host)
	if !ok {
		return "", nil
	}
	return fmt.Sprintf("grpcs://%s:443", host), nil
}

// ObserveReadiness reports whether the per-tenant Route has been admitted by the
// OpenShift router: at least one `status.ingress[]` entry must carry an
// `Admitted=True` condition. Any other state is reported as not Ready with a
// reason suitable for the Gateway `status` field.
func (r *RouteExposure) ObserveReadiness(ctx context.Context, req Request) (Readiness, error) {
	route, err := r.client.Resource(routeGVR).Namespace(req.Namespace).Get(ctx, gatewayRouteName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return Readiness{Ready: false, Reason: "gateway Route not found"}, nil
		}
		return Readiness{}, fmt.Errorf("get route %s/%s: %w", req.Namespace, gatewayRouteName, err)
	}

	ingress, found, err := unstructured.NestedSlice(route.Object, "status", "ingress")
	if err != nil {
		return Readiness{}, fmt.Errorf("read route %s/%s status.ingress: %w", req.Namespace, gatewayRouteName, err)
	}
	if !found || len(ingress) == 0 {
		return Readiness{Ready: false, Reason: "route not yet admitted by any router"}, nil
	}

	notAdmittedReason := "route Admitted condition not reported"
	for _, ing := range ingress {
		ingMap, ok := ing.(map[string]interface{})
		if !ok {
			continue
		}
		conds, _, _ := unstructured.NestedSlice(ingMap, "conditions")
		for _, c := range conds {
			cond, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _, _ := unstructured.NestedString(cond, "type")
			if condType != "Admitted" {
				continue
			}
			status, _, _ := unstructured.NestedString(cond, "status")
			if status == "True" {
				return Readiness{Ready: true}, nil
			}
			reason, _, _ := unstructured.NestedString(cond, "reason")
			if reason == "" {
				reason = "router rejected the route"
			}
			notAdmittedReason = fmt.Sprintf("route not admitted: %s", reason)
		}
	}

	return Readiness{Ready: false, Reason: notAdmittedReason}, nil
}
