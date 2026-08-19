package exposure

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestObserveReadiness(t *testing.T) {
	tests := []struct {
		name      string
		gw        *gatewayv1.Gateway // nil means the per-tenant Gateway does not exist
		wantReady bool
	}{
		{
			name:      "programmed with address is ready",
			gw:        gatewayObj("openshift-ingress", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, []string{"1.2.3.4"}),
			wantReady: true,
		},
		{
			name:      "programmed false is not ready",
			gw:        gatewayObj("openshift-ingress", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionFalse, "Pending")}, []string{"1.2.3.4"}),
			wantReady: false,
		},
		{
			name:      "programmed true but no address is not ready",
			gw:        gatewayObj("openshift-ingress", "gw-tenant-a", []metav1.Condition{programmed(metav1.ConditionTrue, "Programmed")}, nil),
			wantReady: false,
		},
		{
			name:      "no programmed condition is not ready",
			gw:        gatewayObj("openshift-ingress", "gw-tenant-a", []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue}}, []string{"1.2.3.4"}),
			wantReady: false,
		},
		{
			name:      "missing gateway is not ready",
			gw:        nil,
			wantReady: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Seed through Create rather than NewSimpleClientset(objs...): the
			// simple tracker's constructor-seeding does not index objects
			// retrievably in this client version.
			cs := fakegwclient.NewSimpleClientset()
			if tt.gw != nil {
				if _, err := cs.GatewayV1().Gateways(tt.gw.Namespace).Create(context.Background(), tt.gw, metav1.CreateOptions{}); err != nil {
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
