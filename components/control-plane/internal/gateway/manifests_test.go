package gateway

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestApplyManifestToNamespace_ReplacesControlPlaneNamespace(t *testing.T) {
	manifest := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "NetworkPolicy",
		"metadata": map[string]interface{}{
			"name":      "gateway-health",
			"namespace": "NAMESPACE_PLACEHOLDER",
			"annotations": map[string]interface{}{
				"control-plane-namespace": "CONTROL_PLANE_NAMESPACE_PLACEHOLDER",
			},
		},
	}}

	result, err := ApplyManifestToNamespace(manifest, "tenant-ns", "control-plane-ns", GatewayConfig{}, StaticImageDefaults{})
	if err != nil {
		t.Fatalf("ApplyManifestToNamespace() error = %v", err)
	}
	if result.GetNamespace() != "tenant-ns" {
		t.Fatalf("namespace = %q, want %q", result.GetNamespace(), "tenant-ns")
	}
	value, found, err := unstructured.NestedString(result.Object, "metadata", "annotations", "control-plane-namespace")
	if err != nil || !found {
		t.Fatalf("control plane namespace annotation not found: found=%v error=%v", found, err)
	}
	if value != "control-plane-ns" {
		t.Fatalf("control plane namespace = %q, want %q", value, "control-plane-ns")
	}
}

func TestApplyManifestToNamespace_RequiresControlPlaneNamespace(t *testing.T) {
	manifest := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "NetworkPolicy",
		"metadata": map[string]interface{}{
			"name": "gateway-health",
			"annotations": map[string]interface{}{
				"control-plane-namespace": "CONTROL_PLANE_NAMESPACE_PLACEHOLDER",
			},
		},
	}}

	_, err := ApplyManifestToNamespace(manifest, "tenant-ns", "", GatewayConfig{}, StaticImageDefaults{})
	if err == nil || !strings.Contains(err.Error(), "control plane namespace is required") {
		t.Fatalf("ApplyManifestToNamespace() error = %v, want required namespace error", err)
	}
}

func TestApplyCredentialDriverToml_KubernetesSecrets(t *testing.T) {
	lines := []string{
		"[openshell.gateway]",
		"    listen = \"0.0.0.0:8443\"",
		"",
		"    [openshell.gateway.credential_storage]",
		"    key_encryption_key = \"${OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY}\"",
		"",
		"[openshell.gateway.sessions]",
	}

	driver := &CredentialDriverConfig{
		Type:              "kubernetes-secrets",
		KubernetesSecrets: &KubernetesSecretsConfig{Namespace: "cred-ns"},
	}

	result := applyCredentialDriverToml(lines, driver, "tenant-ns")
	joined := strings.Join(result, "\n")

	if strings.Contains(joined, "[openshell.gateway.credential_storage]") {
		t.Error("expected credential_storage section to be removed")
	}
	if strings.Contains(joined, "key_encryption_key") {
		t.Error("expected key_encryption_key line to be removed")
	}
	if !strings.Contains(joined, `credential_drivers = ["kubernetes-secrets"]`) {
		t.Error("expected kubernetes-secrets driver declaration")
	}
	if !strings.Contains(joined, `[openshell.credential_drivers.kubernetes-secrets]`) {
		t.Error("expected kubernetes-secrets driver section")
	}
	if !strings.Contains(joined, `namespace = "cred-ns"`) {
		t.Error("expected namespace to be set")
	}
	if !strings.Contains(joined, "[openshell.gateway.sessions]") {
		t.Error("expected subsequent sections to be preserved")
	}
}

