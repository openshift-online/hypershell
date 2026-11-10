package gateway

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// A generated configuration with no OIDC would ship a gateway with no auth
// mechanism, which refuses to start (issue #809). It must fail before rollout as
// a config validation error so the gateway settles to Failed.
// Verifies: GTW-CFG-01.01, GTW-CFG-01.04
func TestValidateGatewayAuth(t *testing.T) {
	full := OIDCConfig{Issuer: "https://kc.example/realms/x", Audience: "gw-1"}
	tests := []struct {
		name    string
		oidc    OIDCConfig
		wantErr string
	}{
		{name: "issuer and audience pass", oidc: full},
		{name: "no oidc fails", oidc: OIDCConfig{}, wantErr: "missing issuer, audience"},
		{name: "empty issuer fails", oidc: OIDCConfig{Audience: "gw-1"}, wantErr: "missing issuer"},
		{name: "blank audience fails", oidc: OIDCConfig{Issuer: full.Issuer, Audience: " "}, wantErr: "missing audience"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGatewayAuth(GatewayConfig{OIDC: tt.oidc})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			var renderErr *ConfigValidationError
			if !errors.As(err, &renderErr) {
				t.Fatalf("err = %v, want a ConfigValidationError", err)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

// TestConfigValidationError_Unwraps locks the contract the reconciler
// relies on: it detects a config validation failure through the wrapped error chain
// with errors.As so it can settle the gateway to Failed with a reason.
func TestConfigValidationError_Unwraps(t *testing.T) {
	inner := fmt.Errorf("boom")
	wrapped := fmt.Errorf("reconcile gateway g: %w", &ConfigValidationError{Err: inner})

	var target *ConfigValidationError
	if !errors.As(wrapped, &target) {
		t.Fatal("expected errors.As to find ConfigValidationError through wrapping")
	}
	if !errors.Is(wrapped, inner) {
		t.Fatal("expected inner error preserved through unwrap chain")
	}
}
