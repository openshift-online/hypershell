package gateway

import (
	"testing"
)

// Verifies: PLAT-ARCH-09.01, PLAT-ARCH-09.02
func TestGatewayIngressMode(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		setEnv   bool
		opts     ReconcileOpts
		want     string
	}{
		{
			name:   "explicit gateway-api override wins without Gateway API detected",
			setEnv: true, envValue: "gateway-api",
			opts: ReconcileOpts{HasGatewayAPI: false},
			want: IngressModeGatewayAPI,
		},
		{
			name:   "gatewayapi alias maps to gateway-api",
			setEnv: true, envValue: "GatewayAPI",
			opts: ReconcileOpts{HasGatewayAPI: false},
			want: IngressModeGatewayAPI,
		},
		{
			name:   "explicit none override wins over detected Gateway API",
			setEnv: true, envValue: "none",
			opts: ReconcileOpts{HasGatewayAPI: true},
			want: IngressModeNone,
		},
		{
			name:   "off alias disables managed ingress",
			setEnv: true, envValue: "off",
			opts: ReconcileOpts{HasGatewayAPI: true},
			want: IngressModeNone,
		},
		{
			name:   "disabled alias disables managed ingress",
			setEnv: true, envValue: " Disabled ",
			opts: ReconcileOpts{HasGatewayAPI: true},
			want: IngressModeNone,
		},
		{
			name: "auto-detect selects Gateway API when present",
			opts: ReconcileOpts{HasGatewayAPI: true},
			want: IngressModeGatewayAPI,
		},
		{
			name: "auto-detect yields none without Gateway API",
			opts: ReconcileOpts{HasGatewayAPI: false},
			want: IngressModeNone,
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

	t.Run("explicit host equal to own slot under base domain is allowed", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
		ns := NamespaceConfig{Name: "tenant-a", Gateway: GatewayConfig{Route: RouteConfig{Host: "gw-tenant-a.apps.example.com"}}}
		got, err := deriveGatewayHostname(ns)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "gw-tenant-a.apps.example.com" {
			t.Errorf("got %q, want gw-tenant-a.apps.example.com", got)
		}
	})

	t.Run("rejects another tenant's slot under the shared base domain (hijack)", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
		ns := NamespaceConfig{Name: "tenant-a", Gateway: GatewayConfig{Route: RouteConfig{Host: "gw-tenant-b.apps.example.com"}}}
		if _, err := deriveGatewayHostname(ns); err == nil {
			t.Error("expected hijack rejection, got nil")
		}
	})

	t.Run("rejects an arbitrary host under the shared base domain", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
		ns := NamespaceConfig{Name: "tenant-a", Gateway: GatewayConfig{Route: RouteConfig{Host: "victim.apps.example.com"}}}
		if _, err := deriveGatewayHostname(ns); err == nil {
			t.Error("expected rejection of foreign host under base domain, got nil")
		}
	})

	t.Run("external vanity host outside base domain passes through", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
		ns := NamespaceConfig{Name: "tenant-a", Gateway: GatewayConfig{Route: RouteConfig{Host: "gateway.customer.io"}}}
		got, err := deriveGatewayHostname(ns)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "gateway.customer.io" {
			t.Errorf("got %q, want gateway.customer.io", got)
		}
	})
}
