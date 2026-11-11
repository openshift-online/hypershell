package exposure

import (
	"context"
	"errors"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	fakegwclient "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
)

func gatewayObj(namespace, name string, conds []metav1.Condition, addrs []string) *gatewayv1.Gateway {
	statusAddrs := make([]gatewayv1.GatewayStatusAddress, 0, len(addrs))
	for _, a := range addrs {
		statusAddrs = append(statusAddrs, gatewayv1.GatewayStatusAddress{Value: a})
	}
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Status: gatewayv1.GatewayStatus{
			Conditions: conds,
			Addresses:  statusAddrs,
		},
	}
}

func programmed(status metav1.ConditionStatus, reason string) metav1.Condition {
	return metav1.Condition{
		Type:   string(gatewayv1.GatewayConditionProgrammed),
		Status: status,
		Reason: reason,
	}
}

// grpcRouteObj builds the tenant's GRPCRoute (gatewayRouteName, in routeNamespace)
// with the given parent statuses.
func grpcRouteObj(routeNamespace string, parents ...gatewayv1.RouteParentStatus) *gatewayv1.GRPCRoute {
	return &gatewayv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{Name: gatewayRouteName, Namespace: routeNamespace},
		Status: gatewayv1.GRPCRouteStatus{
			RouteStatus: gatewayv1.RouteStatus{Parents: parents},
		},
	}
}

// parentStatus builds a status.parents entry for parentName/parentNamespace
// with the given Accepted/ResolvedRefs condition values.
func parentStatus(parentName, parentNamespace string, accepted, resolvedRefs bool) gatewayv1.RouteParentStatus {
	ns := gatewayv1.Namespace(parentNamespace)
	conds := []metav1.Condition{
		{Type: string(gatewayv1.RouteConditionAccepted), Status: conditionStatus(accepted)},
		{Type: string(gatewayv1.RouteConditionResolvedRefs), Status: conditionStatus(resolvedRefs)},
	}
	return gatewayv1.RouteParentStatus{
		ParentRef: gatewayv1.ParentReference{
			Name:      gatewayv1.ObjectName(parentName),
			Namespace: &ns,
		},
		ControllerName: "example.net/gateway-controller",
		Conditions:     conds,
	}
}

