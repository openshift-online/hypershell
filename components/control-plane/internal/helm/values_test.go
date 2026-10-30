package helm

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

// TestBuild_SandboxRuntimeImage locks in that the sandbox runtime image is
// pinned alongside the supervisor. The two speak a versioned boundary protocol;
// leaving the runtime to the chart default (a moving upstream tag) skews it from
// the pinned supervisor and sandboxes never leave Provisioning.
func TestBuild_SandboxRuntimeImage(t *testing.T) {
	builder := ValuesBuilder{
		Gateway: GatewayConfig{
			Image:               "quay.io/test/gateway:v1",
			SupervisorImage:     "quay.io/test/supervisor:v1",
			SandboxRuntimeImage: "quay.io/test/sandbox:v1",
		},
		Namespace: "test-ns",
	}
	values, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	want := map[string]interface{}{"registry": "quay.io", "repository": "test/sandbox", "tag": "v1"}
	runtime, _ := values["sandboxRuntime"].(map[string]interface{})
	if got := runtime["image"]; !reflect.DeepEqual(got, want) {
		t.Errorf("sandboxRuntime.image = %#v, want %#v", got, want)
	}

	builder.Gateway.SandboxRuntimeImage = ""
	values, err = builder.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if _, ok := values["sandboxRuntime"]; ok {
		t.Errorf("sandboxRuntime set without an image: %#v", values["sandboxRuntime"])
	}
}

// TestBuild_SandboxRuntimeClass verifies that a non-empty SandboxRuntimeClass
// sets server.defaultRuntimeClassName in the built values and that an empty
// value omits the key entirely (so the chart omits runtimeClassName from sandbox pods).
func TestBuild_SandboxRuntimeClass(t *testing.T) {
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

	builder := ValuesBuilder{
		Gateway:             GatewayConfig{Image: "quay.io/test/gateway:v1"},
		Namespace:           "test-ns",
		SandboxRuntimeClass: "kata",
	}
	values, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if got := nested(values, "server", "defaultRuntimeClassName"); got != "kata" {
		t.Errorf("server.defaultRuntimeClassName = %v, want %q", got, "kata")
	}

	builder.SandboxRuntimeClass = ""
	values, err = builder.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if got := nested(values, "server", "defaultRuntimeClassName"); got != nil {
		t.Errorf("server.defaultRuntimeClassName present when SandboxRuntimeClass is empty: %v", got)
	}
}

// TestBuild_SandboxRuntimeClassRendersInChart guards the value path end-to-end:
// renders the vendored chart with SandboxRuntimeClass set and asserts
// default_runtime_class_name reaches the gateway config. Skipped without helm.
func TestBuild_SandboxRuntimeClassRendersInChart(t *testing.T) {
	helmBin, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm binary not found on PATH")
	}
	chart, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "charts", "openshell"))
	if err != nil {
		t.Fatalf("resolve chart path: %v", err)
	}

	builder := ValuesBuilder{
		Gateway: GatewayConfig{
			Image:           "quay.io/test/gateway:v1",
			SupervisorImage: "quay.io/test/supervisor:v1",
		},
		Namespace:           "test-ns",
		SandboxRuntimeClass: "kata",
	}
	values, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	raw, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("marshal values: %v", err)
	}
	valuesFile := filepath.Join(t.TempDir(), "values.json")
	if err := os.WriteFile(valuesFile, raw, 0o600); err != nil {
		t.Fatalf("write values: %v", err)
	}

	out, err := exec.Command(helmBin, "template", "t", chart, "-f", valuesFile,
		"--set", "agentSandbox.preflight.enabled=false").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	want := `default_runtime_class_name = "kata"`
	if !strings.Contains(string(out), want) {
		t.Errorf("rendered chart does not contain %q", want)
	}
}

