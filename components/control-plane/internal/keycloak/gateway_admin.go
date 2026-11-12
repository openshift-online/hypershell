package keycloak

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// gatewayProvisionerUsername is the managed realm user the control plane
// authenticates as to drive a gateway's OpenShell provisioning API (workspace +
// membership) as an openshell-admin (AGT-WSP-01: admin OIDC service-account
// identity). The control plane ensures it exists, grants it the target gateway's
// openshell-admin client role, sets a fresh random password, and mints a token via
// the gateway client's direct-access (ROPC) grant. No password is persisted.
const gatewayProvisionerUsername = "hypershell-gateway-provisioner"

// MintGatewayAdminToken returns a short-lived OpenShell access token scoped to the
// gateway (audience == gatewayClientID) carrying the openshell-admin role, so the
// control plane can call the gateway's provisioning API as an admin. The gateway
// Keycloak client is public with directAccessGrantsEnabled and already carries the
// hypershell.roles protocol mapper, so this minted token validates at the gateway
// exactly like an interactive admin login. The admin password is set fresh per
// mint and never logged or stored.
func (c *Client) MintGatewayAdminToken(ctx context.Context, gatewayClientID string) (string, error) {
	if gatewayClientID == "" {
		return "", fmt.Errorf("empty gateway client id")
	}
	if err := c.ensureGatewayProvisionerUser(ctx); err != nil {
		return "", fmt.Errorf("ensure gateway provisioner user: %w", err)
	}
	// Keycloak role-mapping POST is idempotent (re-adding an existing mapping is a
	// no-op 204), so this is safe to call on every mint.
	if err := c.AssignClientRole(ctx, gatewayClientID, gatewayProvisionerUsername, "openshell-admin"); err != nil {
		return "", fmt.Errorf("grant openshell-admin to provisioner on %s: %w", gatewayClientID, err)
	}
	password, err := c.resetGatewayProvisionerPassword(ctx)
	if err != nil {
		return "", fmt.Errorf("set provisioner password: %w", err)
	}
	token, err := c.passwordGrant(ctx, gatewayClientID, gatewayProvisionerUsername, password)
	if err != nil {
		return "", fmt.Errorf("mint admin token for gateway %s: %w", gatewayClientID, err)
	}
	slog.InfoContext(ctx, "Minted gateway admin token for workspace provisioning", "gateway_client_id", gatewayClientID)
	return token, nil
}

// ensureGatewayProvisionerUser upserts the managed provisioner user with a complete
// profile and no pending required actions, so the direct-access grant is not
// rejected with "Account is not fully set up".
func (c *Client) ensureGatewayProvisionerUser(ctx context.Context) error {
	body, _ := json.Marshal(map[string]any{
		"username":        gatewayProvisionerUsername,
		"enabled":         true,
		"emailVerified":   true,
		"email":           gatewayProvisionerUsername + "@hypershell.local",
		"firstName":       "HyperShell",
		"lastName":        "Gateway Provisioner",
		"requiredActions": []string{},
	})

	uuid, err := c.getUserUUID(ctx, gatewayProvisionerUsername)
	if err != nil {
		var notFound *UserNotFoundError
		if !errors.As(err, &notFound) {
			return err
		}
		path := fmt.Sprintf("/admin/realms/%s/users", c.realm)
		if _, err := c.doRequest(ctx, http.MethodPost, path, body); err != nil {
			return fmt.Errorf("create provisioner user: %w", err)
		}
		return nil
	}
	// Exists: converge the profile so a user left half-configured by an earlier
	// partial run still mints.
	path := fmt.Sprintf("/admin/realms/%s/users/%s", c.realm, uuid)
	if _, err := c.doRequest(ctx, http.MethodPut, path, body); err != nil {
		return fmt.Errorf("update provisioner user: %w", err)
	}
	return nil
}

// resetGatewayProvisionerPassword sets a fresh random, non-temporary password on
// the provisioner user and returns it. The password lives only in memory for the
// immediately following token mint.
func (c *Client) resetGatewayProvisionerPassword(ctx context.Context) (string, error) {
	uuid, err := c.getUserUUID(ctx, gatewayProvisionerUsername)
	if err != nil {
		return "", err
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	// Mixed classes satisfy any realm password policy.
	password := "Aa1!" + hex.EncodeToString(buf)
	body, _ := json.Marshal(map[string]any{"type": "password", "value": password, "temporary": false})
	path := fmt.Sprintf("/admin/realms/%s/users/%s/reset-password", c.realm, uuid)
	if _, err := c.doRequest(ctx, http.MethodPut, path, body); err != nil {
		return "", err
	}
	return password, nil
}

// passwordGrant performs an OAuth2 resource-owner-password-credentials grant
// against the given (public, direct-access-enabled) client and returns the access
// token. Used only for the gateway client to mint an admin token for the managed
// provisioner user.
func (c *Client) passwordGrant(ctx context.Context, clientID, username, password string) (string, error) {
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", c.serverURL, c.realm)
	data := url.Values{
		"grant_type": {"password"},
		"client_id":  {clientID},
		"username":   {username},
		"password":   {password},
		"scope":      {"openid"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		// Do not include the body verbatim: it may echo request params.
		return "", fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(respBody, &tok); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("empty access token from keycloak")
	}
	return tok.AccessToken, nil
}

// ListGroupMemberSubjects returns the OIDC subjects (Keycloak user ids, which are
// the token `sub`) of every member of the named top-level group. It is used to
// converge a CITrust's per-trust group membership onto the gateway workspace: the
// group's members are the brokered CI identities that matched the trust. A missing
// group yields an empty list (the group is created lazily on first realization and
// may hold no members yet).
func (c *Client) ListGroupMemberSubjects(ctx context.Context, groupName string) ([]string, error) {
	groupID, err := c.findGroupIDByName(ctx, groupName)
	if err != nil {
		return nil, err
	}
	if groupID == "" {
		return nil, nil
	}
	var subjects []string
	first := 0
	const pageSize = 100
	for {
		path := fmt.Sprintf("/admin/realms/%s/groups/%s/members?first=%d&max=%d", c.realm, groupID, first, pageSize)
		respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("list members of group %q: %w", groupName, err)
		}
		var users []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(respBody, &users); err != nil {
			return nil, fmt.Errorf("parse group members: %w", err)
		}
		for _, u := range users {
			if u.ID != "" {
				subjects = append(subjects, u.ID)
			}
		}
		if len(users) < pageSize {
			break
		}
		first += pageSize
	}
	return subjects, nil
}

// findGroupIDByName resolves a top-level group id by exact name, or "" when absent.
func (c *Client) findGroupIDByName(ctx context.Context, name string) (string, error) {
	path := fmt.Sprintf("/admin/realms/%s/groups?search=%s&max=100", c.realm, url.QueryEscape(name))
	respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("search group %q: %w", name, err)
	}
	var groups []keycloakGroup
	if err := json.Unmarshal(respBody, &groups); err != nil {
		return "", fmt.Errorf("parse group search: %w", err)
	}
	for _, g := range groups {
		if g.Name == name {
			return g.ID, nil
		}
	}
	return "", nil
}
