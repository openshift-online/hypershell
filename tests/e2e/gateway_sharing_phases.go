package e2e

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"github.com/openshift-online/hypershell/tests/e2e/driver"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// p2_8GatewaySharingRoleBindings validates the role_bindings REST contract
// (E2E-COV-02): an owner shares gateway:viewer access with a second user, the
// RoleBinding reconciler propagates it into Keycloak, the second user gains and
// then loses the ability to create a sandbox, a non-owner cannot grant access,
// and the Keycloak-only roles (gateway:creator, platform:admin) are refused.
// This phase owns a dedicated gateway, distinct from the primary gateway the
// gateway-access-management phase (P2.5) shares through /gateways/{id}/access.
func (s *E2ESuite) p2_8GatewaySharingRoleBindings(t *testing.T) {
	ctx := t.Context()

	gw := s.createGateway(t, "e2e-share-"+s.runID)
	s.trackGateway(gw.ID)
	running := s.waitGatewayRunning(t, gw.ID)
	ref := driver.GatewayRef{Name: running.Name, Namespace: running.Namespace, ID: running.ID}
	s.Require().NoError(s.driver.WaitForGatewayRoute(ctx, ref), "wait for sharing gateway route")
	endpoint, err := s.driver.DiscoverGatewayEndpoint(ctx, ref)
	s.Require().NoError(err, "discover sharing gateway endpoint")
	ownerTok, err := s.gatewayToken(ctx, ref)
	s.Require().NoError(err, "owner per-gateway admin token")
	ownerLocal := s.registerGateway(t, ref, endpoint, ownerTok.AccessToken)
	clientID := fmt.Sprintf("%s-%s", gw.Name, gw.ID)

	// The second user must have a /users record before it can be resolved. The
	// record is auto-provisioned on the user's first authenticated platform
	// request (the api-server provisions on every authenticated request), not by
	// minting a Keycloak token, so make one authenticated call as the developer
	// before resolving the record. Even a 403 passes through the provisioning
	// middleware, so the status is irrelevant here.
	devAPI := s.apiClientForCreds(t, s.devCreds())
	_, _, err = devAPI.RawJSON(ctx, http.MethodGet, "/gateways", nil)
	s.Require().NoError(err, "developer authenticated request triggers user auto-provisioning")
	devUserID := s.findUserIDByUsername(t, s.devCreds().Username)
	viewerRoleID := s.findRoleIDByName(t, "gateway:viewer")

	// E2E-COV-02.03 (first half): a non-owner cannot grant gateway access.
	status, body, err := devAPI.RawJSON(ctx, http.MethodPost, "/role_bindings", sdktypes.RoleBinding{
		Scope: "gateway", RoleID: viewerRoleID, UserID: devUserID, GatewayID: gw.ID,
	})
	s.Require().NoError(err, "non-owner role_bindings create")
	s.Assert().Equalf(http.StatusForbidden, status, "non-owner gateway:viewer grant must be refused (body: %s)", string(body))

	// E2E-COV-02.03 (second half): gateway:creator and platform:admin are
	// Keycloak-only and refused regardless of caller (the admin here does own
	// the gateway, so only the role itself can cause the refusal).
	for _, roleName := range []string{"gateway:creator", "platform:admin"} {
		roleID := s.findRoleIDByName(t, roleName)
		status, body, err := s.admin.RawJSON(ctx, http.MethodPost, "/role_bindings", sdktypes.RoleBinding{Scope: "global", RoleID: roleID})
		s.Require().NoErrorf(err, "role_bindings create for %s", roleName)
		s.Assert().Equalf(http.StatusForbidden, status, "%s can only be assigned via Keycloak (body: %s)", roleName, string(body))
	}

	// E2E-COV-02.01: the owner shares viewer access and the reconciler propagates it.
	binding, err := s.admin.RoleBindings().Create(ctx, &sdktypes.RoleBinding{
		Scope: "gateway", RoleID: viewerRoleID, UserID: devUserID, GatewayID: gw.ID,
	})
	s.Require().NoError(err, "owner creates a gateway:viewer role binding")
	s.Require().NotEmpty(binding.ID, "created role binding id")

	grantTok, err := s.driver.AcquireGatewayTokenWithRole(ctx, s.devCreds(), clientID, "openshell-user")
	s.Require().NoError(err, "second user's per-gateway token gains openshell-user after the grant")
	claims, err := decodeJWTClaims(grantTok.AccessToken)
	s.Require().NoError(err, "decode second user's per-gateway token")
	subject, _ := claims["sub"].(string)
	s.Require().NotEmpty(subject, "second user token subject")

	developerLocal := cliLocalName(ref.Namespace) + "-shared-viewer"
	s.registerGatewayCLI(t, cliGatewayMetadata{
		Name:            developerLocal,
		GatewayEndpoint: endpoint,
		IsRemote:        true,
		AuthMode:        "oidc",
		OIDCIssuer:      envOrDefault("E2E_OIDC_ISSUER", "https://keycloak.hypershell.localhost/realms/hypershell"),
		OIDCClientID:    clientID,
		GatewayInsecure: gatewayTLSInsecure(s.driver.Name()),
	}, grantTok.AccessToken)
	out, err := s.cli(t, ownerLocal, "workspace", "member", "add", "--workspace", "default", "--subject", subject, "--role", "user")
	if err != nil && !strings.Contains(strings.ToLower(out), "already") && !strings.Contains(strings.ToLower(out), "exists") {
		s.Require().NoError(err, "owner grants second user workspace membership: %s", out)
	}

	name := fmt.Sprintf("share-%06d", time.Now().UnixNano()%1000000)
	if out, err = s.cli(t, developerLocal, "sandbox", "create", "--from", s.sandboxImage, "--name", name); err != nil {
		s.Logf("second-user sandbox create returned an error (continuing to poll): %v\n%s", err, out)
	}
	s.Require().NoError(s.waitPodRunning(ctx,
		sandboxPodNamespace(running.WorkspaceMode, running.Namespace, "default"),
		sandboxPodName(running.WorkspaceMode, "default", name),
		durationSecondsEnv("E2E_SANDBOX_TIMEOUT", 120*time.Second)),
		"second user may create a sandbox after the grant")
	s.deleteSandboxAndWaitForCount(t, developerLocal, name, gw.ID, 0)

	// E2E-COV-02.02: revoking the binding removes access.
	s.Require().NoError(s.admin.RoleBindings().Delete(ctx, binding.ID), "owner revokes the role binding")

	// Revocation propagates to Keycloak asynchronously. Poll a FRESH per-gateway
	// token (minted without requiring any role) until its claims no longer carry
	// openshell-user; this proves the revoke took effect rather than accepting any
	// transient token error. Reusing the already-issued grant token would only test
	// that token's natural expiry, not the revoke.
	var postRevokeTok driver.Token
	revokeErr := harness.Poll(ctx, 2*time.Second, durationSecondsEnv("E2E_ROLE_REVOKE_TIMEOUT", 60*time.Second), func(ctx context.Context) (bool, error) {
		tok, err := s.driver.AcquireGatewayToken(ctx, s.devCreds(), clientID)
		if err != nil {
			return false, nil // transient; keep polling
		}
		if driver.TokenHasRole(tok.AccessToken, "openshell-user") {
			return false, nil // revoke not yet propagated
		}
		postRevokeTok = tok
		return true, nil
	})
	s.Require().NoError(revokeErr, "second user's fresh per-gateway token must drop openshell-user after revoke")

	// E2E-COV-02.02: prove the denial end to end - a sandbox create with the
	// role-less token must be refused, not just that the role is gone from claims.
	deniedLocal := cliLocalName(ref.Namespace) + "-revoked-viewer"
	s.registerGatewayCLI(t, cliGatewayMetadata{
		Name:            deniedLocal,
		GatewayEndpoint: endpoint,
		IsRemote:        true,
		AuthMode:        "oidc",
		OIDCIssuer:      envOrDefault("E2E_OIDC_ISSUER", "https://keycloak.hypershell.localhost/realms/hypershell"),
		OIDCClientID:    clientID,
		GatewayInsecure: gatewayTLSInsecure(s.driver.Name()),
	}, postRevokeTok.AccessToken)
	deniedName := fmt.Sprintf("revoked-%06d", time.Now().UnixNano()%1000000)
	out, err = s.cli(t, deniedLocal, "sandbox", "create", "--from", s.sandboxImage, "--name", deniedName)
	s.Require().Errorf(err, "sandbox create with a revoked (role-less) token must fail (output: %s)", out)
	s.Assert().Truef(isAuthorizationDenial(out), "the revoked sandbox create must be refused as an authorization error, not a transient failure (output: %s)", out)
	// Prove nothing slipped through: the gateway's active sandbox count stays zero.
	s.pollActiveSandboxCount(t, gw.ID, 0)

	s.deleteGatewayAndWaitGC(t, gw.ID, ref.Namespace)
}