// TestBuild_SandboxRuntimeImageRendersInChart guards the value path itself: a
// key the chart does not read silently no-ops, which leaves the runtime on the
// moving chart default. It renders the vendored chart with the builder's values
// and asserts the pin reaches the gateway config. Skipped without a helm binary.
func TestBuild_SandboxRuntimeImageRendersInChart(t *testing.T) {
	helmBin, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm binary not found on PATH")
	}
	chart, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "charts", "openshell"))
	if err != nil {
		t.Fatalf("resolve chart path: %v", err)
	}

	builder := ValuesBuilder{
		Gateway: GatewayConfig{
			Image:               "quay.io/test/gateway:v1",
			SupervisorImage:     "quay.io/test/supervisor:v1",
			SandboxRuntimeImage: "quay.io/test/sandbox:v1",
		},
		Namespace: "test-ns",
	}
	values, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	raw, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("marshal values: %v", err)
	}
	valuesFile := filepath.Join(t.TempDir(), "values.json")
	if err := os.WriteFile(valuesFile, raw, 0o600); err != nil {
		t.Fatalf("write values: %v", err)
	}

	// The Agent Sandbox preflight needs a live cluster; disable it for offline rendering.
	out, err := exec.Command(helmBin, "template", "t", chart, "-f", valuesFile,
		"--set", "agentSandbox.preflight.enabled=false").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	want := `sandbox_runtime_image = "quay.io/test/sandbox:v1"`
	if !strings.Contains(string(out), want) {
		t.Errorf("rendered chart does not carry the sandbox runtime pin %q", want)
	}
}

// TestBuild_VaultCredentialDriver locks in the HYPERSHELL-369 Vault driver wiring:
// the resolved coordinates land under server.credentialDrivers.vault.*, the CA
// ConfigMap is referenced only when HasVaultCA, and a projected ServiceAccount
// token scoped to the `vault` audience is injected via the extraVolumes hooks.
// Verifies: OSG-CRED-06.01
func TestBuild_VaultCredentialDriver(t *testing.T) {
	newVault := func() *VaultValues {
		return &VaultValues{
			Address:                 "https://openshell-gateway-vault.openshell-vault.svc:8200",
			Mount:                   "secret",
			KVVersion:               "2",
			AuthMethod:              "kubernetes",
			Role:                    "openshell-gateway",
			KubernetesAuthMount:     "kubernetes",
			ServiceAccountTokenPath: "/var/run/secrets/vault/token",
			TokenAudience:           "vault",
			TokenDir:                "/var/run/secrets/vault",
			TokenExpirySeconds:      3600,
			CAConfigMapName:         "openshell-gateway-vault-ca",
			TimeoutSecs:             30,
		}
	}

	for _, hasCA := range []bool{true, false} {
		builder := ValuesBuilder{
			Gateway: GatewayConfig{
				Image:            "quay.io/test/gateway:latest",
				CredentialDriver: &CredentialDriverConfig{Type: "vault", Vault: newVault()},
			},
			Namespace:  "test-ns",
			HasVaultCA: hasCA,
		}

		values, err := builder.Build()
		if err != nil {
			t.Fatalf("Build() returned error: %v", err)
		}

		server, _ := values["server"].(map[string]interface{})
		cd, _ := server["credentialDrivers"].(map[string]interface{})
		vault, _ := cd["vault"].(map[string]interface{})
		if vault == nil {
			t.Fatalf("hasCA=%v: server.credentialDrivers.vault not set", hasCA)
		}

		want := map[string]interface{}{
			"enabled":                 true,
			"address":                 "https://openshell-gateway-vault.openshell-vault.svc:8200",
			"mount":                   "secret",
			"kvVersion":               "2",
			"authMethod":              "kubernetes",
			"role":                    "openshell-gateway",
			"kubernetesAuthMount":     "kubernetes",
			"serviceAccountTokenPath": "/var/run/secrets/vault/token",
		}
		for k, v := range want {
			if vault[k] != v {
				t.Errorf("hasCA=%v: vault[%q] = %v, want %v", hasCA, k, vault[k], v)
			}
		}

		if hasCA {
			if vault["caConfigMapName"] != "openshell-gateway-vault-ca" {
				t.Errorf("hasCA=true: caConfigMapName = %v, want openshell-gateway-vault-ca", vault["caConfigMapName"])
			}
		} else if vault["caConfigMapName"] != nil {
			t.Errorf("hasCA=false: caConfigMapName should be absent, got %v", vault["caConfigMapName"])
		}

		// Projected vault-audience token via extraVolumes.
		extraVolumes, _ := server["extraVolumes"].([]interface{})
		if len(extraVolumes) != 1 {
			t.Fatalf("hasCA=%v: expected 1 extraVolume, got %d", hasCA, len(extraVolumes))
		}
		vol, _ := extraVolumes[0].(map[string]interface{})
		if vol["name"] != "vault-sa-token" {
			t.Errorf("extraVolume name = %v, want vault-sa-token", vol["name"])
		}
		projected, _ := vol["projected"].(map[string]interface{})
		sources, _ := projected["sources"].([]interface{})
		if len(sources) != 1 {
			t.Fatalf("expected 1 projected source, got %d", len(sources))
		}
		src, _ := sources[0].(map[string]interface{})
		sat, _ := src["serviceAccountToken"].(map[string]interface{})
		if sat["audience"] != "vault" {
			t.Errorf("projected token audience = %v, want vault", sat["audience"])
		}
		if sat["path"] != "token" {
			t.Errorf("projected token path = %v, want token", sat["path"])
		}

		mounts, _ := server["extraVolumeMounts"].([]interface{})
		if len(mounts) != 1 {
			t.Fatalf("expected 1 extraVolumeMount, got %d", len(mounts))
		}
		mount, _ := mounts[0].(map[string]interface{})
		if mount["mountPath"] != "/var/run/secrets/vault" || mount["name"] != "vault-sa-token" {
			t.Errorf("extraVolumeMount = %v, want name=vault-sa-token mountPath=/var/run/secrets/vault", mount)
		}
	}
}

