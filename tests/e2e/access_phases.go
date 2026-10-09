package e2e

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

func (s *E2ESuite) p2_5GatewayAccessManagement(t *testing.T) {
	if s.driver.Name() != "kind" || envOrDefault("E2E_OIDC_GRANT", "password") != "password" {
		t.Skip("secondary-user gateway access matrix requires the Kind password-grant environment")
	}
	ctx := t.Context()
	gatewayID := s.primary.ID
	access := s.admin.GatewayAccesses()

	ownerList, err := access.List(ctx, gatewayID, nil)
	s.Require().NoError(err, "owner lists gateway access")
	s.Assert().Equal(sdktypes.GatewayAccessRoleOwner, ownerList.Capabilities.CallerRole)
	s.Assert().True(ownerList.Capabilities.CanManageAccess)
	s.Assert().True(ownerList.Capabilities.CanManageOwners)

	devUsername := s.devCreds().Username
	directory, err := access.SearchDirectory(ctx, gatewayID, devUsername)
	s.Require().NoError(err, "owner searches gateway user directory")
	s.Require().True(directoryContains(directory.Items, devUsername), "directory contains developer %q", devUsername)
	runtimeUsername := "e2e-directory-" + s.runID
	s.Require().NoError(s.driver.CreateTestUser(ctx, runtimeUsername, "E2e-directory-"+s.runID+"!"), "create runtime directory user")
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.driver.DeleteTestUser(cleanupCtx, runtimeUsername); err != nil {
			t.Errorf("delete runtime directory user: %v", err)
		}
	}()
	s.Require().NoError(harness.Poll(ctx, time.Second, 30*time.Second, func(ctx context.Context) (bool, error) {
		users, err := access.SearchDirectory(ctx, gatewayID, runtimeUsername)
		return err == nil && directoryContains(users.Items, runtimeUsername), nil
	}), "gateway directory picks up a user created after the suite started")

	devGrant, err := access.Create(ctx, gatewayID, &sdktypes.GatewayAccessGrantRequest{
		Username: devUsername,
		Role:     sdktypes.GatewayAccessRoleUser,
	})
	s.Require().NoError(err, "owner grants developer gateway user access")
	s.Require().NotEmpty(devGrant.UserID)

	devAPI := s.apiClientForCreds(t, s.devCreds())
	devList, err := devAPI.GatewayAccesses().List(ctx, gatewayID, nil)
	s.Require().NoError(err, "gateway user lists access")
	s.Assert().Equal(sdktypes.GatewayAccessRoleUser, devList.Capabilities.CallerRole)
	s.Assert().False(devList.Capabilities.CanManageAccess)
	status, body, err := devAPI.RawJSON(ctx, http.MethodPost, "/gateways/"+gatewayID+"/access", sdktypes.GatewayAccessGrantRequest{
		Username: s.platformAdminCreds().Username,
		Role:     sdktypes.GatewayAccessRoleUser,
	})
	s.Require().NoError(err, "gateway user access grant request")
	s.Assert().Equalf(http.StatusForbidden, status, "gateway user cannot grant access (body: %s)", string(body))

	if !s.skipKubeChecks {
		s.exerciseDeveloperSandbox(t)
	}

	s.Require().NoError(s.driver.AssignRealmRole(ctx, s.platformAdminCreds().Username, "platform:admin"), "grant platform:admin")
	platformAPI := s.apiClientForCreds(t, s.platformAdminCreds())
	adminGrant, err := access.Create(ctx, gatewayID, &sdktypes.GatewayAccessGrantRequest{
		Username: s.platformAdminCreds().Username,
		Role:     sdktypes.GatewayAccessRoleAdmin,
	})
	s.Require().NoError(err, "owner grants gateway admin access")

	platformList, err := platformAPI.GatewayAccesses().List(ctx, gatewayID, nil)
	s.Require().NoError(err, "gateway admin lists access")
	s.Assert().True(platformList.Capabilities.CanManageAccess)
	s.Assert().False(platformList.Capabilities.CanManageOwners)

	status, body, err = platformAPI.RawJSON(ctx, http.MethodPatch,
		fmt.Sprintf("/gateways/%s/access/%s", gatewayID, devGrant.UserID),
		sdktypes.GatewayAccessChangeRoleRequest{Role: sdktypes.GatewayAccessRoleOwner})
	s.Require().NoError(err, "gateway admin owner-escalation request")
	s.Assert().Equalf(http.StatusForbidden, status, "gateway admin cannot grant owner (body: %s)", string(body))

	updated, err := platformAPI.GatewayAccesses().Update(ctx, gatewayID, devGrant.UserID,
		&sdktypes.GatewayAccessChangeRoleRequest{Role: sdktypes.GatewayAccessRoleAdmin})
	s.Require().NoError(err, "gateway admin promotes user to admin")
	s.Assert().Equal(sdktypes.GatewayAccessRoleAdmin, updated.Role)
	_, err = platformAPI.GatewayAccesses().Update(ctx, gatewayID, devGrant.UserID,
		&sdktypes.GatewayAccessChangeRoleRequest{Role: sdktypes.GatewayAccessRoleUser})
	s.Require().NoError(err, "gateway admin restores user role")

	updated, err = access.Update(ctx, gatewayID, adminGrant.UserID,
		&sdktypes.GatewayAccessChangeRoleRequest{Role: sdktypes.GatewayAccessRoleOwner})
	s.Require().NoError(err, "owner grants another owner")
	s.Assert().Equal(sdktypes.GatewayAccessRoleOwner, updated.Role)
	_, err = access.Update(ctx, gatewayID, adminGrant.UserID,
		&sdktypes.GatewayAccessChangeRoleRequest{Role: sdktypes.GatewayAccessRoleAdmin})
	s.Require().NoError(err, "owner restores admin role")

	crossGateway := s.createGateway(t, "e2e-access-"+s.runID)
	s.trackGateway(crossGateway.ID)
	crossGrant, err := platformAPI.GatewayAccesses().Create(ctx, crossGateway.ID, &sdktypes.GatewayAccessGrantRequest{
		Username: devUsername,
		Role:     sdktypes.GatewayAccessRoleUser,
	})
	s.Require().NoError(err, "platform admin grants access on a gateway it does not own")
	_, err = platformAPI.GatewayAccesses().Delete(ctx, crossGateway.ID, crossGrant.UserID)
	s.Require().NoError(err, "platform admin revokes access on a gateway it does not own")

	_, err = access.Delete(ctx, gatewayID, devGrant.UserID)
	s.Require().NoError(err, "owner revokes developer access")
	finalList, err := access.List(ctx, gatewayID, nil)
	s.Require().NoError(err, "list access after revoke")
	s.Assert().False(accessListContains(finalList.Items, devUsername), "revoked developer absent from access list")
}

