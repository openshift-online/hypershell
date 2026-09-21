package helm

import (
	"testing"
)

func TestBuild_TrustedCAConfigMapName(t *testing.T) {
	tests := []struct {
		name         string
		hasTrustedCA bool
		oidcIssuer   string
		wantPresent  bool
	}{
		{
			name:         "present when HasTrustedCA and OIDC configured",
			hasTrustedCA: true,
			oidcIssuer:   "https://keycloak.example.com/realms/test",
			wantPresent:  true,
		},
		{
			name:         "absent when HasTrustedCA is false",
			hasTrustedCA: false,
			oidcIssuer:   "https://keycloak.example.com/realms/test",
			wantPresent:  false,
		},
		{
			name:         "absent when OIDC issuer is empty",
			hasTrustedCA: true,
			oidcIssuer:   "",
			wantPresent:  false,
		},
		{
			name:         "absent when both false and empty",
			hasTrustedCA: false,
			oidcIssuer:   "",
			wantPresent:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := ValuesBuilder{
				Gateway: GatewayConfig{
					Image: "quay.io/test/gateway:latest",
					OIDC:  OIDCConfig{Issuer: tc.oidcIssuer},
				},
				Namespace:    "test-ns",
				HasTrustedCA: tc.hasTrustedCA,
			}

			values, err := builder.Build()
			if err != nil {
				t.Fatalf("Build() returned error: %v", err)
			}

			server, _ := values["server"].(map[string]interface{})
			var caName interface{}
			if server != nil {
				if oidc, ok := server["oidc"].(map[string]interface{}); ok {
					caName = oidc["caConfigMapName"]
				}
			}

			if tc.wantPresent {
				if caName != "gateway-trusted-ca" {
					t.Errorf("expected caConfigMapName=%q, got %v", "gateway-trusted-ca", caName)
				}
			} else {
				if caName != nil {
					t.Errorf("expected caConfigMapName to be absent, got %v", caName)
				}
			}
		})
	}
}
