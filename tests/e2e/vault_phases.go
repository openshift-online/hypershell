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
)

// Vault credential-driver end-to-end coverage (HYPERSHELL-369).
//
// The control-plane suite owns the behaviors the control plane is responsible for:
//   - the flat-vault-default happy path (a gateway provisions vault-backed, the serving CA
//     is mirrored, no KEK) is asserted in the short suite by p1_2InfraVerification;
//   - the real gateway-process resolve under its per-tenant mount
//     (openshell-gw/<namespace>/.../provider-credentials/*) is exercised end-to-end by the
//     sandbox lifecycle (p2_1), which drives the actual OpenShell gateway, not a stand-in;
//   - fail-closed when Vault is unreachable is asserted here (p2_7), through the product API.
//
// The templated per-tenant policy itself (round-trip under the token's own
// openshell-gw/<namespace>/.../provider-credentials/*, no list, no metadata delete, no reach
// into another tenant's mount) is the GitOps Contract, authored in hypershell-gitops. It is
// verified once, in the Vault fixture, by scripts/cluster/vault.sh's own self-test -- not by
// the suite reaching into Vault with a synthetic identity. See
// specs/platform/openshell-gateway-credentials.spec.md (Requirement: End-to-End Vault
// Coverage in CI).

// p2_7VaultFailClosed asserts the control plane fails a vault-backed gateway closed when its
// Vault is unreachable: EnvironmentReady reports a Vault reachability failure and the gateway
// never reaches Running. It needs no Vault fixture and no kube access -- it drives the product
// API only, pointing the driver at an address that does not resolve.
func (s *E2ESuite) p2_7VaultFailClosed(t *testing.T) {
	s.vaultFailClosedScenario(t.Context(), t)
}

// vaultFailClosedScenario provisions a gateway pointed at an unreachable Vault and asserts
// fail-closed: EnvironmentReady reports a Vault reachability failure and the gateway never
// reaches Running.
func (s *E2ESuite) vaultFailClosedScenario(ctx context.Context, t *testing.T) {
	name := "e2e-vault-failclosed-" + s.runID
	body := gatewayCreateRequestForDriver(name, s.driver.Name(), s.gatewayOIDCConfig())
	// https (TLS) + non-loopback is a valid driver config; the host does not resolve, so
	// the pre-deploy reachability probe fails closed. The override lives under the `vault`
	// key (CredentialDriverConfig.Vault), which the control plane unmarshals; any other key
	// is ignored and would silently fall back to the reachable convention address.
	body.CredentialDriver = `{"type":"vault","vault":{"address":"https://openshell-gateway-vault.unreachable.svc:8200"}}`
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