func (s *E2ESuite) exerciseDeveloperSandbox(t *testing.T) {
	ctx := t.Context()
	clientID := fmt.Sprintf("%s-%s", s.primary.Name, s.primary.ID)
	tok, err := s.driver.AcquireGatewayTokenWithRole(ctx, s.devCreds(), clientID, "openshell-user")
	s.Require().NoError(err, "developer gateway token carries openshell-user")
	claims, err := decodeJWTClaims(tok.AccessToken)
	s.Require().NoError(err, "decode developer gateway token")
	subject, _ := claims["sub"].(string)
	s.Require().NotEmpty(subject, "developer token subject")

	endpoint, err := s.driver.DiscoverGatewayEndpoint(ctx, s.primary)
	s.Require().NoError(err, "discover primary endpoint for developer")
	developerLocal := cliLocalName(s.primary.Namespace) + "-developer"
	s.registerGatewayCLI(t, cliGatewayMetadata{
		Name:            developerLocal,
		GatewayEndpoint: endpoint,
		IsRemote:        true,
		AuthMode:        "oidc",
		OIDCIssuer:      envOrDefault("E2E_OIDC_ISSUER", "https://keycloak.hypershell.localhost/realms/hypershell"),
		OIDCClientID:    clientID,
		GatewayInsecure: gatewayTLSInsecure(s.driver.Name()),
	}, tok.AccessToken)
	out, err := s.cli(t, s.primaryLocalName, "workspace", "member", "add", "--workspace", "default", "--subject", subject, "--role", "user")
	if err != nil && !strings.Contains(strings.ToLower(out), "already") && !strings.Contains(strings.ToLower(out), "exists") {
		s.Require().NoError(err, "admin grants developer default workspace membership: %s", out)
	}

	name := fmt.Sprintf("dev-%06d", time.Now().UnixNano()%1000000)
	if out, err = s.cli(t, developerLocal, "sandbox", "create", "--name", name); err != nil {
		t.Logf("developer sandbox create returned an error (continuing to poll): %v\n%s", err, out)
	}
	s.Require().NoError(s.waitPodRunning(ctx, s.primary.Namespace, "default--"+name,
		durationSecondsEnv("E2E_SANDBOX_TIMEOUT", 120*time.Second)), "developer may create sandbox as workspace user")
	s.deleteSandboxAndWaitForCount(t, developerLocal, name, s.primary.ID, 0)
}

