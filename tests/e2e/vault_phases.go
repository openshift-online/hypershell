package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Per-cluster Vault serving-CA source the control plane reads for its reachability probe.
// These mirror the control-plane conventions (gateway.VaultCADefaultSourceNamespace /
// VaultCADefaultSourceSecret / VaultCAKey), duplicated here because that package is internal.
const (
	vaultCASourceNamespace = "openshell-vault"
	vaultCASourceSecret    = "openshell-gateway-vault-tls"
	vaultCAKey             = "ca.crt"
)

// Vault credential-driver end-to-end coverage (HYPERSHELL-369).
//
// The control-plane suite owns the behaviors the control plane is responsible for:
//   - the flat-vault-default happy path (a gateway provisions vault-backed, the serving CA
//     is mirrored, no KEK) is asserted in the long suite by p1_2InfraVerification;
//   - the real gateway-process resolve under its per-tenant mount
//     (openshell-gw/<namespace>/.../provider-credentials/*) is exercised end-to-end by the
//     sandbox lifecycle (p2_1), which drives the actual OpenShell gateway, not a stand-in;
//   - fail-closed when Vault is unreachable is asserted here (p2_7), by making the per-cluster
//     Vault unreachable to the control plane and provisioning a gateway through the product API.
//
// The templated per-tenant policy itself (round-trip under the token's own
// openshell-gw/<namespace>/.../provider-credentials/*, no list, metadata delete allowed, no reach
// into another tenant's mount) is the GitOps Contract, authored in hypershell-gitops. It is
// verified once, in the Vault fixture, by scripts/cluster/vault.sh's own self-test -- not by
// the suite reaching into Vault with a synthetic identity. See
// specs/platform/openshell-gateway-credentials.spec.md (Requirement: End-to-End Vault
// Coverage in CI).

// assertVaultProviderSecretNotDisclosed asserts that a vault-backed gateway does not disclose
// or persist OpenShell Provider secret material outside Vault (HYPERSHELL-369). The generated
// gateway configuration selects the vault driver and carries only Vault *references* -- a
// Kubernetes-auth ServiceAccount token plus a CA-bundle path -- never inline credential
// material; and the in-tree / KEK credential-storage path that persisted provider secrets in
// the gateway database is absent, both from the generated config and as a Kubernetes Secret in
// the tenant namespace. Provider secret values therefore live only in Vault, resolved at runtime
// under the gateway's per-tenant mount; the gateway database retains at most an opaque handle.
// See specs/platform/openshell-gateway-credentials.spec.md (Scenario OSG-CRED-14.03).
//
// This covers non-disclosure at the generated-configuration and manifest layer. Database-record
// and API-response non-disclosure follow by construction: with no KEK credential-storage path
// there is no persisted provider secret to serialize, and the gateway API exposes only the
// gateway resource, not Vault contents.
func (s *E2ESuite) assertVaultProviderSecretNotDisclosed(t *testing.T, ns string) {
	t.Helper()
	ctx := t.Context()

	cfg, err := s.clients.Kube.CoreV1().ConfigMaps(ns).Get(ctx, "openshell-gateway-config", metav1.GetOptions{})
	s.Require().NoError(err, "get gateway config ConfigMap")
	toml := cfg.Data["gateway.toml"]

	// The generated config resolves provider credentials from Vault by reference only: a
	// Kubernetes-auth SA token plus a CA-bundle *path*, never an embedded secret value.
	s.Assert().Contains(toml, "[openshell.credential_drivers.vault]", "gateway.toml selects the vault credential driver")
	s.Assert().Contains(toml, `auth_method = "kubernetes"`, "vault driver authenticates by Kubernetes SA token (a reference, not an embedded secret)")
	s.Assert().Contains(toml, "ca_bundle =", "vault driver trusts Vault via a CA-bundle path reference")

	// The in-tree / KEK credential-storage path -- the mechanism that persisted provider
	// secrets in the gateway database -- must be absent from the generated configuration.
	s.Assert().NotContains(toml, "[openshell.gateway.credential_storage]", "no KEK credential-storage stanza in generated config (provider secrets are not DB-persisted)")
	s.Assert().NotContains(toml, "in_tree", "no in-tree credential transport in generated config (provider secrets are not DB-persisted)")

	// No KEK Secret, and no provider-credential Secret, is persisted in the tenant namespace:
	// provider secret values live only in Vault.
	_, kekErr := s.clients.Kube.CoreV1().Secrets(ns).Get(ctx, "openshell-gateway-credential-kek", metav1.GetOptions{})
	s.Assert().True(apierrors.IsNotFound(kekErr), "no KEK secret is provisioned for a vault-backed gateway")

	secrets, err := s.clients.Kube.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	s.Require().NoError(err, "list tenant-namespace secrets")
	for _, sec := range secrets.Items {
		name := strings.ToLower(sec.Name)
		if strings.Contains(name, "provider-cred") || strings.Contains(name, "provider-secret") || strings.Contains(name, "credential-kek") {
			s.Failf("provider secret persisted as a Kubernetes Secret",
				"tenant namespace %s carries Secret %q; vault-backed provider secrets must live only in Vault", ns, sec.Name)
		}
	}
}

