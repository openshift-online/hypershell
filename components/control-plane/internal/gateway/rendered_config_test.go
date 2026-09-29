package gateway

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestValidateRenderedGatewayConfig(t *testing.T) {
	const base = `
[openshell]
version = 1

[openshell.gateway]
bind_address = "0.0.0.0:8080"
server_sans = ["a.example.com"]

[openshell.gateway.auth]
allow_unauthenticated_users = false
`
	const oidcSection = "\n[openshell.gateway.oidc]\nissuer = \"https://kc.example/realms/x\"\n"

	tests := []struct {
		name    string
		toml    string
		config  GatewayConfig
		wantErr string // substring; empty means expect success
	}{
		{
			name:   "well-formed without oidc passes",
			toml:   base,
			config: GatewayConfig{},
		},
		{
			name:    "malformed toml fails",
			toml:    "[openshell.gateway\nserver_sans = [",
			config:  GatewayConfig{},
			wantErr: "not well-formed TOML",
		},
		{
			name:   "oidc enabled and coherent passes",
			toml:   base + oidcSection,
			config: GatewayConfig{OIDC: OIDCConfig{Issuer: "https://kc.example/realms/x"}},
		},
		{
			name:    "oidc enabled but section missing fails",
			toml:    base,
			config:  GatewayConfig{OIDC: OIDCConfig{Issuer: "https://kc.example/realms/x"}},
			wantErr: "no [openshell.gateway.oidc] section",
		},
		{
			name:    "oidc enabled but issuer empty fails",
			toml:    base + "\n[openshell.gateway.oidc]\naudience = \"x\"\n",
			config:  GatewayConfig{OIDC: OIDCConfig{Issuer: "https://kc.example/realms/x"}},
			wantErr: "no issuer",
		},
		{
			name: "oidc enabled but unauthenticated still allowed fails",
			toml: "\n[openshell.gateway.auth]\nallow_unauthenticated_users = true\n" +
				"\n[openshell.gateway.oidc]\nissuer = \"https://kc.example/realms/x\"\n",
			config:  GatewayConfig{OIDC: OIDCConfig{Issuer: "https://kc.example/realms/x"}},
			wantErr: "still allows unauthenticated users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRenderedGatewayConfig(tt.toml, tt.config)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

// TestRenderedConfigValidationError_Unwraps locks the contract the reconciler
// relies on: it detects a rendered-config failure through the wrapped error chain
// with errors.As so it can settle the gateway to Failed with a reason.
func TestRenderedConfigValidationError_Unwraps(t *testing.T) {
	inner := fmt.Errorf("boom")
	wrapped := fmt.Errorf("reconcile gateway g: %w", &RenderedConfigValidationError{Err: inner})

	var target *RenderedConfigValidationError
	if !errors.As(wrapped, &target) {
		t.Fatal("expected errors.As to find RenderedConfigValidationError through wrapping")
	}
	if !errors.Is(wrapped, inner) {
		t.Fatal("expected inner error preserved through unwrap chain")
	}
}
