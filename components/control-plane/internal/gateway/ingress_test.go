package gateway

import (
	"testing"
)

func TestGatewayIngressMode(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		setEnv   bool
		opts     ReconcileOpts
		want     string
	}{
		{
			name:   "explicit route override wins over capabilities",
			setEnv: true, envValue: "route",
			opts: ReconcileOpts{HasGatewayAPI: true, IsOpenShift: true},
			want: ingressModeRoute,
		},
		{
			name:   "explicit gateway-api override",
			setEnv: true, envValue: "gateway-api",
			opts: ReconcileOpts{HasGatewayAPI: false, IsOpenShift: true},
			want: ingressModeGatewayAPI,
		},
		{
			name:   "routes alias maps to route",
			setEnv: true, envValue: "Routes",
			opts: ReconcileOpts{IsOpenShift: true},
			want: ingressModeRoute,
		},
		{
			name:   "none disables managed ingress",
			setEnv: true, envValue: "none",
			opts: ReconcileOpts{HasGatewayAPI: true, IsOpenShift: true},
			want: ingressModeNone,
		},
		{
			name: "auto-detect prefers Gateway API when present",
			opts: ReconcileOpts{HasGatewayAPI: true, IsOpenShift: true},
			want: ingressModeGatewayAPI,
		},
		{
			name: "auto-detect falls back to Route on OpenShift without Gateway API",
			opts: ReconcileOpts{HasGatewayAPI: false, IsOpenShift: true},
			want: ingressModeRoute,
		},
		{
			name: "auto-detect yields none on a plain cluster",
			opts: ReconcileOpts{HasGatewayAPI: false, IsOpenShift: false},
			want: ingressModeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv("GATEWAY_INGRESS_MODE", tt.envValue)
			} else {
				t.Setenv("GATEWAY_INGRESS_MODE", "")
			}
			if got := gatewayIngressMode(tt.opts); got != tt.want {
				t.Errorf("gatewayIngressMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeriveGatewayHostname(t *testing.T) {
	t.Run("explicit host wins", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "example.com")
		ns := NamespaceConfig{Name: "tenant-a", Gateway: GatewayConfig{Route: RouteConfig{Host: "custom.example.net"}}}
		got, err := deriveGatewayHostname(ns)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "custom.example.net" {
			t.Errorf("got %q, want custom.example.net", got)
		}
	})

	t.Run("derived from base domain", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
		ns := NamespaceConfig{Name: "tenant-a"}
		got, err := deriveGatewayHostname(ns)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "gw-tenant-a.apps.example.com" {
			t.Errorf("got %q, want gw-tenant-a.apps.example.com", got)
		}
	})

	t.Run("errors when neither host nor base domain set", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "")
		ns := NamespaceConfig{Name: "tenant-a"}
		if _, err := deriveGatewayHostname(ns); err == nil {
			t.Error("expected error, got nil")
		}
	})
}