// p2_7VaultFailClosed asserts the control plane fails a vault-backed gateway closed when the
// per-cluster Vault is unreachable: EnvironmentReady reports a Vault reachability failure and
// the gateway never reaches Running.
//
// Vault is a black box (HYPERSHELL-369): there are no per-gateway overrides, so a gateway
// cannot be pointed at a bad address through the API. Vault reachability is a cluster-wide
// property (one Vault per cluster, one convention address), so the scenario makes that Vault
// unreachable to the control plane by corrupting the serving-CA source Secret the reachability
// probe reads, then restores it. Because that break is cluster-wide, this step runs serially as
// the final behavior check (after the parallel P2 wave and read-only P3), never concurrently
// with other gateway provisioning. It needs kube access to mutate the CA source Secret.
func (s *E2ESuite) p2_7VaultFailClosed(t *testing.T) {
	s.vaultFailClosedScenario(t.Context(), t)
}

// vaultFailClosedScenario makes the per-cluster Vault unreachable to the control plane, then
// provisions a gateway and asserts fail-closed: EnvironmentReady reports a Vault reachability
// failure and the gateway never reaches Running. It restores Vault reachability before returning.
func (s *E2ESuite) vaultFailClosedScenario(ctx context.Context, t *testing.T) {
	restore := s.breakVaultReachability(ctx, t)
	defer restore()

	name := "e2e-vault-failclosed-" + s.runID
	// Vault-backed with no overrides (black box): credential_driver is
	// control-plane-managed and not user-settable (PLAT-DATA-23), and the flat
	// vault default already applies, so the create carries only name + placement.
	// The control plane resolves the convention address and reads the (now
	// corrupted) serving CA, so the pre-deploy reachability probe fails closed.
	body := gatewayCreateRequestForDriver(name, s.driver.Name())
	status, resp, err := s.admin.RawJSON(ctx, http.MethodPost, "/gateways", body)
	s.Require().NoError(err, "create fail-closed gateway")
	s.Require().Equalf(http.StatusCreated, status, "create fail-closed gateway status (body: %s)", string(resp))
	var created sdktypes.Gateway
	s.Require().NoError(json.Unmarshal(resp, &created), "unmarshal fail-closed gateway")
	s.trackGateway(created.ID)
	defer func() { _ = s.admin.Gateways().Delete(context.Background(), created.ID) }()

	timeout := durationSecondsEnv("E2E_VAULT_FAILCLOSED_TIMEOUT", 150*time.Second)
	var lastReason string
	err = harness.Poll(ctx, 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
		gw, getErr := s.admin.Gateways().Get(ctx, created.ID)
		if getErr != nil {
			return false, nil // transient
		}
		if gw.Phase == "Running" {
			return false, fmt.Errorf("fail-closed gateway reached Running despite an unreachable Vault")
		}
		for _, cond := range gw.ProvisioningConditions {
			if typeName, _ := cond["type"].(string); typeName != "EnvironmentReady" {
				continue
			}
			lastReason = conditionText(cond)
			statusStr, _ := cond["condition_status"].(string)
			if statusStr != "Complete" && mentionsVault(lastReason) {
				return true, nil
			}
		}
		return false, nil
	})
	s.Require().NoErrorf(err, "fail-closed: EnvironmentReady never reported a Vault reachability failure (last=%q)", lastReason)
}

// breakVaultReachability makes the per-cluster Vault unreachable to the control plane by
// replacing the serving-CA source Secret's ca.crt with bytes that are not a usable
// certificate, so the control plane's reachability probe (which reads that CA) fails for any
// newly provisioned gateway. It returns a restore function that puts the original CA back.
// The Secret is a plain fixture Secret (not cert-manager-managed), so the edit sticks until
// restored. Scoped to this serial step so it never races with concurrent provisioning.
func (s *E2ESuite) breakVaultReachability(ctx context.Context, t *testing.T) func() {
	secrets := s.clients.Kube.CoreV1().Secrets(vaultCASourceNamespace)
	orig, err := secrets.Get(ctx, vaultCASourceSecret, metav1.GetOptions{})
	s.Require().NoErrorf(err, "get Vault serving-CA source Secret %s/%s", vaultCASourceNamespace, vaultCASourceSecret)
	originalCA := append([]byte(nil), orig.Data[vaultCAKey]...)
	s.Require().NotEmptyf(originalCA, "Vault serving-CA source Secret %s/%s has no %q", vaultCASourceNamespace, vaultCASourceSecret, vaultCAKey)

	broken := orig.DeepCopy()
	broken.Data[vaultCAKey] = []byte("fail-closed-e2e: not a certificate")
	_, err = secrets.Update(ctx, broken, metav1.UpdateOptions{})
	s.Require().NoError(err, "corrupt Vault serving-CA source Secret to simulate unreachable Vault")

	return func() {
		// Restore on the background context so a cancelled test context still reverts the break.
		rctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cur, getErr := secrets.Get(rctx, vaultCASourceSecret, metav1.GetOptions{})
		if getErr != nil {
			t.Logf("WARN could not re-read Vault CA source Secret to restore it: %v", getErr)
			return
		}
		cur.Data[vaultCAKey] = originalCA
		if _, updErr := secrets.Update(rctx, cur, metav1.UpdateOptions{}); updErr != nil {
			t.Logf("WARN could not restore Vault CA source Secret: %v", updErr)
		}
	}
}

// conditionText flattens a provisioning condition's values so the Vault reason can be
// matched regardless of the exact reason/message key.
func conditionText(cond map[string]any) string {
	var b strings.Builder
	for k, v := range cond {
		fmt.Fprintf(&b, " %s=%v", k, v)
	}
	return b.String()
}

func mentionsVault(text string) bool {
	l := strings.ToLower(text)
	return strings.Contains(l, "vault") || strings.Contains(l, "reach") || strings.Contains(l, "unreachable")
}