// isAuthorizationDenial reports whether CLI output denotes an authorization
// refusal (forbidden / unauthorized / permission denied) rather than a transient
// connection or gateway error, so a revoke check cannot pass on an unrelated
// failure.
func isAuthorizationDenial(output string) bool {
	lower := strings.ToLower(stripANSI(output))
	for _, marker := range []string{
		"forbidden",
		"unauthorized",
		"permission denied",
		"permissiondenied",
		"does not have permission",
		"not authorized",
		"access denied",
		"status 403",
		" 403",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// findUserIDByUsername resolves username's id through the admin /users inventory.
func (s *E2ESuite) findUserIDByUsername(t *testing.T, username string) string {
	// Auto-provisioning is conditional and can lag behind the request that
	// triggers it (HYPERSHELL-415), so poll /users rather than reading once.
	var id string
	err := harness.Poll(t.Context(), 2*time.Second, durationSecondsEnv("E2E_USER_PROVISION_TIMEOUT", 60*time.Second), func(ctx context.Context) (bool, error) {
		list, listErr := s.admin.Users().List(ctx, &sdktypes.ListOptions{Size: 200})
		if listErr != nil {
			return false, nil
		}
		for _, u := range list.Items {
			if u.Username == username {
				id = u.ID
				return true, nil
			}
		}
		return false, nil
	})
	s.Require().NoErrorf(err, "username %q not present in /users", username)
	return id
}

// findRoleIDByName resolves name's id through Roles().List, since role_bindings
// binds by role_id, not role name.
func (s *E2ESuite) findRoleIDByName(t *testing.T, name string) string {
	list, err := s.admin.Roles().List(t.Context(), &sdktypes.ListOptions{Size: 200})
	s.Require().NoErrorf(err, "list roles to resolve %s", name)
	for _, r := range list.Items {
		if r.Name == name {
			return r.ID
		}
	}
	s.Require().Failf("role not found", "role %q not present in /roles", name)
	return ""
}