func conditionStatus(b bool) metav1.ConditionStatus {
	if b {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

const publishedAddress = "grpcs://gw-tenant-a.apps.example.com:443"

// Verifies: OSG-ROUT-08.01, OSG-ROUT-08.02, OSG-ROUT-08.04, OSG-ROUT-08.05
func TestObserveReadiness(t *testing.T) {
	tests := []struct {
		name         string
		dedicated    *gatewayv1.Gateway // Gateway gw-<ns> in the tenant namespace; nil means it does not exist
		shared       *gatewayv1.Gateway // Gateway in the ingress namespace; nil means it does not exist
		route        *gatewayv1.GRPCRoute
		routeAddress string
		wantReady    bool
		wantReason   string // substring; checked only when non-empty
	}{
		// A dedicated per-tenant Gateway is authoritative when it exists.
		{
			name:         "dedicated gateway programmed, own route accepted, address published is ready",
			dedicated:    gatewayObj("tenant-a", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"}),
			route:        grpcRouteObj("tenant-a", parentStatus("gw-tenant-a", "tenant-a", true, true)),
			routeAddress: publishedAddress,
			wantReady:    true,
		},
		{
			name:      "dedicated gateway not programmed while shared is programmed is not ready",
			dedicated: gatewayObj("tenant-a", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionFalse, "Pending")}, []string{"1.2.3.4"}),
			shared:    gatewayObj("openshift-ingress", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"}),
			wantReady: false,
		},
		{
			name:      "dedicated gateway programmed false is not ready",
			dedicated: gatewayObj("tenant-a", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionFalse, "Pending")}, []string{"1.2.3.4"}),
			wantReady: false,
		},
		{
			name:      "dedicated gateway programmed true but no address is not ready",
			dedicated: gatewayObj("tenant-a", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, nil),
			wantReady: false,
		},
		{
			name:      "dedicated gateway with no programmed condition is not ready",
			dedicated: gatewayObj("tenant-a", "gw-tenant-a", []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue}}, []string{"1.2.3.4"}),
			wantReady: false,
		},
		{
			// HYPERSHELL-504: a Programmed parent alone must not report Ready when
			// this gateway's own GRPCRoute has not yet been created or accepted -
			// the exact race from the incident (a Helm retry re-applying this
			// gateway's resources while the shared Gateway was already programmed
			// for another tenant).
			name:         "dedicated gateway programmed but own GRPCRoute missing is not ready",
			dedicated:    gatewayObj("tenant-a", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"}),
			routeAddress: publishedAddress,
			wantReady:    false,
			wantReason:   "GRPCRoute not accepted",
		},
		{
			name:         "dedicated gateway programmed but own GRPCRoute not yet accepted is not ready",
			dedicated:    gatewayObj("tenant-a", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"}),
			route:        grpcRouteObj("tenant-a", parentStatus("gw-tenant-a", "tenant-a", false, true)),
			routeAddress: publishedAddress,
			wantReady:    false,
			wantReason:   "GRPCRoute not accepted",
		},
		{
			name:         "dedicated gateway programmed but own GRPCRoute ResolvedRefs false is not ready",
			dedicated:    gatewayObj("tenant-a", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"}),
			route:        grpcRouteObj("tenant-a", parentStatus("gw-tenant-a", "tenant-a", true, false)),
			routeAddress: publishedAddress,
			wantReady:    false,
			wantReason:   "GRPCRoute not accepted",
		},
		{
			name:         "dedicated gateway programmed, route accepted for a different parent is not ready",
			dedicated:    gatewayObj("tenant-a", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"}),
			route:        grpcRouteObj("tenant-a", parentStatus("some-other-gateway", "tenant-a", true, true)),
			routeAddress: publishedAddress,
			wantReady:    false,
			wantReason:   "GRPCRoute not accepted",
		},
		{
			name:      "dedicated gateway programmed, own route accepted, but routeAddress not yet published is not ready",
			dedicated: gatewayObj("tenant-a", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"}),
			route:     grpcRouteObj("tenant-a", parentStatus("gw-tenant-a", "tenant-a", true, true)),
			wantReady: false,
			// routeAddress left empty.
			wantReason: "route address not yet published",
		},

		// Without a dedicated Gateway, the shared Gateway is observed
		// (today's fallback behavior, unchanged).
		{
			name:         "no dedicated gateway, shared programmed, own route accepted, address published is ready",
			shared:       gatewayObj("openshift-ingress", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"}),
			route:        grpcRouteObj("tenant-a", parentStatus("gw-tenant-a", "openshift-ingress", true, true)),
			routeAddress: publishedAddress,
			wantReady:    true,
		},
		{
			name:      "no dedicated gateway and shared not programmed is not ready",
			shared:    gatewayObj("openshift-ingress", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionFalse, "Pending")}, []string{"1.2.3.4"}),
			wantReady: false,
		},
		{
			name:      "no dedicated gateway and shared programmed true but no address is not ready",
			shared:    gatewayObj("openshift-ingress", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, nil),
			wantReady: false,
		},
		{
			name:      "no dedicated gateway and shared with no programmed condition is not ready",
			shared:    gatewayObj("openshift-ingress", "gw-tenant-a", []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue}}, []string{"1.2.3.4"}),
			wantReady: false,
		},
		{
			name:      "no dedicated gateway and missing shared gateway is not ready",
			wantReady: false,
		},
		{
			// This is the exact incident race: the shared Gateway is
			// Programmed=True because another tenant's GRPCRoute already
			// attached to it, but this gateway's own GRPCRoute has not yet been
			// accepted.
			name:         "no dedicated gateway, shared already programmed for another tenant, this gateway's own route not yet accepted is not ready",
			shared:       gatewayObj("openshift-ingress", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"}),
			route:        grpcRouteObj("tenant-a", parentStatus("gw-tenant-a", "openshift-ingress", false, false)),
			routeAddress: publishedAddress,
			wantReady:    false,
			wantReason:   "GRPCRoute not accepted",
		},
		{
			name:      "no dedicated gateway, shared programmed, own route accepted, but routeAddress not yet published is not ready",
			shared:    gatewayObj("openshift-ingress", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"}),
			route:     grpcRouteObj("tenant-a", parentStatus("gw-tenant-a", "openshift-ingress", true, true)),
			wantReady: false,
			// routeAddress left empty.
			wantReason: "route address not yet published",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pin Gateway API placement env vars so the test is isolated from
			// the exported Makefile variables (e.g. GATEWAY_API_GATEWAY_NAMESPACE
			// set by a prior kind-up run in the same shell session).
			t.Setenv("GATEWAY_API_GATEWAY_NAMESPACE", "openshift-ingress")
			t.Setenv("GATEWAY_API_GATEWAY_NAME", "")
			// Seed through Create rather than NewSimpleClientset(objs...): the
			// simple tracker's constructor-seeding does not index objects
			// retrievably in this client version.
			cs := fakegwclient.NewSimpleClientset()
			for _, gw := range []*gatewayv1.Gateway{tt.dedicated, tt.shared} {
				if gw == nil {
					continue
				}
				if _, err := cs.GatewayV1().Gateways(gw.Namespace).Create(context.Background(), gw, metav1.CreateOptions{}); err != nil {
					t.Fatalf("seeding gateway: %v", err)
				}
			}
			if tt.route != nil {
				if _, err := cs.GatewayV1().GRPCRoutes(tt.route.Namespace).Create(context.Background(), tt.route, metav1.CreateOptions{}); err != nil {
					t.Fatalf("seeding GRPCRoute: %v", err)
				}
			}

			adapter := NewGatewayAPIExposure(cs)
			got, err := adapter.ObserveReadiness(context.Background(), Request{Namespace: "tenant-a", RouteAddress: tt.routeAddress})
			if err != nil {
				t.Fatalf("ObserveReadiness returned error: %v", err)
			}
			if got.Ready != tt.wantReady {
				t.Fatalf("Ready = %v, want %v (reason=%q)", got.Ready, tt.wantReady, got.Reason)
			}
			if !got.Ready && got.Reason == "" {
				t.Fatalf("expected a reason when not ready")
			}
			if tt.wantReason != "" && !strings.Contains(got.Reason, tt.wantReason) {
				t.Fatalf("reason = %q, want it to contain %q", got.Reason, tt.wantReason)
			}
		})
	}
}

func TestObserveReadinessDedicatedGatewayError(t *testing.T) {
	t.Setenv("GATEWAY_API_GATEWAY_NAMESPACE", "openshift-ingress")
	t.Setenv("GATEWAY_API_GATEWAY_NAME", "")

	// A shared programmed Gateway exists, so a fallback would report Ready.
	shared := gatewayObj("openshift-ingress", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"})
	cs := fakegwclient.NewSimpleClientset()
	if _, err := cs.GatewayV1().Gateways(shared.Namespace).Create(context.Background(), shared, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seeding shared gateway: %v", err)
	}
	// Fail only the dedicated per-tenant Get: a transient API error must be
	// returned, not silently read as "no dedicated gateway" and fallen back.
	cs.PrependReactor("get", "gateways", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.(k8stesting.GetAction).GetNamespace() == "tenant-a" {
			return true, nil, errors.New("transient apiserver error")
		}
		return false, nil, nil
	})

	adapter := NewGatewayAPIExposure(cs)
	got, err := adapter.ObserveReadiness(context.Background(), Request{Namespace: "tenant-a"})
	if err == nil {
		t.Fatalf("ObserveReadiness expected error, got readiness %+v", got)
	}
	if !strings.Contains(err.Error(), "transient apiserver error") {
		t.Fatalf("error = %v, want it to contain the cause %q", err, "transient apiserver error")
	}
	if !strings.Contains(err.Error(), "tenant-a/gw-tenant-a") {
		t.Fatalf("error = %v, want it to name the dedicated gateway tenant-a/gw-tenant-a", err)
	}
}

// A transient error reading this gateway's own GRPCRoute must propagate, never
// be read as "not accepted" and silently report not-Ready.
//
// Verifies: OSG-ROUT-08
func TestObserveReadinessGRPCRouteError(t *testing.T) {
	t.Setenv("GATEWAY_API_GATEWAY_NAMESPACE", "openshift-ingress")
	t.Setenv("GATEWAY_API_GATEWAY_NAME", "")

	dedicated := gatewayObj("tenant-a", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"})
	cs := fakegwclient.NewSimpleClientset()
	if _, err := cs.GatewayV1().Gateways(dedicated.Namespace).Create(context.Background(), dedicated, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seeding dedicated gateway: %v", err)
	}
	cs.PrependReactor("get", "grpcroutes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("transient apiserver error")
	})

	adapter := NewGatewayAPIExposure(cs)
	got, err := adapter.ObserveReadiness(context.Background(), Request{Namespace: "tenant-a", RouteAddress: publishedAddress})
	if err == nil {
		t.Fatalf("ObserveReadiness expected error, got readiness %+v", got)
	}
	if !strings.Contains(err.Error(), "transient apiserver error") {
		t.Fatalf("error = %v, want it to contain the cause %q", err, "transient apiserver error")
	}
}

func TestResolveAddress(t *testing.T) {
	adapter := NewGatewayAPIExposure(fakegwclient.NewSimpleClientset())

	t.Run("derived from base domain", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
		got, err := adapter.ResolveAddress(context.Background(), Request{Namespace: "tenant-a"})
		if err != nil {
			t.Fatalf("ResolveAddress error: %v", err)
		}
		if want := "grpcs://gw-tenant-a.apps.example.com:443"; got != want {
			t.Fatalf("address = %q, want %q", got, want)
		}
	})

	t.Run("explicit host wins", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
		got, err := adapter.ResolveAddress(context.Background(), Request{Namespace: "tenant-a", Host: "custom.example.com"})
		if err != nil {
			t.Fatalf("ResolveAddress error: %v", err)
		}
		if want := "grpcs://custom.example.com:443"; got != want {
			t.Fatalf("address = %q, want %q", got, want)
		}
	})

	t.Run("empty when base domain unset", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "")
		got, err := adapter.ResolveAddress(context.Background(), Request{Namespace: "tenant-a"})
		if err != nil {
			t.Fatalf("ResolveAddress error: %v", err)
		}
		if got != "" {
			t.Fatalf("address = %q, want empty", got)
		}
	})
}
