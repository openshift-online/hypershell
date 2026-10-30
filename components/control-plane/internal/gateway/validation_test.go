package gateway

import (
	"encoding/json"
	"strings"
	"testing"
)

// Verifies: OSG-CORE-07.02
func TestValidateImageReference(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantErr bool
	}{
		{name: "empty is invalid", ref: "", wantErr: true},
		{name: "bare name with tag", ref: "postgres:18", wantErr: false},
		{name: "docker hub library path", ref: "docker.io/library/postgres:18", wantErr: false},
		{name: "ghcr multi-segment path with tag", ref: "ghcr.io/nvidia/openshell/gateway:0.0.101", wantErr: false},
		{name: "multi-segment path with tag and reference", ref: "quay.io/opendatahub/odh-openshell-gateway:v0.0.109-rhaiv.0@sha256:a80b79e514826e8d57ea137749cf18a6e7f3d92e26bfefe005f3a9c4a55b8bdd", wantErr: false},
		{name: "quay long path with tag", ref: "quay.io/redhat-services-prod/hcm-eng-prod-tenant/hypershell-api-server-main:dev", wantErr: false},
		{name: "digest reference", ref: "registry.redhat.io/rhel9/postgresql-16@sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", wantErr: false},
		// In-cluster registry service address carries an explicit port; this is
		// the address the kubelet pulls mirrored images by on ROKS.
		{name: "internal registry host with port", ref: "image-registry.openshift-image-registry.svc:5000/openshift/openshell-gateway:0.0.101", wantErr: false},
		{name: "internal registry host with port, no tag", ref: "image-registry.openshift-image-registry.svc:5000/openshift/postgres", wantErr: false},
		{name: "localhost registry with port", ref: "localhost:5000/hypershell-controller:dev", wantErr: false},
		{name: "shell metacharacter rejected", ref: "postgres:18;rm -rf /", wantErr: true},
		{name: "command substitution rejected", ref: "postgres:$(whoami)", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateImageReference(tt.ref)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateImageReference(%q) error = %v, wantErr %v", tt.ref, err, tt.wantErr)
			}
		})
	}
}

// Verifies: OSG-ROUT-03.02
func TestValidateGatewayConfigRouteHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{name: "empty host is valid (derived from base domain)", host: "", wantErr: false},
		{name: "well-formed host is valid", host: "gw-tenant-a.apps.example.com", wantErr: false},
		{name: "uppercase is invalid (not a DNS label)", host: "GW-Tenant.example.com", wantErr: true},
		{name: "leading dot is invalid", host: ".example.com", wantErr: true},
		{name: "whitespace is invalid", host: "gw tenant.example.com", wantErr: true},
		{name: "trailing newline is invalid", host: "gw-tenant.example.com\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GatewayConfig{Route: RouteConfig{Host: tt.host}}
			err := ValidateGatewayConfig(cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateGatewayConfig(host=%q) error = %v, wantErr %v", tt.host, err, tt.wantErr)
			}
		})
	}
}

