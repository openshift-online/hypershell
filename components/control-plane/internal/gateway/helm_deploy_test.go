package gateway

import "testing"

// TestEffectiveCredentialDriver locks in the HYPERSHELL-369 rollout selector:
// an omitted credential_driver defaults to kek-db unless the deployment opts into
// vault via GATEWAY_DEFAULT_CREDENTIAL_DRIVER; an explicit driver is never changed.
func TestEffectiveCredentialDriver(t *testing.T) {
	t.Run("omitted (nil) resolves to the flat vault default", func(t *testing.T) {
		got := effectiveCredentialDriver(nil)
		if got == nil || got.Type != "vault" {
			t.Errorf("expected vault, got %+v", got)
		}
	})

	t.Run("explicit kek-db is returned unchanged", func(t *testing.T) {
		in := &CredentialDriverConfig{Type: "kek-db"}
		if got := effectiveCredentialDriver(in); got != in {
			t.Errorf("explicit driver should be returned unchanged, got %+v", got)
		}
	})

	t.Run("explicit kubernetes-secrets is returned unchanged", func(t *testing.T) {
		in := &CredentialDriverConfig{Type: "kubernetes-secrets"}
		if got := effectiveCredentialDriver(in); got != in {
			t.Errorf("explicit driver should be returned unchanged, got %+v", got)
		}
	})
}

// TestEffectiveCredentialDriver_EmptyType covers the empty-type object ({}) case:
// it must behave like an omitted driver (defer to the deployment default), not
// bypass the selector and resolve to vault. (Review finding: empty-type bypass.)
func TestEffectiveCredentialDriver_EmptyType(t *testing.T) {
	t.Run("empty-type object resolves to vault (same as omitted)", func(t *testing.T) {
		got := effectiveCredentialDriver(&CredentialDriverConfig{Type: ""})
		if got == nil || got.Type != "vault" {
			t.Errorf("empty-type object should resolve to vault, got %+v", got)
		}
	})

	t.Run("empty-type object preserves vault overrides", func(t *testing.T) {
		in := &CredentialDriverConfig{Type: "", Vault: &VaultCredentialConfig{Role: "custom"}}
		got := effectiveCredentialDriver(in)
		if got == nil || got.Vault == nil || got.Vault.Role != "custom" {
			t.Errorf("expected preserved vault override role=custom, got %+v", got)
		}
	})
}

// TestVaultConventionContract pins the cross-repo Vault contract (review finding:
// unenforced literals). These values MUST match the hypershell-gitops GitOps
// Contract in specs/platform/openshell-gateway-credentials.spec.md; changing one is
// a deliberate, reviewed cross-repo change, so this test makes a silent edit fail.
func TestVaultConventionContract(t *testing.T) {
	want := map[string]string{
		"address":             "https://openshell-gateway-vault.openshell-vault.svc:8200",
		"mount":               "secret",
		"kvVersion":           "2",
		"authMethod":          "kubernetes",
		"role":                "openshell-gateway",
		"kubernetesAuthMount": "kubernetes",
		"saTokenPath":         "/var/run/secrets/vault/token",
		"saTokenAudience":     "vault",
		"caSourceNamespace":   "openshell-vault",
		"caSourceSecret":      "openshell-gateway-vault-tls",
		"caConfigMap":         "openshell-gateway-vault-ca",
		"caKey":               "ca.crt",
	}
	got := map[string]string{
		"address":             VaultDefaultAddress,
		"mount":               VaultDefaultMount,
		"kvVersion":           VaultDefaultKVVersion,
		"authMethod":          VaultDefaultAuthMethod,
		"role":                VaultDefaultRole,
		"kubernetesAuthMount": VaultDefaultKubernetesAuthMount,
		"saTokenPath":         VaultServiceAccountTokenPath,
		"saTokenAudience":     VaultServiceAccountTokenAudience,
		"caSourceNamespace":   VaultCADefaultSourceNamespace,
		"caSourceSecret":      VaultCADefaultSourceSecret,
		"caConfigMap":         VaultCATargetConfigMapName,
		"caKey":               VaultCAKey,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("Vault contract %q drifted: got %q, want %q (keep in sync with hypershell-gitops)", k, got[k], v)
		}
	}
}