func TestApplyCredentialDriverToml_Vault(t *testing.T) {
	lines := []string{
		"[openshell.gateway]",
		"    listen = \"0.0.0.0:8443\"",
		"",
		"    [openshell.gateway.credential_storage]",
		"    key_encryption_key = \"${OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY}\"",
	}

	driver := &CredentialDriverConfig{
		Type: "vault",
		Vault: &VaultCredentialConfig{
			Address: "https://vault.example.com",
			Role:    "gw-role",
		},
	}

	result := applyCredentialDriverToml(lines, driver, "tenant-ns")
	joined := strings.Join(result, "\n")

	if strings.Contains(joined, "[openshell.gateway.credential_storage]") {
		t.Error("expected credential_storage section to be removed")
	}
	if !strings.Contains(joined, `credential_drivers = ["vault"]`) {
		t.Error("expected vault driver declaration")
	}
	if !strings.Contains(joined, `[openshell.credential_drivers.vault]`) {
		t.Error("expected vault driver section")
	}
	if !strings.Contains(joined, `address = "https://vault.example.com"`) {
		t.Error("expected vault address")
	}
	if !strings.Contains(joined, `role = "gw-role"`) {
		t.Error("expected vault role")
	}
	if !strings.Contains(joined, `mount = "secret"`) {
		t.Error("expected default mount")
	}
	if !strings.Contains(joined, `service_account_token_path = "/var/run/secrets/vault/token"`) {
		t.Error("expected SA token path for kubernetes auth")
	}
}

func TestApplyCredentialDriverToml_NoCredentialStorageSection(t *testing.T) {
	lines := []string{
		"[openshell.gateway]",
		"    listen = \"0.0.0.0:8443\"",
	}

	driver := &CredentialDriverConfig{
		Type: "kubernetes-secrets",
	}

	result := applyCredentialDriverToml(lines, driver, "tenant-ns")
	joined := strings.Join(result, "\n")

	if !strings.Contains(joined, `credential_drivers = ["kubernetes-secrets"]`) {
		t.Error("expected kubernetes-secrets driver declaration even without existing credential_storage section")
	}
	if !strings.Contains(joined, `namespace = "tenant-ns"`) {
		t.Error("expected tenant namespace to be used as default")
	}
	if !strings.Contains(joined, "[openshell.gateway]") {
		t.Error("expected original content preserved")
	}
}

func TestApplyCredentialDriverDeploymentOverrides_RemovesKEKEnvVar(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "openshell-gateway"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "openshell-gateway",
								"env": []interface{}{
									map[string]interface{}{"name": "SOME_VAR", "value": "keep"},
									map[string]interface{}{"name": "OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY", "value": "remove-me"},
									map[string]interface{}{"name": "OTHER_VAR", "value": "also-keep"},
								},
							},
						},
					},
				},
			},
		},
	}

	driver := &CredentialDriverConfig{Type: "kubernetes-secrets"}
	if err := applyCredentialDriverDeploymentOverrides(obj, driver); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	container := containers[0].(map[string]interface{})
	envList, _, _ := unstructured.NestedSlice(container, "env")

	for _, e := range envList {
		em := e.(map[string]interface{})
		if em["name"] == "OPENSHELL_GATEWAY_CREDENTIAL_KEY_ENCRYPTION_KEY" {
			t.Error("expected KEK env var to be removed")
		}
	}
	if len(envList) != 2 {
		t.Errorf("expected 2 env vars remaining, got %d", len(envList))
	}
}

func TestApplyCredentialDriverDeploymentOverrides_VaultAddsVolume(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   map[string]interface{}{"name": "openshell-gateway"},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name": "openshell-gateway",
								"env":  []interface{}{},
							},
						},
						"volumes": []interface{}{},
					},
				},
			},
		},
	}

	driver := &CredentialDriverConfig{
		Type: "vault",
		Vault: &VaultCredentialConfig{
			Address: "https://vault.example.com",
			Role:    "gw-role",
		},
	}
	if err := applyCredentialDriverDeploymentOverrides(obj, driver); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	container := containers[0].(map[string]interface{})
	mounts, _, _ := unstructured.NestedSlice(container, "volumeMounts")
	if len(mounts) != 1 {
		t.Fatalf("expected 1 volume mount, got %d", len(mounts))
	}
	mount := mounts[0].(map[string]interface{})
	if mount["name"] != "vault-sa-token" {
		t.Errorf("expected mount name vault-sa-token, got %v", mount["name"])
	}
	if mount["mountPath"] != "/var/run/secrets/vault" {
		t.Errorf("expected mountPath /var/run/secrets/vault, got %v", mount["mountPath"])
	}

	volumes, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "volumes")
	if len(volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(volumes))
	}
	vol := volumes[0].(map[string]interface{})
	if vol["name"] != "vault-sa-token" {
		t.Errorf("expected volume name vault-sa-token, got %v", vol["name"])
	}
}
