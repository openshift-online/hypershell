package reconciler

import "testing"

func TestResolveDefaultGatewayOIDC(t *testing.T) {
	tests := []struct {
		name         string
		env          map[string]string
		wantOK       bool
		wantIssuer   string
		wantClientID string
		wantAudience string
	}{
		{
			name:   "no issuer configured leaves gateways unauthenticated",
			env:    map[string]string{},
			wantOK: false,
		},
		{
			name:         "defaults from OIDC_ISSUER with openshell-cli client and audience",
			env:          map[string]string{"OIDC_ISSUER": "https://keycloak.example.com/realms/hypershell"},
			wantOK:       true,
			wantIssuer:   "https://keycloak.example.com/realms/hypershell",
			wantClientID: "openshell-cli",
			wantAudience: "openshell-cli",
		},
		{
			name: "GATEWAY_OIDC_ISSUER overrides OIDC_ISSUER",
			env: map[string]string{
				"OIDC_ISSUER":         "https://cp-issuer.example.com/realms/cp",
				"GATEWAY_OIDC_ISSUER": "https://gw-issuer.example.com/realms/gw",
			},
			wantOK:       true,
			wantIssuer:   "https://gw-issuer.example.com/realms/gw",
			wantClientID: "openshell-cli",
			wantAudience: "openshell-cli",
		},
		{
			name: "client id and audience are overridable",
			env: map[string]string{
				"OIDC_ISSUER":            "https://keycloak.example.com/realms/hypershell",
				"GATEWAY_OIDC_CLIENT_ID": "custom-cli",
				"GATEWAY_OIDC_AUDIENCE":  "custom-aud",
			},
			wantOK:       true,
			wantIssuer:   "https://keycloak.example.com/realms/hypershell",
			wantClientID: "custom-cli",
			wantAudience: "custom-aud",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all inputs, then apply the case's environment.
			for _, key := range []string{"OIDC_ISSUER", "GATEWAY_OIDC_ISSUER", "GATEWAY_OIDC_CLIENT_ID", "GATEWAY_OIDC_AUDIENCE"} {
				t.Setenv(key, "")
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			got, ok := resolveDefaultGatewayOIDC()
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.Issuer != tt.wantIssuer {
				t.Errorf("issuer = %q, want %q", got.Issuer, tt.wantIssuer)
			}
			if got.ClientID != tt.wantClientID {
				t.Errorf("client_id = %q, want %q", got.ClientID, tt.wantClientID)
			}
			if got.Audience != tt.wantAudience {
				t.Errorf("audience = %q, want %q", got.Audience, tt.wantAudience)
			}
		})
	}
}
