package e2e

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The control plane reads the Vault serving CA from the Secret named by its
// GATEWAY_VAULT_CA_SECRET env ("namespace/name"). The dev/e2e drivers point it at a
// per-environment copy (scripts/cluster/vault.sh, VAULT_CA_COPY_NAMESPACE). These mirror
// the control-plane conventions (gateway.VaultCAKey and the default source), duplicated
// here because that package is internal.
const (
	vaultCAEnvVar            = "GATEWAY_VAULT_CA_SECRET"
	vaultCASharedNamespace   = "openshell-vault"
	vaultCAKey               = "ca.crt"
	controllerDeploymentName = "hypershell-controller"
	controllerContainerName  = "controller"
	vaultFailClosedBrokenCA  = "fail-closed-e2e: not a certificate"
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

// p2_7VaultFailClosed asserts the control plane fails a vault-backed gateway closed when the
// per-cluster Vault is unreachable: EnvironmentReady reports a Vault reachability failure and
// the gateway never reaches Running.
//
// Vault is a black box (HYPERSHELL-369): there are no per-gateway overrides, so a gateway
// cannot be pointed at a bad address through the API. The scenario instead corrupts the
// serving CA the control plane's reachability probe reads. That CA is this environment's own
// copy (GATEWAY_VAULT_CA_SECRET on the controller), never the shared openshell-vault Secret,
// so the break cannot leak to other environments on a shared cluster, even if this process
// is killed before it restores the CA. Within this run the break still affects every gateway
// this control plane provisions, so the step stays serial and last (after the parallel P2
// wave and read-only P3). It needs kube access to read the controller Deployment and mutate
// the copy.
// Verifies: OSG-CRED-14.02
func (s *E2ESuite) p2_7VaultFailClosed(t *testing.T) {
	s.vaultFailClosedScenario(t.Context(), t)
}

// vaultFailClosedScenario makes the per-cluster Vault unreachable to the control plane, then
// provisions a gateway and asserts fail-closed: EnvironmentReady reports a Vault reachability
// failure and the gateway never reaches Running. It restores Vault reachability before returning.
func (s *E2ESuite) vaultFailClosedScenario(ctx context.Context, t *testing.T) {
	restore := s.breakVaultReachability(ctx, t)
	t.Cleanup(restore)
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

// vaultCASourceFromController returns the namespace/name of the Secret this environment's
// control plane reads the Vault serving CA from (its GATEWAY_VAULT_CA_SECRET env). It refuses
// to proceed when the env is unset or points at the shared openshell-vault namespace: breaking
// that Secret would break Vault for every environment on the cluster (the 2026-11-20 CI
// incident, when a killed run left it corrupted).
func (s *E2ESuite) vaultCASourceFromController(ctx context.Context) (namespace, name string) {
	ns := s.driver.PlatformNamespace()
	dep, err := s.clients.Kube.AppsV1().Deployments(ns).Get(ctx, controllerDeploymentName, metav1.GetOptions{})
	s.Require().NoErrorf(err, "get controller Deployment %s/%s", ns, controllerDeploymentName)
	var value string
	for _, c := range dep.Spec.Template.Spec.Containers {
		if c.Name != controllerContainerName {
			continue
		}
		for _, e := range c.Env {
			if e.Name == vaultCAEnvVar {
				value = e.Value
			}
		}
	}
	namespace, name, ok := strings.Cut(value, "/")
	s.Require().Truef(ok && namespace != "" && name != "",
		"controller %s/%s has no usable %s (%q); the driver must point it at a per-environment CA copy before P2.7 can run",
		ns, controllerDeploymentName, vaultCAEnvVar, value)
	s.Require().NotEqualf(vaultCASharedNamespace, namespace,
		"%s points at the shared %s namespace; P2.7 refuses to corrupt shared Vault state", vaultCAEnvVar, vaultCASharedNamespace)
	return namespace, name
}

// isPEMCertificate reports whether b holds at least one parseable PEM certificate.
func isPEMCertificate(b []byte) bool {
	for {
		var block *pem.Block
		block, b = pem.Decode(b)
		if block == nil {
			return false
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		if _, err := x509.ParseCertificate(block.Bytes); err == nil {
			return true
		}
	}
}

// breakVaultReachability makes Vault unreachable to this environment's control plane by
// replacing ca.crt in the control plane's per-environment CA copy with bytes that are not a
// certificate, so the reachability probe fails for any newly provisioned gateway. It returns
// an idempotent restore function; the caller registers it with t.Cleanup as well as defer.
//
// It refuses to start when the current ca.crt is not a valid PEM certificate, so an
// already-broken value (left by an earlier killed run) is never saved as the "original" and
// written back.
func (s *E2ESuite) breakVaultReachability(ctx context.Context, t *testing.T) func() {
	caNS, caName := s.vaultCASourceFromController(ctx)
	secrets := s.clients.Kube.CoreV1().Secrets(caNS)
	orig, err := secrets.Get(ctx, caName, metav1.GetOptions{})
	s.Require().NoErrorf(err, "get Vault serving-CA Secret %s/%s", caNS, caName)
	originalCA := append([]byte(nil), orig.Data[vaultCAKey]...)
	s.Require().Truef(isPEMCertificate(originalCA),
		"Vault serving-CA Secret %s/%s %q is not a valid PEM certificate before P2.7 starts; refusing to save it as the original (re-run the environment bring-up to restore it)",
		caNS, caName, vaultCAKey)

	broken := orig.DeepCopy()
	broken.Data[vaultCAKey] = []byte(vaultFailClosedBrokenCA)
	_, err = secrets.Update(ctx, broken, metav1.UpdateOptions{})
	s.Require().NoError(err, "corrupt Vault serving-CA Secret to simulate unreachable Vault")

	var once sync.Once
	return func() {
		once.Do(func() {
			// Restore on the background context so a cancelled test context still reverts the break.
			rctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cur, getErr := secrets.Get(rctx, caName, metav1.GetOptions{})
			if getErr != nil {
				t.Logf("WARN could not re-read Vault CA Secret %s/%s to restore it: %v", caNS, caName, getErr)
				return
			}
			if cur.Data == nil {
				cur.Data = map[string][]byte{}
			}
			cur.Data[vaultCAKey] = originalCA
			if _, updErr := secrets.Update(rctx, cur, metav1.UpdateOptions{}); updErr != nil {
				t.Logf("WARN could not restore Vault CA Secret %s/%s: %v", caNS, caName, updErr)
			}
		})
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
