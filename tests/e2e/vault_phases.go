package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// Vault credential-driver end-to-end coverage (HYPERSHELL-369). These scenarios run
// against the stand-in HashiCorp Vault that scripts/kind/vault.sh stands up in the Kind
// environment, configured to the GitOps Contract (KV v2 at secret, kubernetes auth, the
// openshell-gateway role + namespace-templated policy, serving CA). They prove the
// cloud-agnostic driver path that is identical on every cloud; the cloud-specific seal,
// storage, and identity are validated per cluster in the gitops repo, not here.
//
// The flat-vault-default happy path (a gateway provisions vault-backed, the serving CA
// is mirrored, no KEK) is asserted in the short suite by p1_2InfraVerification. This
// deeper, tagLong phase adds the three spec scenarios: round-trip, namespace isolation,
// and fail-closed. See specs/platform/openshell-gateway-credentials.spec.md
// (Requirement: End-to-End Vault Coverage in CI).
const (
	vaultNamespace     = "openshell-vault"
	vaultPodLabel      = "app=openshell-gateway-vault"
	vaultK8sRole       = "openshell-gateway"
	vaultGatewaySAName = "openshell-gateway"
	vaultTokenAudience = "vault"
	// Read-only verifier identity (scripts/cluster/vault.sh) the suite uses to assert Vault
	// state without root (e.g. that a deleted gateway's KV path was purged).
	vaultVerifierRole   = "e2e-verifier"
	vaultVerifierSAName = "e2e-verifier"
)

// vaultPod resolves the stand-in Vault pod name by label (scripts/cluster/vault.sh runs it
// as a Deployment, so the name is not a fixed StatefulSet ordinal).
func (s *E2ESuite) vaultPod(ctx context.Context) (string, error) {
	pods, err := s.clients.Kube.CoreV1().Pods(vaultNamespace).List(ctx, metav1.ListOptions{LabelSelector: vaultPodLabel})
	if err != nil {
		return "", err
	}
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			return p.Name, nil
		}
	}
	return "", fmt.Errorf("no Running pod with %q in namespace %s", vaultPodLabel, vaultNamespace)
}

// p2_7VaultCredentialCoverage exercises the three end-to-end Vault scenarios. It is a
// single phase (one concern, like p2_2/p2_5) with three labeled, sequential blocks.
func (s *E2ESuite) p2_7VaultCredentialCoverage(t *testing.T) {
	switch s.driver.Name() {
	case "kind", "openshift":
		// Both environments stand up the stand-in Vault (scripts/cluster/vault.sh).
	default:
		t.Skipf("vault credential coverage needs the stand-in Vault (driver=%s)", s.driver.Name())
	}
	if s.skipKubeChecks {
		t.Skip("vault credential coverage needs kube exec/token access")
	}
	ctx := t.Context()

	// Confirm the stand-in Vault is present before exercising it.
	if _, err := s.vaultPod(ctx); err != nil {
		t.Skipf("stand-in Vault pod not found in %s: %v", vaultNamespace, err)
	}

	nsA := s.primary.Namespace
	s.Require().NotEmpty(nsA, "primary gateway namespace")

	// Scenario 1: a Vault-backed gateway stores and reads a provider secret.
	// Driven as the gateway's own identity (the openshell-gateway SA minted with the
	// vault audience) because the product's provider-credential API is not exercised by
	// the suite; this proves the exact mechanism the gateway uses: kubernetes-auth login
	// under the openshell-gateway role, then write/read/list under its namespace path.
	s.ensureGatewaySA(ctx, t, nsA)
	tokA := s.mintVaultToken(ctx, t, nsA, vaultGatewaySAName)
	out, err := s.vaultAsGateway(ctx, tokA, fmt.Sprintf(`
vault kv put secret/openshell/%[1]s/e2e-roundtrip value=hs369-roundtrip >/dev/null || { echo WRITE_FAILED; exit 1; }
GOT=$(vault kv get -field=value secret/openshell/%[1]s/e2e-roundtrip) || { echo READ_FAILED; exit 1; }
vault kv list secret/openshell/%[1]s >/dev/null || { echo LIST_FAILED; exit 1; }
echo "ROUNDTRIP=$GOT"
`, nsA))
	s.Require().NoErrorf(err, "scenario 1 round-trip exec: %s", out)
	s.Assert().Containsf(out, "ROUNDTRIP=hs369-roundtrip", "provider secret round-trips under secret/data/openshell/%s/* (out: %s)", nsA, out)

	// Scenario 2: namespace isolation holds between two gateway identities. A second
	// tenant namespace with its own openshell-gateway SA (the role binds that SA name in
	// any namespace) must reach only its own path and be denied the first's, and vice
	// versa.
	nsB := "vault-e2e-tenant-b-" + s.runID
	s.ensureGatewaySA(ctx, t, nsB)
	defer func() {
		_ = s.clients.Kube.CoreV1().Namespaces().Delete(context.Background(), nsB, metav1.DeleteOptions{})
	}()
	tokB := s.mintVaultToken(ctx, t, nsB, vaultGatewaySAName)
	outB, err := s.vaultAsGateway(ctx, tokB, fmt.Sprintf(`
vault kv put secret/openshell/%[1]s/own value=b-ok >/dev/null || { echo B_OWN_WRITE_FAILED; exit 1; }
if vault kv get secret/openshell/%[2]s/e2e-roundtrip 2>&1 | grep -qiE "permission denied|403"; then echo B_DENIED_A; else echo B_REACHED_A; fi
`, nsB, nsA))
	s.Require().NoErrorf(err, "scenario 2 isolation exec (B): %s", outB)
	s.Assert().Containsf(outB, "B_DENIED_A", "gateway B must be denied gateway A's path (out: %s)", outB)
	s.Assert().NotContains(outB, "B_OWN_WRITE_FAILED", "gateway B must reach its own path")

	outA, err := s.vaultAsGateway(ctx, tokA, fmt.Sprintf(`
if vault kv get secret/openshell/%[1]s/own 2>&1 | grep -qiE "permission denied|403"; then echo A_DENIED_B; else echo A_REACHED_B; fi
`, nsB))
	s.Require().NoErrorf(err, "scenario 2 isolation exec (A): %s", outA)
	s.Assert().Containsf(outA, "A_DENIED_B", "gateway A must be denied gateway B's path (out: %s)", outA)

	// Scenario 3: fail-closed is observable when Vault is unavailable. Provision a
	// gateway whose credential driver points at an unreachable Vault address and assert
	// the pre-deploy reachability probe fails EnvironmentReady with a Vault-named reason,
	// and the gateway never reports Running.
	s.vaultFailClosedScenario(ctx, t)

	// Scenario 4: provider-secret purge on gateway delete. Prod runs this on every delete,
	// so cover it end to end: a dedicated gateway (the primary is left untouched), a secret
	// written under its namespace path, then delete and verify the path is purged.
	s.vaultPurgeScenario(ctx, t)
}