func TestValidateOIDCConfig(t *testing.T) {
	tests := []struct {
		name    string
		oidc    OIDCConfig
		wantErr bool
	}{
		{name: "no issuer skips validation", oidc: OIDCConfig{}, wantErr: false},
		{name: "issuer without roles is valid", oidc: OIDCConfig{Issuer: "https://kc/realm"}, wantErr: false},
		{
			// A BYO config that sets roles but omits roles_claim delegates to the
			// gateway's own default (groups); validation must not reject it on the
			// reconcile path, or pre-existing gateways would fail to reconcile.
			name:    "roles mapped without roles_claim delegates to gateway default",
			oidc:    OIDCConfig{Issuer: "https://kc/realm", AdminRole: "admin", UserRole: "user"},
			wantErr: false,
		},
		{
			name:    "roles mapped with top-level roles_claim is valid",
			oidc:    OIDCConfig{Issuer: "https://kc/realm", AdminRole: "admin", UserRole: "user", RolesClaim: "roles"},
			wantErr: false,
		},
		{
			name:    "roles mapped with nested roles_claim is valid",
			oidc:    OIDCConfig{Issuer: "https://kc/realm", AdminRole: "admin", UserRole: "user", RolesClaim: "realm_access.roles"},
			wantErr: false,
		},
		{
			name:    "managed model claim path is valid",
			oidc:    OIDCConfig{Issuer: "https://kc/realm", AdminRole: "openshell-admin", UserRole: "openshell-user", RolesClaim: "hypershell.roles"},
			wantErr: false,
		},
		{
			name:    "only admin_role set is rejected",
			oidc:    OIDCConfig{Issuer: "https://kc/realm", AdminRole: "admin", RolesClaim: "roles"},
			wantErr: true,
		},
		{
			name:    "malformed roles_claim with spaces is rejected",
			oidc:    OIDCConfig{Issuer: "https://kc/realm", RolesClaim: "realm access.roles"},
			wantErr: true,
		},
		{
			name:    "malformed roles_claim with leading dot is rejected",
			oidc:    OIDCConfig{Issuer: "https://kc/realm", RolesClaim: ".roles"},
			wantErr: true,
		},
		{
			name:    "roles_claim without role mapping still validates format",
			oidc:    OIDCConfig{Issuer: "https://kc/realm", RolesClaim: "groups"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateOIDCConfig(tt.oidc); (err != nil) != tt.wantErr {
				t.Fatalf("ValidateOIDCConfig(%+v) error = %v, wantErr %v", tt.oidc, err, tt.wantErr)
			}
		})
	}
}