// TestBuild_KekDbCredentialDriver verifies the explicit kek-db driver leaves both
// external drivers disabled (so the chart provisions the KEK store) and injects no
// vault token volume.
func TestBuild_KekDbCredentialDriver(t *testing.T) {
	builder := ValuesBuilder{
		Gateway: GatewayConfig{
			Image:            "quay.io/test/gateway:latest",
			CredentialDriver: &CredentialDriverConfig{Type: "kek-db"},
		},
		Namespace: "test-ns",
	}
	values, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	server, _ := values["server"].(map[string]interface{})
	cd, _ := server["credentialDrivers"].(map[string]interface{})
	vault, _ := cd["vault"].(map[string]interface{})
	if vault != nil && vault["enabled"] == true {
		t.Errorf("kek-db: vault should not be enabled")
	}
	ks, _ := cd["kubernetesSecrets"].(map[string]interface{})
	if ks == nil || ks["enabled"] != false {
		t.Errorf("kek-db: kubernetesSecrets.enabled should be false, got %v", ks)
	}
	if server["extraVolumes"] != nil {
		t.Errorf("kek-db: no extraVolumes expected, got %v", server["extraVolumes"])
	}
}

// TestBuild_WorkspaceMode locks in the workspace-mode wiring required by
// openshell-gateway.spec.md "Gateway Workspace Mode": an empty mode (legacy
// gateways) omits the Helm value so the chart default (shared) keeps applying;
// a set mode lands under server.drivers.kubernetes.workspaceMode; and managed
// mode also derives a per-gateway unique gateway_id from the API-assigned
// namespace so per-workspace namespaces never collide across gateways.
func TestBuild_WorkspaceMode(t *testing.T) {
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

	// Empty mode (legacy gateway): nothing is set, chart default applies.
	builder := ValuesBuilder{
		Gateway:   GatewayConfig{Image: "quay.io/test/gateway:v1"},
		Namespace: "openshell-deadbeefdeadbeef",
	}
	values, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if got := nested(values, "server", "drivers", "kubernetes", "workspaceMode"); got != nil {
		t.Errorf("server.drivers.kubernetes.workspaceMode present for empty mode: %v", got)
	}
	if got := nested(values, "server", "sandboxJwt", "gatewayId"); got != nil {
		t.Errorf("server.sandboxJwt.gatewayId present for empty mode: %v", got)
	}

	// Managed mode: mode and the derived per-gateway gateway_id are both set.
	builder.Gateway.WorkspaceMode = "managed"
	values, err = builder.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if got := nested(values, "server", "drivers", "kubernetes", "workspaceMode"); got != "managed" {
		t.Errorf("server.drivers.kubernetes.workspaceMode = %v, want %q", got, "managed")
	}
	if got := nested(values, "server", "sandboxJwt", "gatewayId"); got != "deadbeefdeadbeef" {
		t.Errorf("server.sandboxJwt.gatewayId = %v, want %q", got, "deadbeefdeadbeef")
	}

	// Explicit shared mode: the mode is passed but no custom gateway_id is
	// derived (shared sandboxes live in the gateway namespace).
	builder.Gateway.WorkspaceMode = "shared"
	values, err = builder.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	if got := nested(values, "server", "drivers", "kubernetes", "workspaceMode"); got != "shared" {
		t.Errorf("server.drivers.kubernetes.workspaceMode = %v, want %q", got, "shared")
	}
	if got := nested(values, "server", "sandboxJwt", "gatewayId"); got != nil {
		t.Errorf("server.sandboxJwt.gatewayId set for shared mode: %v", got)
	}

	// Managed mode requires an API-assigned namespace shape; a foreign
	// namespace cannot produce a unique gateway_id and must fail loudly
	// rather than collide per-workspace namespaces across gateways.
	builder.Gateway.WorkspaceMode = "managed"
	builder.Namespace = "custom-namespace"
	if _, err := builder.Build(); err == nil {
		t.Errorf("Build() succeeded for managed mode with non API-assigned namespace, want error")
	}
}

