package helm

import (
	"testing"
)

// TestBuild_IngressExposure locks in that the gateway exposure resource follows
// the resolved ingress mode (UseOpenShiftRoute), not raw Gateway API capability.
// The regression it guards: on IBM ROKS the Gateway API CRDs are present but no
// controller runs, so keying the decision off capability emitted a dead
// GRPCRoute and never enabled the chart's openshiftRoute -- the gateway got no
// Route and hung on "Verifying gateway health".
func TestBuild_IngressExposure(t *testing.T) {
	nested := func(m map[string]interface{}, path ...string) interface{} {
		var cur interface{} = m
		for _, k := range path {
			mm, ok := cur.(map[string]interface{})
			if !ok {
				return nil
			}
			cur = mm[k]
		}
		return cur
	}

	tests := []struct {
		name              string
		useOpenShiftRoute bool
		externalCAIssuer  string
		wantRouteEnabled  bool
		wantIssuerSet     bool
		wantMtlsDisabled  bool
	}{
		{
			name:              "default (Gateway API) does not enable openshiftRoute",
			useOpenShiftRoute: false,
			wantRouteEnabled:  false,
			wantMtlsDisabled:  true,
		},
		{
			name:              "route mode without issuer enables self-signed passthrough Route",
			useOpenShiftRoute: true,
			externalCAIssuer:  "",
			wantRouteEnabled:  true,
			wantIssuerSet:     false,
		},
		{
			name:              "route mode with issuer wires cert-manager",
			useOpenShiftRoute: true,
			externalCAIssuer:  "gateway-ca",
			wantRouteEnabled:  true,
			wantIssuerSet:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := ValuesBuilder{
				Gateway: GatewayConfig{
					Image: "quay.io/test/gateway:latest",
					Route: RouteConfig{Host: "gw-test.apps.example.com", Enabled: true},
				},
				Namespace:            "test-ns",
				IsOpenShift:          true,
				HasCertManager:       true,
				IngressBaseDomain:    "apps.example.com",
				UseOpenShiftRoute:    tc.useOpenShiftRoute,
				ExternalCAIssuerName: tc.externalCAIssuer,
				ExternalCAIssuerKind: "ClusterIssuer",
			}

			values, err := builder.Build()
			if err != nil {
				t.Fatalf("Build() returned error: %v", err)
			}

			routeEnabled := nested(values, "openshiftRoute", "enabled") == true
			if routeEnabled != tc.wantRouteEnabled {
				t.Errorf("openshiftRoute.enabled = %v, want %v", routeEnabled, tc.wantRouteEnabled)
			}

			issuerSet := nested(values, "certManager", "serverIssuerRef", "name") != nil
			if issuerSet != tc.wantIssuerSet {
				t.Errorf("certManager.serverIssuerRef.name set = %v, want %v", issuerSet, tc.wantIssuerSet)
			}

			if tc.wantMtlsDisabled {
				if nested(values, "server", "tls", "enableMtls") != false {
					t.Errorf("expected server.tls.enableMtls=false in Gateway API mode, got %v",
						nested(values, "server", "tls", "enableMtls"))
				}
			}
		})
	}
}

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