// ensureGatewaySA makes sure an openshell-gateway ServiceAccount exists in namespace
// (creating the namespace too when missing). The primary gateway already has one; a
// second tenant namespace needs it created. Idempotent.
func (s *E2ESuite) ensureGatewaySA(ctx context.Context, t *testing.T, namespace string) {
	_, err := s.clients.Kube.CoreV1().Namespaces().Create(ctx,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		s.Require().NoErrorf(err, "create namespace %s", namespace)
	}
	_, err = s.clients.Kube.CoreV1().ServiceAccounts(namespace).Create(ctx,
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: vaultGatewaySAName, Namespace: namespace}}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		s.Require().NoErrorf(err, "create ServiceAccount %s/%s", namespace, vaultGatewaySAName)
	}
}

// mintVaultToken requests a short-lived ServiceAccount token with the vault audience,
// the token a gateway presents to Vault's kubernetes auth.
func (s *E2ESuite) mintVaultToken(ctx context.Context, t *testing.T, namespace, sa string) string {
	exp := int64(600)
	tr, err := s.clients.Kube.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, sa, &authnv1.TokenRequest{
		Spec: authnv1.TokenRequestSpec{Audiences: []string{vaultTokenAudience}, ExpirationSeconds: &exp},
	}, metav1.CreateOptions{})
	s.Require().NoErrorf(err, "mint vault-audience token for %s/%s", namespace, sa)
	s.Require().NotEmpty(tr.Status.Token, "token for %s/%s", namespace, sa)
	return tr.Status.Token
}

// vaultAsGateway runs a shell script inside the Vault pod authenticated as a gateway
// identity. The token is passed on stdin (never argv); the script runs with VAULT_TOKEN
// exported from a kubernetes-auth login under the openshell-gateway role.
func (s *E2ESuite) vaultAsGateway(ctx context.Context, token, script string) (string, error) {
	return s.vaultAsRole(ctx, token, vaultK8sRole, script)
}

// vaultAsRole runs a shell script inside the Vault pod authenticated under the given
// kubernetes-auth role. The token (a vault-audience SA token) is passed on stdin.
func (s *E2ESuite) vaultAsRole(ctx context.Context, token, role, script string) (string, error) {
	wrapper := `read -r JWT
CT=$(vault write -field=token auth/kubernetes/login role=` + role + ` jwt="$JWT") || { echo LOGIN_FAILED; exit 1; }
export VAULT_TOKEN="$CT"
` + script
	return s.vaultExec(ctx, token+"\n", "sh", "-c", wrapper)
}

