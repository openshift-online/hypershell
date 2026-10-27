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

// Verifies: OSG-ROUT-08.01, OSG-ROUT-08.02
func TestObserveReadiness(t *testing.T) {
	tests := []struct {
		name      string
		dedicated *gatewayv1.Gateway // Gateway gw-<ns> in the tenant namespace; nil means it does not exist
		shared    *gatewayv1.Gateway // Gateway in the ingress namespace; nil means it does not exist
		wantReady bool
	}{
		// A dedicated per-tenant Gateway is authoritative when it exists.
		{
			name:      "dedicated gateway programmed with address is ready",
			dedicated: gatewayObj("tenant-a", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"}),
			wantReady: true,
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

		// Without a dedicated Gateway, the shared Gateway is observed
		// (today's fallback behavior, unchanged).
		{
			name:      "no dedicated gateway falls back to shared programmed gateway",
			shared:    gatewayObj("openshift-ingress", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"}),
			wantReady: true,
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

			adapter := NewGatewayAPIExposure(cs)
			got, err := adapter.ObserveReadiness(context.Background(), Request{Namespace: "tenant-a"})
			if err != nil {
				t.Fatalf("ObserveReadiness returned error: %v", err)
			}
			if got.Ready != tt.wantReady {
				t.Fatalf("Ready = %v, want %v (reason=%q)", got.Ready, tt.wantReady, got.Reason)
			}
			if !got.Ready && got.Reason == "" {
				t.Fatalf("expected a reason when not ready")
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
