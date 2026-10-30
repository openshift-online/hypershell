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
//     is mirrored, no KEK) is asserted in the short suite by p1_2InfraVerification;
//   - the real gateway-process resolve under its per-tenant mount
//     (openshell-gw/<namespace>/.../provider-credentials/*) is exercised end-to-end by the
//     sandbox lifecycle (p2_1), which drives the actual OpenShell gateway, not a stand-in;
//   - fail-closed when Vault is unreachable is asserted here (p2_7), by making the per-cluster
//     Vault unreachable to the control plane and provisioning a gateway through the product API.
//
// The templated per-tenant policy itself (round-trip under the token's own
// openshell-gw/<namespace>/.../provider-credentials/*, no list, no metadata delete, no reach
// into another tenant's mount) is the GitOps Contract, authored in hypershell-gitops. It is
// verified once, in the Vault fixture, by scripts/cluster/vault.sh's own self-test -- not by
// the suite reaching into Vault with a synthetic identity. See
// specs/platform/openshell-gateway-credentials.spec.md (Requirement: End-to-End Vault
// Coverage in CI).

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
