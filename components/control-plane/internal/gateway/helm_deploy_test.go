package gateway

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
)

// TestDeployGatewayViaHelm_AdditionalCASignatureThreading verifies the
// materialization-presence signal reaches the values passed to Helm. The
// annotation checksum drives the workload roll when the bundle changes.
func TestDeployGatewayViaHelm_AdditionalCASignatureThreading(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "values.yaml")
	helmScript := filepath.Join(t.TempDir(), "helm")
	if err := os.WriteFile(helmScript, []byte(`#!/bin/sh
case "$1" in
status)
  echo "release not found" >&2
  exit 1
  ;;
install)
  shift
  while [ "$#" -gt 0 ]; do
    if [ "$1" = "--values" ]; then
      cp "$2" "$VALUES_CAPTURE"
      exit 0
    fi
    shift
  done
  exit 1
  ;;
esac
`), 0o700); err != nil {
		t.Fatalf("write fake helm: %v", err)
	}
	t.Setenv("VALUES_CAPTURE", capture)

	if err := deployGatewayViaHelm(
		context.Background(),
		&helm.ShellClient{HelmBinary: helmScript},
		NamespaceConfig{Name: "openshell-0123456789abcdef", Gateway: GatewayConfig{WorkspaceMode: "managed"}},
		ReconcileOpts{},
		false,
		false,
		"openshell-destination-ca-test",
	); err != nil {
		t.Fatalf("deployGatewayViaHelm() error = %v", err)
	}

	values, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("read captured values: %v", err)
	}
	if !strings.Contains(string(values), "additionalCaConfigMapName: openshell-destination-ca-test") {
		t.Errorf("captured Helm values do not name the additional CA ConfigMap:\n%s", values)
	}
}

// TestEffectiveCredentialDriver locks in the HYPERSHELL-369 flat default: an omitted
// (or empty-type) credential_driver resolves to vault; an explicit driver is never changed.
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
}

// TestVaultConventionContract pins the cross-repo Vault contract (review finding:
// unenforced literals). These values MUST match the hypershell-gitops GitOps
// Contract in specs/platform/openshell-gateway-credentials.spec.md; changing one is
// a deliberate, reviewed cross-repo change, so this test makes a silent edit fail.
// Verifies: OSG-CRED-03.01
func TestVaultConventionContract(t *testing.T) {
	want := map[string]string{
		"address":             "https://openshell-gateway-vault.openshell-vault.svc:8200",
		"mountPrefix":         "openshell-gw/",
		"kvVersion":           "2",
		"authMethod":          "kubernetes",
		"role":                "openshell-gateway",
		"provisionerRole":     "openshell-gateway-provisioner",
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
		"mountPrefix":         VaultTenantMountPrefix,
		"kvVersion":           VaultDefaultKVVersion,
		"authMethod":          VaultDefaultAuthMethod,
		"role":                VaultDefaultRole,
		"provisionerRole":     VaultProvisionerRole,
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