// TestBuild_WorkspaceModeRendersInChart guards the value path end-to-end: the
// control-plane-built values must reach the chart's rendered gateway.toml as
// workspace_mode = "managed" and the derived gateway_id must land in both the
// gateway JWT identity and the Kubernetes driver section. Skipped without helm.
func TestBuild_WorkspaceModeRendersInChart(t *testing.T) {
	helmBin, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm binary not found on PATH")
	}
	chart, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "charts", "openshell"))
	if err != nil {
		t.Fatalf("resolve chart path: %v", err)
	}

	builder := ValuesBuilder{
		Gateway: GatewayConfig{
			Image:           "quay.io/test/gateway:v1",
			SupervisorImage: "quay.io/test/supervisor:v1",
			WorkspaceMode:   "managed",
		},
		Namespace: "openshell-deadbeefdeadbeef",
	}
	values, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() returned error: %v", err)
	}
	raw, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("marshal values: %v", err)
	}
	valuesFile := filepath.Join(t.TempDir(), "values.json")
	if err := os.WriteFile(valuesFile, raw, 0o600); err != nil {
		t.Fatalf("write values: %v", err)
	}

	out, err := exec.Command(helmBin, "template", "t", chart, "-f", valuesFile,
		"--set", "agentSandbox.preflight.enabled=false").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	rendered := string(out)
	for _, want := range []string{
		`workspace_mode = "managed"`,
		`gateway_id = "deadbeefdeadbeef"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered chart does not contain %q", want)
		}
	}
}

// TestBuild_ServerDnsNames guards that the gateway's injected ServerDnsNames
// (which include the gateway's derived external hostname) reach BOTH chart
// keys that can feed certificate SANs: pkiInitJob.serverDnsNames (certgen hook,
// cert-manager disabled) and certManager.serverDnsNames (server_sans in
// gateway.toml and the internal server Certificate, cert-manager enabled).
// The regression it guards: with cert-manager enabled the chart ignores
// pkiInitJob.serverDnsNames entirely, so the external hostname never reached
// server_sans or the Certificate SANs (docs/investigations/per-gateway-certificates.md,
// Gap C). It also locks in that buildIngressValues keeps winning in Route
// mode with an external issuer: certManager.serverDnsNames must stay [hostname]
// there, not the core-values list.
func TestBuild_ServerDnsNames(t *testing.T) {
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

	dnsNames := []string{"gw-test-ns.apps.example.com", "internal.example.com"}

	tests := []struct {
		name                    string
		hasCertManager          bool
		routeEnabled            bool
		useOpenShiftRoute       bool
		externalCAIssuer        string
		wantPkiInitNames        bool
		wantCertMgrNames        bool
		wantCertMgrHostnameOnly bool
	}{
		{
			name:             "cert-manager enabled writes both pkiInitJob and certManager serverDnsNames",
			hasCertManager:   true,
			wantPkiInitNames: true,
			wantCertMgrNames: true,
		},
		{
			name:             "cert-manager disabled writes only pkiInitJob serverDnsNames",
			hasCertManager:   false,
			wantPkiInitNames: true,
			wantCertMgrNames: false,
		},
		{
			name:                    "route mode with external issuer keeps certManager serverDnsNames as hostname only",
			hasCertManager:          true,
			routeEnabled:            true,
			useOpenShiftRoute:       true,
			externalCAIssuer:        "gateway-ca",
			wantPkiInitNames:        true,
			wantCertMgrHostnameOnly: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			routeHost := "gw-test-ns.apps.example.com"
			builder := ValuesBuilder{
				Gateway: GatewayConfig{
					Image:          "quay.io/test/gateway:latest",
					ServerDnsNames: dnsNames,
					Route:          RouteConfig{Host: routeHost, Enabled: tc.routeEnabled},
				},
				Namespace:            "test-ns",
				HasCertManager:       tc.hasCertManager,
				IngressBaseDomain:    "apps.example.com",
				UseOpenShiftRoute:    tc.useOpenShiftRoute,
				ExternalCAIssuerName: tc.externalCAIssuer,
				ExternalCAIssuerKind: "ClusterIssuer",
			}

			values, err := builder.Build()
			if err != nil {
				t.Fatalf("Build() returned error: %v", err)
			}

			pkiNames := nested(values, "pkiInitJob", "serverDnsNames")
			if tc.wantPkiInitNames {
				if !reflect.DeepEqual(pkiNames, dnsNames) {
					t.Errorf("pkiInitJob.serverDnsNames = %v, want %v", pkiNames, dnsNames)
				}
			} else if pkiNames != nil {
				t.Errorf("pkiInitJob.serverDnsNames should be absent, got %v", pkiNames)
			}

			certMgrNames := nested(values, "certManager", "serverDnsNames")
			switch {
			case tc.wantCertMgrHostnameOnly:
				want := []string{routeHost}
				if !reflect.DeepEqual(certMgrNames, want) {
					t.Errorf("certManager.serverDnsNames = %v, want %v (ingress override must win)", certMgrNames, want)
				}
			case tc.wantCertMgrNames:
				if !reflect.DeepEqual(certMgrNames, dnsNames) {
					t.Errorf("certManager.serverDnsNames = %v, want %v", certMgrNames, dnsNames)
				}
			default:
				if certMgrNames != nil {
					t.Errorf("certManager.serverDnsNames should be absent, got %v", certMgrNames)
				}
			}
		})
	}
}