func (s *E2ESuite) p2_6GatewayServiceAccounts(t *testing.T) {
	ctx := t.Context()
	expires := time.Now().UTC().Add(2 * time.Hour)
	created, err := s.admin.OpenShellGatewayServiceAccounts().Create(ctx, s.primary.ID,
		&sdktypes.OpenShellGatewayServiceAccountCreateRequest{
			Name:           "e2e-service-account-" + s.runID,
			Role:           sdktypes.OpenShellGatewayServiceAccountRoleOpenshellAdmin,
			CredentialType: "client_secret",
			ExpiresAt:      &expires,
		})
	s.Require().NoError(err, "create gateway service account")
	s.Require().Equal(s.primary.ID, created.GatewayID)
	s.Require().Equal(sdktypes.OpenShellGatewayServiceAccountStatusReady, created.Status)
	s.Require().NotEmpty(created.Subject)
	s.Require().NotEmpty(created.Credential.ClientSecret)
	s.Require().Equal("client_credentials", created.Credential.GrantType)
	s.Require().Equal(fmt.Sprintf("%s-%s", s.primary.Name, s.primary.ID), created.Credential.Audience)
	s.Require().Equal(created.ClientID, created.Credential.ClientID)
	s.Require().Equal(strings.TrimSuffix(envOrDefault("E2E_OIDC_ISSUER", "https://keycloak.hypershell.localhost/realms/hypershell"), "/"), created.Credential.Issuer)
	s.Require().Equal(created.Credential.Issuer+"/protocol/openid-connect/token", created.Credential.TokenEndpoint)

	for attempt := range 2 {
		token, status, tokenErr := requestClientCredentialsToken(ctx, created.Credential)
		s.Require().NoError(tokenErr, "mint gateway service-account token %d", attempt+1)
		s.Require().Equal(http.StatusOK, status)
		claims, decodeErr := decodeJWTClaims(token)
		s.Require().NoError(decodeErr, "decode gateway service-account token")
		s.Require().NoError(validateGatewayServiceAccountClaims(claims, created), "gateway service-account token claims")
	}

	_, err = s.admin.OpenShellGatewayServiceAccounts().Revoke(ctx, s.primary.ID, created.ID)
	s.Require().NoError(err, "revoke gateway service account")
	s.Require().NoError(harness.Poll(ctx, 2*time.Second, 90*time.Second, func(ctx context.Context) (bool, error) {
		_, status, err := requestClientCredentialsToken(ctx, created.Credential)
		return err == nil && (status == http.StatusBadRequest || status == http.StatusUnauthorized), nil
	}), "revoked gateway service account still issues tokens")

	_, err = s.admin.OpenShellGatewayServiceAccounts().Delete(ctx, s.primary.ID, created.ID)
	s.Require().NoError(err, "delete gateway service account")
	s.Require().NoError(harness.Poll(ctx, 2*time.Second, 90*time.Second, func(ctx context.Context) (bool, error) {
		status, _, err := s.admin.RawJSON(ctx, http.MethodGet,
			fmt.Sprintf("/gateways/%s/service_accounts/%s", s.primary.ID, created.ID), nil)
		return err == nil && status == http.StatusNotFound, nil
	}), "gateway service account was not deleted")
}

func directoryContains(items []sdktypes.GatewayDirectoryUser, username string) bool {
	for _, item := range items {
		if item.Username == username {
			return true
		}
	}
	return false
}

func accessListContains(items []sdktypes.GatewayAccessListItem, username string) bool {
	for _, item := range items {
		if item.Username == username {
			return true
		}
	}
	return false
}

func requestClientCredentialsToken(ctx context.Context, credential sdktypes.OpenShellGatewayServiceAccountCredential) (string, int, error) {
	form := url.Values{
		"grant_type":    {credential.GrantType},
		"client_id":     {credential.ClientID},
		"client_secret": {credential.ClientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, credential.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("build gateway service-account token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("gateway service-account token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("read gateway service-account token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, nil
	}
	var response struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", resp.StatusCode, fmt.Errorf("decode gateway service-account token response: %w", err)
	}
	if response.AccessToken == "" {
		return "", resp.StatusCode, fmt.Errorf("token response has no access_token")
	}
	return response.AccessToken, resp.StatusCode, nil
}

func decodeJWTClaims(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode JWT payload: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("parse JWT claims: %w", err)
	}
	return claims, nil
}

func validateGatewayServiceAccountClaims(claims map[string]any, account *sdktypes.OpenShellGatewayServiceAccountCreateResponse) error {
	if claims["iss"] != account.Credential.Issuer || claims["sub"] != account.Subject {
		return fmt.Errorf("issuer or subject does not match the created service account")
	}
	var audiences []string
	switch value := claims["aud"].(type) {
	case string:
		audiences = []string{value}
	case []any:
		for _, item := range value {
			if audience, ok := item.(string); ok {
				audiences = append(audiences, audience)
			}
		}
	}
	if len(audiences) != 1 || audiences[0] != account.Credential.Audience {
		return fmt.Errorf("audience %v does not match %q", audiences, account.Credential.Audience)
	}
	hypershell, _ := claims["hypershell"].(map[string]any)
	rolesValue, _ := hypershell["roles"].([]any)
	roles := map[string]bool{}
	for _, value := range rolesValue {
		if role, ok := value.(string); ok {
			roles[role] = true
		}
	}
	if len(roles) != 2 || !roles["openshell-admin"] || !roles["openshell-user"] {
		return fmt.Errorf("roles %v do not contain exactly openshell-admin and openshell-user", roles)
	}
	expiresAt, ok := claims["exp"].(float64)
	if !ok || expiresAt <= float64(time.Now().Unix()+30) || expiresAt > float64(time.Now().Unix()+330) {
		return fmt.Errorf("token expiration %v is outside the expected lifetime", claims["exp"])
	}
	return nil
}