// vaultExec runs a command inside the Kind Vault pod, returning combined stdout+stderr.
func (s *E2ESuite) vaultExec(ctx context.Context, stdin string, command ...string) (string, error) {
	pod, err := s.vaultPod(ctx)
	if err != nil {
		return "", err
	}
	req := s.clients.Kube.CoreV1().RESTClient().Post().
		Resource("pods").Name(pod).Namespace(vaultNamespace).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: command,
			Stdin:   stdin != "",
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)
	exec, err := remotecommand.NewSPDYExecutor(s.clients.Config, http.MethodPost, req.URL())
	if err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	opts := remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr}
	if stdin != "" {
		opts.Stdin = strings.NewReader(stdin)
	}
	err = exec.StreamWithContext(ctx, opts)
	return stdout.String() + stderr.String(), err
}

// vaultFailClosedScenario provisions a gateway pointed at an unreachable Vault and
// asserts fail-closed: EnvironmentReady reports a Vault reachability failure and the
// gateway never reaches Running.
func (s *E2ESuite) vaultFailClosedScenario(ctx context.Context, t *testing.T) {
	name := "e2e-vault-failclosed-" + s.runID
	body := gatewayCreateRequestForDriver(name, s.driver.Name(), s.gatewayOIDCConfig())
	// https (TLS) + non-loopback is a valid driver config; the host does not resolve, so
	// the pre-deploy reachability probe fails closed.
	body.CredentialDriver = `{"type":"vault","config":{"address":"https://openshell-gateway-vault.unreachable.svc:8200"}}`
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

// vaultVerifierToken ensures the read-only e2e-verifier ServiceAccount exists and returns a
// vault-audience token for it (used to assert Vault state without root).
func (s *E2ESuite) vaultVerifierToken(ctx context.Context, t *testing.T) string {
	_, err := s.clients.Kube.CoreV1().ServiceAccounts(vaultNamespace).Create(ctx,
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: vaultVerifierSAName, Namespace: vaultNamespace}}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		s.Require().NoErrorf(err, "create verifier ServiceAccount %s/%s", vaultNamespace, vaultVerifierSAName)
	}
	return s.mintVaultToken(ctx, t, vaultNamespace, vaultVerifierSAName)
}

// vaultPurgeScenario provisions a dedicated vault-backed gateway, writes a provider secret
// under its namespace path, deletes the gateway, and verifies the control plane purged the
// KV path (W3). The primary gateway is left untouched.
func (s *E2ESuite) vaultPurgeScenario(ctx context.Context, t *testing.T) {
	created := s.createGateway(t, "e2e-vault-purge-"+s.runID)
	s.trackGateway(created.ID)
	running := s.waitGatewayRunning(t, created.ID)
	gwNS := running.Namespace
	s.Require().NotEmpty(gwNS, "purge gateway namespace")

	// Write a provider secret under the gateway's own namespace path, as its identity.
	s.ensureGatewaySA(ctx, t, gwNS)
	gwTok := s.mintVaultToken(ctx, t, gwNS, vaultGatewaySAName)
	out, err := s.vaultAsGateway(ctx, gwTok, fmt.Sprintf(
		"vault kv put secret/openshell/%s/purge-probe value=to-be-purged >/dev/null && echo WROTE", gwNS))
	s.Require().NoErrorf(err, "purge: write probe secret: %s", out)
	s.Require().Containsf(out, "WROTE", "purge: wrote probe secret (out: %s)", out)

	// As the read-only verifier, confirm the secret is present before delete.
	verTok := s.vaultVerifierToken(ctx, t)
	vout, err := s.vaultAsRole(ctx, verTok, vaultVerifierRole, fmt.Sprintf(
		"vault kv get secret/openshell/%s/purge-probe >/dev/null 2>&1 && echo PRESENT || echo ABSENT", gwNS))
	s.Require().NoErrorf(err, "purge: pre-delete verify: %s", vout)
	s.Require().Containsf(vout, "PRESENT", "purge: probe secret present before delete (out: %s)", vout)

	// Delete the gateway; the control plane purges its KV path before removing the SA/ns.
	err = s.admin.Gateways().Delete(ctx, created.ID)
	s.Require().NoError(err, "purge: delete gateway")

	// Poll until the namespace path is purged (the probe key is gone).
	timeout := durationSecondsEnv("E2E_VAULT_PURGE_TIMEOUT", 180*time.Second)
	var last string
	perr := harness.Poll(ctx, 5*time.Second, timeout, func(pctx context.Context) (bool, error) {
		o, e := s.vaultAsRole(pctx, verTok, vaultVerifierRole, fmt.Sprintf(
			"vault kv list secret/openshell/%s 2>&1 || true", gwNS))
		if e != nil {
			return false, nil
		}
		last = o
		return !strings.Contains(o, "purge-probe"), nil
	})
	s.Require().NoErrorf(perr, "purge: KV path secret/openshell/%s not purged after delete (last list: %s)", gwNS, last)
	t.Logf("purge: gateway %s deleted and its Vault KV path purged", created.ID)
}