// Verifies: OSG-CRED-02.01, OSG-CRED-02.02
func TestValidateCredentialDriverConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *CredentialDriverConfig
		wantErr bool
	}{
		{
			name:    "nil config is valid",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "kubernetes-secrets is valid",
			config:  &CredentialDriverConfig{Type: "kubernetes-secrets"},
			wantErr: false,
		},
		{
			name: "kubernetes-secrets with namespace is valid",
			config: &CredentialDriverConfig{
				Type:              "kubernetes-secrets",
				KubernetesSecrets: &KubernetesSecretsConfig{Namespace: "my-ns"},
			},
			wantErr: false,
		},
		{
			// HYPERSHELL-369: Vault is a black box - credential_driver carries NO per-gateway
			// Vault coordinates (no address/role/mount/auth), so `type: vault` alone is the
			// whole contract; the control plane fills everything from the per-cluster
			// conventions + deployment config.
			name:    "vault (type only) is valid; no overrides exist",
			config:  &CredentialDriverConfig{Type: "vault"},
			wantErr: false,
		},
		{
			name:    "kek-db is valid",
			config:  &CredentialDriverConfig{Type: "kek-db"},
			wantErr: false,
		},
		{
			name:    "unsupported type is invalid",
			config:  &CredentialDriverConfig{Type: "aws-secrets-manager"},
			wantErr: true,
		},
		{
			// An empty-typed driver object (credential_driver: {}) is unspecified and resolves
			// to the flat vault default, same as an omitted field (OSG-CRED-01); validation
			// accepts it here and effectiveCredentialDriver fills in vault at reconcile.
			name:    "empty type is valid (resolves to the vault default)",
			config:  &CredentialDriverConfig{Type: ""},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCredentialDriverConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCredentialDriverConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Verifies: OSG-CRED-02.03
//
// Vault is a black box: a credential_driver.vault object (or any Vault override key) has no
// field on CredentialDriverConfig, so it is rejected when the stored driver JSON is decoded
// with DisallowUnknownFields - the exact decoder the reconciler uses before reconciling. This
// asserts that contract on the same struct + decoder option the production path uses.
func TestCredentialDriverRejectsVaultOverride(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{"vault type only is accepted", `{"type":"vault"}`, false},
		{"vault.address override is rejected", `{"type":"vault","vault":{"address":"https://vault.example.com:8200"}}`, true},
		{"vault.role override is rejected", `{"type":"vault","vault":{"role":"openshell-gateway-tenant-a"}}`, true},
		{"bare vault object is rejected", `{"type":"vault","vault":{}}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec := json.NewDecoder(strings.NewReader(tc.json))
			dec.DisallowUnknownFields()
			var c CredentialDriverConfig
			err := dec.Decode(&c)
			if (err != nil) != tc.wantErr {
				t.Fatalf("decode(%s) err = %v; wantErr %v", tc.json, err, tc.wantErr)
			}
		})
	}
}

func TestValidateGatewayConfig_SandboxRuntimeImage(t *testing.T) {
	if err := ValidateGatewayConfig(GatewayConfig{SandboxRuntimeImage: "quay.io/test/sandbox:v1"}); err != nil {
		t.Fatalf("valid sandbox runtime image rejected: %v", err)
	}
	if err := ValidateGatewayConfig(GatewayConfig{SandboxRuntimeImage: "not a valid ref!"}); err == nil {
		t.Fatal("invalid sandbox runtime image accepted")
	}
}

// TestValidateGatewayConfigWorkspaceMode covers the reconcile-time vocabulary
// check for the OpenShell workspace mode (openshell-gateway.spec.md, "Gateway
// Workspace Mode"): the three chart-supported values pass, anything else fails,
// and an empty mode (legacy gateways) is accepted.
func TestValidateGatewayConfigWorkspaceMode(t *testing.T) {
	for _, mode := range []string{"shared", "managed", "operator"} {
		if err := ValidateGatewayConfig(GatewayConfig{WorkspaceMode: mode}); err != nil {
			t.Errorf("ValidateGatewayConfig(workspaceMode=%q) error = %v, want nil", mode, err)
		}
	}
	if err := ValidateGatewayConfig(GatewayConfig{}); err != nil {
		t.Errorf("ValidateGatewayConfig(empty workspaceMode) error = %v, want nil", err)
	}
	for _, mode := range []string{"combined", "Managed", ""} {
		if mode == "" {
			continue
		}
		err := ValidateGatewayConfig(GatewayConfig{WorkspaceMode: mode})
		if err == nil {
			t.Errorf("ValidateGatewayConfig(workspaceMode=%q) = nil, want error", mode)
		}
	}
}

// TestValidateGatewayConfigRouteCertificate guards the dedicated-certificate
// opt-in shape: a route.certificate gateway always uses the derived
// gw-<namespace> hostname, so pairing it with an explicit route.host is
// rejected rather than silently ignored.
func TestValidateGatewayConfigRouteCertificate(t *testing.T) {
	t.Run("certificate with an explicit host is rejected", func(t *testing.T) {
		cfg := GatewayConfig{Route: RouteConfig{
			Host:        "gw-tenant-a.apps.example.com",
			Certificate: &RouteCertificateConfig{},
		}}
		err := ValidateGatewayConfig(cfg)
		if err == nil {
			t.Fatal("expected route.host + route.certificate to be rejected, got nil")
		}
		if !strings.Contains(err.Error(), "route.host is not supported with route.certificate") {
			t.Errorf("error does not explain the dedicated-certificate hostname rule: %v", err)
		}
	})

	t.Run("certificate alone is valid", func(t *testing.T) {
		cfg := GatewayConfig{Route: RouteConfig{
			Enabled:     true,
			Certificate: &RouteCertificateConfig{},
		}}
		if err := ValidateGatewayConfig(cfg); err != nil {
			t.Fatalf("route.certificate alone must be accepted: %v", err)
		}
	})

	t.Run("explicit host without certificate keeps shared-Gateway semantics", func(t *testing.T) {
		cfg := GatewayConfig{Route: RouteConfig{Host: "gw-tenant-a.apps.example.com"}}
		if err := ValidateGatewayConfig(cfg); err != nil {
			t.Fatalf("route.host without route.certificate must stay valid: %v", err)
		}
	})
}
