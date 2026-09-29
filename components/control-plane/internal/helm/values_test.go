package helm

import (
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
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

// TestBuild_GatewayResources guards that the gateway container always gets
// explicit requests and limits (the upstream chart defaults to `resources: {}`,
// which leaves the gateway BestEffort), and that a GATEWAY_RESOURCES override
// replaces the defaults.
func TestBuild_GatewayResources(t *testing.T) {
	override, err := ParseGatewayResources(`{"limits":{"memory":"2Gi"}}`)
	if err != nil {
		t.Fatalf("ParseGatewayResources: %v", err)
	}

	tests := []struct {
		name      string
		resources *corev1.ResourceRequirements
		want      map[string]interface{}
	}{
		{
			name: "defaults when unset",
			want: map[string]interface{}{
				"requests": map[string]interface{}{"cpu": "100m", "memory": "512Mi"},
				"limits":   map[string]interface{}{"cpu": "500m", "memory": "1Gi"},
			},
		},
		{
			name:      "override replaces defaults",
			resources: &override,
			want: map[string]interface{}{
				"limits": map[string]interface{}{"memory": "2Gi"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			builder := ValuesBuilder{
				Gateway:   GatewayConfig{Image: "quay.io/test/gateway:latest"},
				Namespace: "test-ns",
				Resources: tc.resources,
			}

			values, err := builder.Build()
			if err != nil {
				t.Fatalf("Build() returned error: %v", err)
			}
			if !reflect.DeepEqual(values["resources"], tc.want) {
				t.Errorf("resources = %#v, want %#v", values["resources"], tc.want)
			}
		})
	}
}

func TestParseGatewayResources(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "requests and limits", raw: `{"requests":{"cpu":"100m","memory":"512Mi"},"limits":{"cpu":"500m","memory":"1Gi"}}`},
		{name: "memory limit only", raw: `{"limits":{"memory":"1Gi"}}`},
		{name: "invalid JSON", raw: `{"limits":`, wantErr: "parse resources JSON"},
		{name: "unknown field", raw: `{"limit":{"memory":"1Gi"}}`, wantErr: "unknown field"},
		{name: "invalid quantity", raw: `{"limits":{"memory":"lots"}}`, wantErr: "parse resources JSON"},
		{name: "trailing data", raw: `{"limits":{"memory":"1Gi"}} {}`, wantErr: "unexpected data"},
		{name: "empty object", raw: `{}`, wantErr: "limits.memory is required"},
		{name: "no memory limit", raw: `{"limits":{"cpu":"1"}}`, wantErr: "limits.memory is required"},
		{name: "request exceeds limit", raw: `{"requests":{"memory":"2Gi"},"limits":{"memory":"1Gi"}}`, wantErr: "exceeds limits.memory"},
		{name: "zero limit", raw: `{"limits":{"memory":"0"}}`, wantErr: "must be positive"},
		{name: "negative request", raw: `{"requests":{"cpu":"-1"},"limits":{"memory":"1Gi"}}`, wantErr: "must not be negative"},
		{name: "claims", raw: `{"limits":{"memory":"1Gi"},"claims":[{"name":"gpu"}]}`, wantErr: "claims are not supported"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseGatewayResources(tc.raw)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}
