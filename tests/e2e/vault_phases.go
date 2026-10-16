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
// against the stand-in HashiCorp Vault that scripts/cluster/vault.sh stands up in both the
// Kind and OpenShift environments, configured to the GitOps Contract (KV v2 at secret,
// kubernetes auth, the openshell-gateway role + capability-scoped policy, serving CA). They
// prove the cloud-agnostic driver path that is identical on every cloud; the cloud-specific
// seal, storage, and identity are validated per cluster in the gitops repo, not here.
//
// The flat-vault-default happy path (a gateway provisions vault-backed, the serving CA is
// mirrored, no KEK) is asserted in the short suite by p1_2InfraVerification, and the real
// gateway-process resolve at provider-credentials/* is exercised by the sandbox lifecycle
// phases. This deeper, tagLong phase adds the Vault-specific spec scenarios: round-trip at
// the real path, the capability boundary (no list, no reach outside the prefix), and
// fail-closed. See specs/platform/openshell-gateway-credentials.spec.md (Requirement:
// End-to-End Vault Coverage in CI).
const (
	vaultNamespace     = "openshell-vault"
	vaultPodLabel      = "app=openshell-gateway-vault"
	vaultK8sRole       = "openshell-gateway"
	vaultGatewaySAName = "openshell-gateway"
	vaultTokenAudience = "vault"
	// providerCredsPrefix is the flat, hash-addressed KV prefix the OpenShell gateway runtime
	// uses for every provider credential; the gateway policy is capability-scoped to it.
	providerCredsPrefix = "secret/openshell/provider-credentials"
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

	// Scenario 1: a Vault-backed gateway stores and reads a provider secret at the exact
	// flat, hash-addressed prefix the OpenShell runtime uses. Driven as the gateway's own
	// identity (the openshell-gateway SA minted with the vault audience) because the
	// product's provider-credential API is not exercised by the suite; this proves the
	// exact mechanism the gateway uses: kubernetes-auth login under the openshell-gateway
	// role, then create/read under provider-credentials/*. A unique key is used (and then
	// deleted best-effort) so it never collides with a real runtime entry in the shared
	// folder; there is no list, so leftovers would be inert anyway.
	s.ensureGatewaySA(ctx, t, nsA)
	tokA := s.mintVaultToken(ctx, t, nsA, vaultGatewaySAName)
	rtKey := providerCredsPrefix + "/e2e-roundtrip-" + s.runID
	out, err := s.vaultAsGateway(ctx, tokA, fmt.Sprintf(`
vault kv put %[1]s value=hs369-roundtrip >/dev/null || { echo WRITE_FAILED; exit 1; }
GOT=$(vault kv get -field=value %[1]s) || { echo READ_FAILED; exit 1; }
vault kv metadata delete %[1]s >/dev/null 2>&1 || true
echo "ROUNDTRIP=$GOT"
`, rtKey))
	s.Require().NoErrorf(err, "scenario 1 round-trip exec: %s", out)
	s.Assert().Containsf(out, "ROUNDTRIP=hs369-roundtrip", "provider secret round-trips under %s/* (out: %s)", providerCredsPrefix, out)

	// Scenario 2: the capability boundary holds. As the gateway's own identity: (a) a LIST
	// on the provider-credentials folder is denied (no list capability, so one gateway can
	// neither enumerate nor discover another's entries), and (b) any read/write outside the
	// provider-credentials/ prefix is denied (the policy grants nothing there).
	outCap, err := s.vaultAsGateway(ctx, tokA, fmt.Sprintf(`
if vault kv list %[1]s 2>&1 | grep -qiE "permission denied|403"; then echo LIST_DENIED; else echo LIST_ALLOWED; fi
if vault kv put secret/openshell/not-allowed/x value=nope 2>&1 | grep -qiE "permission denied|403"; then echo OUTSIDE_WRITE_DENIED; else echo OUTSIDE_WRITE_ALLOWED; fi
if vault kv get secret/openshell/not-allowed/x 2>&1 | grep -qiE "permission denied|403"; then echo OUTSIDE_READ_DENIED; else echo OUTSIDE_READ_ALLOWED; fi
`, providerCredsPrefix))
	s.Require().NoErrorf(err, "scenario 2 capability exec: %s", outCap)
	s.Assert().Containsf(outCap, "LIST_DENIED", "gateway must be denied LIST on the provider-credentials folder (out: %s)", outCap)
	s.Assert().Containsf(outCap, "OUTSIDE_WRITE_DENIED", "gateway must be denied writes outside provider-credentials/ (out: %s)", outCap)
	s.Assert().Containsf(outCap, "OUTSIDE_READ_DENIED", "gateway must be denied reads outside provider-credentials/ (out: %s)", outCap)

	// Scenario 3: fail-closed is observable when Vault is unavailable. Provision a
	// gateway whose credential driver points at an unreachable Vault address and assert
	// the pre-deploy reachability probe fails EnvironmentReady with a Vault-named reason,
	// and the gateway never reports Running.
	s.vaultFailClosedScenario(ctx, t)
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
	// The stand-in Vault serves TLS with a DNS-only SAN; the in-pod CLI reaches it over
	// the loopback listener, so point VAULT_ADDR at 127.0.0.1 and skip verification for the
	// exec (matches scripts/cluster/vault.sh; prod TLS trust is covered by the control-plane
	// reachability/authn probes, not this test harness).
	wrapper := `export VAULT_ADDR=https://127.0.0.1:8200 VAULT_SKIP_VERIFY=true
read -r JWT
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
