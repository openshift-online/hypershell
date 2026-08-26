package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const deviceAuthorizationGrantAttribute = "oauth2.device.authorization.grant.enabled"

// Client wraps the Keycloak Admin REST API for gateway OIDC provisioning.
type Client struct {
	serverURL    string
	realm        string
	clientID     string
	clientSecret string

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
	httpClient  *http.Client
}

// NewClient creates a Keycloak Admin REST API client that authenticates
// using the client_credentials grant.
func NewClient(serverURL, realm, clientID, clientSecret string) *Client {
	return &Client{
		serverURL:    strings.TrimRight(serverURL, "/"),
		realm:        realm,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Issuer returns the OIDC issuer URL for the configured realm.
func (c *Client) Issuer() string {
	return fmt.Sprintf("%s/realms/%s", c.serverURL, c.realm)
}

// Realm returns the configured realm name.
func (c *Client) Realm() string {
	return c.realm
}

type keycloakClient struct {
	ID         string            `json:"id,omitempty"`
	ClientID   string            `json:"clientId"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type keycloakRole struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

type keycloakUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// ClientNotFoundError is returned when a Keycloak client lookup finds no
// matching clientId. The RoleBindingReconciler uses this to distinguish the
// expected race (client not yet provisioned) from permanent failures.
type ClientNotFoundError struct {
	ClientID string
}

func (e *ClientNotFoundError) Error() string {
	return fmt.Sprintf("keycloak client %s not found", e.ClientID)
}

// ProvisionGatewayClient creates a Keycloak OIDC client for a gateway with
// roles and protocol mappers. Returns the Keycloak-internal client UUID.
func (c *Client) ProvisionGatewayClient(ctx context.Context, gatewayName string) (string, error) {
	log.Printf("INFO keycloak: creating OIDC client %s in realm %s", gatewayName, c.realm)
	clientUUID, err := c.createClient(ctx, gatewayName)
	if err != nil {
		return "", fmt.Errorf("create keycloak client: %w", err)
	}
	log.Printf("INFO keycloak: created client %s (uuid=%s)", gatewayName, clientUUID)

	log.Printf("INFO keycloak: creating roles [openshell-admin, openshell-user] on client %s", gatewayName)
	if err := c.createClientRoles(ctx, clientUUID); err != nil {
		log.Printf("WARN keycloak: role creation failed for %s, rolling back client: %v", gatewayName, err)
		if rollbackErr := c.deleteClientByUUID(ctx, clientUUID); rollbackErr != nil {
			log.Printf("WARN keycloak: failed to rollback client %s after role creation failure: %v", gatewayName, rollbackErr)
		}
		return "", fmt.Errorf("create client roles: %w", err)
	}
	log.Printf("INFO keycloak: created roles on client %s", gatewayName)

	log.Printf("INFO keycloak: creating protocol mappers on client %s", gatewayName)
	if err := c.createProtocolMappers(ctx, clientUUID, gatewayName); err != nil {
		log.Printf("WARN keycloak: protocol mapper creation failed for %s, rolling back client: %v", gatewayName, err)
		if rollbackErr := c.deleteClientByUUID(ctx, clientUUID); rollbackErr != nil {
			log.Printf("WARN keycloak: failed to rollback client %s after mapper creation failure: %v", gatewayName, rollbackErr)
		}
		return "", fmt.Errorf("create protocol mappers: %w", err)
	}
	log.Printf("INFO keycloak: created protocol mappers on client %s", gatewayName)

	return clientUUID, nil
}

// DeleteGatewayClient removes the Keycloak client for a gateway.
// Returns nil if the client does not exist.
func (c *Client) DeleteGatewayClient(ctx context.Context, gatewayName string) error {
	log.Printf("INFO keycloak: looking up client %s for deletion", gatewayName)
	clientUUID, err := c.getClientUUID(ctx, gatewayName)
	if err != nil {
		return err
	}
	if clientUUID == "" {
		log.Printf("INFO keycloak: client %s not found, nothing to delete", gatewayName)
		return nil
	}
	log.Printf("INFO keycloak: deleting client %s (uuid=%s)", gatewayName, clientUUID)
	if err := c.deleteClientByUUID(ctx, clientUUID); err != nil {
		return err
	}
	log.Printf("INFO keycloak: deleted client %s", gatewayName)
	return nil
}

// DeleteGatewayServiceAccountClients disables and removes every
// HyperShell-managed service-account client for a gateway. Selection uses the
// immutable gateway attribute, so it also removes clients orphaned from the API
// database. All clients are disabled before any are deleted.
func (c *Client) DeleteGatewayServiceAccountClients(ctx context.Context, gatewayID string) error {
	const pageSize = 100
	listed := make([]keycloakClient, 0)
	for first := 0; ; first += pageSize {
		path := fmt.Sprintf("/admin/realms/%s/clients?first=%d&max=%d", c.realm, first, pageSize)
		response, err := c.doRequest(ctx, http.MethodGet, path, nil)
		if err != nil {
			return errors.New("list Keycloak clients")
		}
		var page []keycloakClient
		if err := json.Unmarshal(response, &page); err != nil {
			return fmt.Errorf("parse keycloak client list: %w", err)
		}
		listed = append(listed, page...)
		if len(page) < pageSize {
			break
		}
	}
	managed := make([]string, 0)
	for _, item := range listed {
		clientPath := fmt.Sprintf("/admin/realms/%s/clients/%s", c.realm, url.PathEscape(item.ID))
		body, getErr := c.doRequest(ctx, http.MethodGet, clientPath, nil)
		if getErr != nil {
			return errors.New("inspect Keycloak client")
		}
		var representation map[string]json.RawMessage
		if err := json.Unmarshal(body, &representation); err != nil {
			return fmt.Errorf("parse keycloak client: %w", err)
		}
		var attributes map[string]string
		if raw := representation["attributes"]; len(raw) != 0 {
			_ = json.Unmarshal(raw, &attributes)
		}
		if attributes["hypershell.service-account"] != "true" || attributes["hypershell.gateway-id"] != gatewayID {
			continue
		}
		representation["enabled"], _ = json.Marshal(false)
		payload, _ := json.Marshal(representation)
		if _, putErr := c.doRequest(ctx, http.MethodPut, clientPath, payload); putErr != nil {
			return errors.New("disable gateway service-account client")
		}
		managed = append(managed, item.ID)
	}
	for _, clientUUID := range managed {
		if err := c.deleteClientByUUID(ctx, clientUUID); err != nil {
			return errors.New("delete gateway service-account client")
		}
	}
	return nil
}

// AssignClientRole assigns a Keycloak client role to a user on a gateway.
func (c *Client) AssignClientRole(ctx context.Context, gatewayName, username, roleName string) error {
	log.Printf("INFO keycloak: assigning role %s to user %s on client %s", roleName, username, gatewayName)

	log.Printf("INFO keycloak: resolving client UUID for %s", gatewayName)
	clientUUID, err := c.getClientUUID(ctx, gatewayName)
	if err != nil {
		return fmt.Errorf("get client UUID for %s: %w", gatewayName, err)
	}
	if clientUUID == "" {
		return &ClientNotFoundError{ClientID: gatewayName}
	}
	log.Printf("INFO keycloak: resolved client %s to uuid=%s", gatewayName, clientUUID)

	log.Printf("INFO keycloak: resolving role %s on client %s", roleName, gatewayName)
	roleUUID, err := c.getClientRoleUUID(ctx, clientUUID, roleName)
	if err != nil {
		return fmt.Errorf("get role UUID for %s on %s: %w", roleName, gatewayName, err)
	}
	log.Printf("INFO keycloak: resolved role %s to uuid=%s", roleName, roleUUID)

	log.Printf("INFO keycloak: resolving user %s", username)
	userUUID, err := c.getUserUUID(ctx, username)
	if err != nil {
		return fmt.Errorf("get user UUID for %s: %w", username, err)
	}
	log.Printf("INFO keycloak: resolved user %s to uuid=%s", username, userUUID)

	roles := []keycloakRole{{ID: roleUUID, Name: roleName}}
	body, _ := json.Marshal(roles)

	path := fmt.Sprintf("/admin/realms/%s/users/%s/role-mappings/clients/%s",
		c.realm, userUUID, clientUUID)
	_, err = c.doRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return fmt.Errorf("assign role %s to %s on %s: %w", roleName, username, gatewayName, err)
	}

	log.Printf("INFO keycloak: assigned role %s to user %s on client %s", roleName, username, gatewayName)
	return nil
}

// RemoveClientRole removes a Keycloak client role from a user on a gateway.
func (c *Client) RemoveClientRole(ctx context.Context, gatewayName, username, roleName string) error {
	log.Printf("INFO keycloak: removing role %s from user %s on client %s", roleName, username, gatewayName)

	clientUUID, err := c.getClientUUID(ctx, gatewayName)
	if err != nil {
		return fmt.Errorf("get client UUID for %s: %w", gatewayName, err)
	}
	if clientUUID == "" {
		log.Printf("INFO keycloak: client %s not found, nothing to remove", gatewayName)
		return nil
	}

	roleUUID, err := c.getClientRoleUUID(ctx, clientUUID, roleName)
	if err != nil {
		return fmt.Errorf("get role UUID for %s on %s: %w", roleName, gatewayName, err)
	}

	userUUID, err := c.getUserUUID(ctx, username)
	if err != nil {
		return fmt.Errorf("get user UUID for %s: %w", username, err)
	}

	roles := []keycloakRole{{ID: roleUUID, Name: roleName}}
	body, _ := json.Marshal(roles)

	path := fmt.Sprintf("/admin/realms/%s/users/%s/role-mappings/clients/%s",
		c.realm, userUUID, clientUUID)
	_, err = c.doRequest(ctx, http.MethodDelete, path, body)
	if err != nil {
		return fmt.Errorf("remove role %s from %s on %s: %w", roleName, username, gatewayName, err)
	}

	log.Printf("INFO keycloak: removed role %s from user %s on client %s", roleName, username, gatewayName)
	return nil
}

// GetClientUUID returns the Keycloak-internal UUID for a client by clientId,
// or empty string if not found.
func (c *Client) GetClientUUID(ctx context.Context, gatewayName string) (string, error) {
	uuid, err := c.getClientUUID(ctx, gatewayName)
	if err != nil {
		log.Printf("WARN keycloak: failed to look up client %s: %v", gatewayName, err)
		return "", err
	}
	if uuid == "" {
		log.Printf("INFO keycloak: client %s not found in realm %s", gatewayName, c.realm)
	} else {
		log.Printf("INFO keycloak: client %s found (uuid=%s)", gatewayName, uuid)
	}
	return uuid, nil
}

// GetConsoleClientSecret returns the client secret for an existing console
// client by clientId. The console reconciler calls it when the console client
// already exists and the secret must be written into the console Secret again.
func (c *Client) GetConsoleClientSecret(ctx context.Context, consoleClientID string) (string, error) {
	clientUUID, err := c.getClientUUID(ctx, consoleClientID)
	if err != nil {
		return "", err
	}
	if clientUUID == "" {
		return "", &ClientNotFoundError{ClientID: consoleClientID}
	}
	return c.getClientSecret(ctx, clientUUID)
}

func (c *Client) createClient(ctx context.Context, gatewayName string) (string, error) {
	payload := map[string]interface{}{
		"clientId":                  gatewayName,
		"name":                      gatewayName,
		"publicClient":              true,
		"standardFlowEnabled":       true,
		"directAccessGrantsEnabled": true,
		"fullScopeAllowed":          false,
		"redirectUris":              []string{"http://127.0.0.1:*", "http://localhost:*"},
		"attributes": map[string]string{
			"pkce.code.challenge.method":      "S256",
			deviceAuthorizationGrantAttribute: "true",
		},
		"defaultClientScopes": []string{
			"profile", "email", "roles", "web-origins", "acr",
		},
		"protocolMappers": []map[string]interface{}{
			{
				"name":           "audience",
				"protocol":       "openid-connect",
				"protocolMapper": "oidc-audience-mapper",
				"config": map[string]string{
					"included.client.audience": gatewayName,
					"id.token.claim":           "false",
					"access.token.claim":       "true",
				},
			},
		},
	}

	body, _ := json.Marshal(payload)
	path := fmt.Sprintf("/admin/realms/%s/clients", c.realm)
	resp, err := c.doRequestRaw(ctx, http.MethodPost, path, body)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusConflict {
		return "", fmt.Errorf("keycloak client %s already exists", gatewayName)
	}
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create client returned %d: %s", resp.StatusCode, string(respBody))
	}

	location := resp.Header.Get("Location")
	parts := strings.Split(location, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("no client UUID in Location header")
	}
	return parts[len(parts)-1], nil
}

// EnsureDeviceAuthorizationGrant enables OAuth 2.0 Device Authorization Grant
// on an existing Keycloak client. The full representation is fetched and
// updated so unrelated client settings and attributes are preserved.
func (c *Client) EnsureDeviceAuthorizationGrant(ctx context.Context, clientUUID string) error {
	path := fmt.Sprintf("/admin/realms/%s/clients/%s", c.realm, clientUUID)
	respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return fmt.Errorf("get keycloak client %s: %w", clientUUID, err)
	}

	var representation map[string]json.RawMessage
	if err := json.Unmarshal(respBody, &representation); err != nil {
		return fmt.Errorf("parse keycloak client %s: %w", clientUUID, err)
	}

	attributes := make(map[string]string)
	if rawAttributes, ok := representation["attributes"]; ok &&
		len(rawAttributes) > 0 && !bytes.Equal(bytes.TrimSpace(rawAttributes), []byte("null")) {
		if err := json.Unmarshal(rawAttributes, &attributes); err != nil {
			return fmt.Errorf("parse attributes for keycloak client %s: %w", clientUUID, err)
		}
	}

	if attributes[deviceAuthorizationGrantAttribute] == "true" {
		return nil
	}
	attributes[deviceAuthorizationGrantAttribute] = "true"

	rawAttributes, err := json.Marshal(attributes)
	if err != nil {
		return fmt.Errorf("marshal attributes for keycloak client %s: %w", clientUUID, err)
	}
	representation["attributes"] = rawAttributes

	body, err := json.Marshal(representation)
	if err != nil {
		return fmt.Errorf("marshal keycloak client %s: %w", clientUUID, err)
	}
	if _, err := c.doRequest(ctx, http.MethodPut, path, body); err != nil {
		return fmt.Errorf("enable device authorization grant on keycloak client %s: %w", clientUUID, err)
	}
	return nil
}

func (c *Client) createClientRoles(ctx context.Context, clientUUID string) error {
	for _, roleName := range []string{"openshell-admin", "openshell-user"} {
		role := keycloakRole{Name: roleName}
		body, _ := json.Marshal(role)
		path := fmt.Sprintf("/admin/realms/%s/clients/%s/roles", c.realm, clientUUID)
		if _, err := c.doRequest(ctx, http.MethodPost, path, body); err != nil {
			return fmt.Errorf("create role %s: %w", roleName, err)
		}
	}
	return nil
}

func (c *Client) createProtocolMappers(ctx context.Context, clientUUID, gatewayName string) error {
	mappers := []map[string]interface{}{
		{
			"name":           "sub",
			"protocol":       "openid-connect",
			"protocolMapper": "oidc-sub-mapper",
			"config": map[string]string{
				"access.token.claim": "true",
			},
		},
		{
			"name":           "client-roles",
			"protocol":       "openid-connect",
			"protocolMapper": "oidc-usermodel-client-role-mapper",
			"config": map[string]string{
				"claim.name":                           "hypershell.roles",
				"multivalued":                          "true",
				"jsonType.label":                       "String",
				"id.token.claim":                       "true",
				"access.token.claim":                   "true",
				"usermodel.clientRoleMapping.clientId": gatewayName,
			},
		},
	}

	path := fmt.Sprintf("/admin/realms/%s/clients/%s/protocol-mappers/models", c.realm, clientUUID)
	for _, mapper := range mappers {
		body, _ := json.Marshal(mapper)
		if _, err := c.doRequest(ctx, http.MethodPost, path, body); err != nil {
			return fmt.Errorf("create mapper %s: %w", mapper["name"], err)
		}
	}
	return nil
}

func (c *Client) getClientUUID(ctx context.Context, clientID string) (string, error) {
	path := fmt.Sprintf("/admin/realms/%s/clients?clientId=%s", c.realm, url.QueryEscape(clientID))
	respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}

	var clients []keycloakClient
	if err := json.Unmarshal(respBody, &clients); err != nil {
		return "", fmt.Errorf("parse client list: %w", err)
	}
	for _, kc := range clients {
		if kc.ClientID == clientID {
			return kc.ID, nil
		}
	}
	return "", nil
}

func (c *Client) listClientRoles(ctx context.Context, clientUUID string) ([]keycloakRole, error) {
	path := fmt.Sprintf("/admin/realms/%s/clients/%s/roles", c.realm, clientUUID)
	respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var roles []keycloakRole
	if err := json.Unmarshal(respBody, &roles); err != nil {
		return nil, fmt.Errorf("parse client roles: %w", err)
	}
	return roles, nil
}

// EnsureConsoleClientConfig reconciles the full desired configuration of an
// existing console client so drift is corrected on every reconcile rather than
// only at creation. It updates the redirect URIs and web origins (which change
// when the console host / base domain changes -- a stale value breaks the OIDC
// redirect and CORS), re-asserts fullScopeAllowed=false and the confidential
// standard-flow flags (per-gateway isolation depends on fullScopeAllowed being
// false), upserts the protocol mappers (audience, client-roles, sub), and
// re-grants the gateway-client scope mappings. It is idempotent and safe to run
// each pass.
func (c *Client) EnsureConsoleClientConfig(ctx context.Context, consoleClientID, gatewayClientID, redirectURI, webOrigin string) error {
	consoleUUID, err := c.getClientUUID(ctx, consoleClientID)
	if err != nil {
		return fmt.Errorf("resolve console client %s: %w", consoleClientID, err)
	}
	if consoleUUID == "" {
		return fmt.Errorf("console client %s not found", consoleClientID)
	}
	if err := c.updateConsoleClientRepresentation(ctx, consoleUUID, consoleClientID, redirectURI, webOrigin); err != nil {
		return err
	}
	if err := c.ensureConsoleProtocolMappers(ctx, consoleUUID, gatewayClientID); err != nil {
		return err
	}
	return c.addConsoleScopeMappings(ctx, consoleUUID, gatewayClientID)
}

// updateConsoleClientRepresentation GET-merges the desired config fields onto the
// existing console client representation and PUTs it back, so fields Keycloak
// manages outside this set (e.g. the client secret) are preserved while the
// console-owned settings are reconciled to their desired values.
func (c *Client) updateConsoleClientRepresentation(ctx context.Context, consoleUUID, consoleClientID, redirectURI, webOrigin string) error {
	path := fmt.Sprintf("/admin/realms/%s/clients/%s", c.realm, consoleUUID)
	respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return fmt.Errorf("get console client %s: %w", consoleClientID, err)
	}
	var rep map[string]interface{}
	if err := json.Unmarshal(respBody, &rep); err != nil {
		return fmt.Errorf("parse console client %s: %w", consoleClientID, err)
	}

	rep["publicClient"] = false
	rep["standardFlowEnabled"] = true
	rep["directAccessGrantsEnabled"] = false
	rep["serviceAccountsEnabled"] = false
	rep["fullScopeAllowed"] = false
	rep["redirectUris"] = []string{redirectURI}
	rep["webOrigins"] = []string{webOrigin}

	attrs, _ := rep["attributes"].(map[string]interface{})
	if attrs == nil {
		attrs = map[string]interface{}{}
	}
	attrs["pkce.code.challenge.method"] = "S256"
	rep["attributes"] = attrs
	rep["defaultClientScopes"] = []string{
		"profile", "email", "roles", "web-origins", "acr",
	}

	body, err := json.Marshal(rep)
	if err != nil {
		return fmt.Errorf("marshal console client %s: %w", consoleClientID, err)
	}
	if _, err := c.doRequest(ctx, http.MethodPut, path, body); err != nil {
		return fmt.Errorf("update console client %s: %w", consoleClientID, err)
	}
	return nil
}

// addConsoleScopeMappings grants the console client (by UUID) scope for every
// role defined on the gateway client. With fullScopeAllowed=false (required for
// per-gateway isolation), Keycloak filters every role mapper's output to the
// requesting client's scope, so without this grant the console access token
// omits hypershell.roles entirely and the gateway denies every request with
// "role 'openshell-user' required". Idempotent (re-POSTing existing scope
// mappings is a Keycloak no-op) and grants only this one gateway client's roles,
// so isolation is preserved. See EnsureConsoleClientConfig / ProvisionConsoleClient.
func (c *Client) addConsoleScopeMappings(ctx context.Context, consoleUUID, gatewayClientID string) error {
	gatewayUUID, err := c.getClientUUID(ctx, gatewayClientID)
	if err != nil {
		return fmt.Errorf("resolve gateway client %s: %w", gatewayClientID, err)
	}
	if gatewayUUID == "" {
		return fmt.Errorf("gateway client %s not found", gatewayClientID)
	}
	roles, err := c.listClientRoles(ctx, gatewayUUID)
	if err != nil {
		return fmt.Errorf("list roles on gateway client %s: %w", gatewayClientID, err)
	}
	if len(roles) == 0 {
		return nil
	}
	body, err := json.Marshal(roles)
	if err != nil {
		return fmt.Errorf("marshal scope mapping roles: %w", err)
	}
	path := fmt.Sprintf("/admin/realms/%s/clients/%s/scope-mappings/clients/%s", c.realm, consoleUUID, gatewayUUID)
	if _, err := c.doRequest(ctx, http.MethodPost, path, body); err != nil {
		return fmt.Errorf("add console scope mappings from gateway client %s: %w", gatewayClientID, err)
	}
	return nil
}

func (c *Client) getClientRoleUUID(ctx context.Context, clientUUID, roleName string) (string, error) {
	path := fmt.Sprintf("/admin/realms/%s/clients/%s/roles/%s", c.realm, clientUUID, url.PathEscape(roleName))
	respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}

	var role keycloakRole
	if err := json.Unmarshal(respBody, &role); err != nil {
		return "", fmt.Errorf("parse role: %w", err)
	}
	return role.ID, nil
}

func (c *Client) getUserUUID(ctx context.Context, username string) (string, error) {
	path := fmt.Sprintf("/admin/realms/%s/users?username=%s&exact=true", c.realm, url.QueryEscape(username))
	respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}

	var users []keycloakUser
	if err := json.Unmarshal(respBody, &users); err != nil {
		return "", fmt.Errorf("parse user list: %w", err)
	}
	for _, u := range users {
		if u.Username == username {
			return u.ID, nil
		}
	}
	return "", fmt.Errorf("keycloak user %s not found", username)
}

func (c *Client) deleteClientByUUID(ctx context.Context, clientUUID string) error {
	path := fmt.Sprintf("/admin/realms/%s/clients/%s", c.realm, clientUUID)
	_, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	return err
}

func (c *Client) ensureToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		return nil
	}

	log.Printf("INFO keycloak: acquiring admin token from %s/realms/%s (client_id=%s)", c.serverURL, c.realm, c.clientID)
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", c.serverURL, c.realm)
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return fmt.Errorf("parse token response: %w", err)
	}
	if tok.AccessToken == "" {
		return fmt.Errorf("empty access token from keycloak")
	}
	if tok.ExpiresIn <= 0 {
		tok.ExpiresIn = 300
	}

	c.token = tok.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(float64(tok.ExpiresIn)*0.8) * time.Second)
	log.Printf("INFO keycloak: acquired admin token (expires_in=%ds)", tok.ExpiresIn)
	return nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	resp, err := c.doRequestRaw(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("keycloak %s %s returned %d: %s", method, path, resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func (c *Client) doRequestRaw(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("authenticate to keycloak: %w", err)
	}

	fullURL := c.serverURL + path
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

// ProvisionConsoleClient creates a confidential Keycloak OIDC client for the
// web console, wires it to the given gateway client for audience and role
// mappers, and returns the new client's UUID and generated client secret.
func (c *Client) ProvisionConsoleClient(ctx context.Context, consoleClientID, gatewayClientID, redirectURI, webOrigin string) (clientUUID string, clientSecret string, err error) {
	log.Printf("INFO keycloak: creating confidential console client %s in realm %s", consoleClientID, c.realm)
	clientUUID, err = c.createConsoleClient(ctx, consoleClientID, gatewayClientID, redirectURI, webOrigin)
	if err != nil {
		return "", "", fmt.Errorf("create console keycloak client: %w", err)
	}
	log.Printf("INFO keycloak: created console client %s (uuid=%s)", consoleClientID, clientUUID)

	log.Printf("INFO keycloak: creating protocol mappers on console client %s", consoleClientID)
	if err = c.createConsoleProtocolMappers(ctx, clientUUID, gatewayClientID); err != nil {
		log.Printf("WARN keycloak: protocol mapper creation failed for %s, rolling back client: %v", consoleClientID, err)
		if rollbackErr := c.deleteClientByUUID(ctx, clientUUID); rollbackErr != nil {
			log.Printf("WARN keycloak: failed to rollback console client %s after mapper creation failure: %v", consoleClientID, rollbackErr)
		}
		return "", "", fmt.Errorf("create console protocol mappers: %w", err)
	}
	log.Printf("INFO keycloak: created protocol mappers on console client %s", consoleClientID)

	log.Printf("INFO keycloak: granting console client %s scope for gateway client %s roles", consoleClientID, gatewayClientID)
	if err = c.addConsoleScopeMappings(ctx, clientUUID, gatewayClientID); err != nil {
		log.Printf("WARN keycloak: scope mapping failed for %s, rolling back client: %v", consoleClientID, err)
		if rollbackErr := c.deleteClientByUUID(ctx, clientUUID); rollbackErr != nil {
			log.Printf("WARN keycloak: failed to rollback console client %s after scope mapping failure: %v", consoleClientID, rollbackErr)
		}
		return "", "", fmt.Errorf("grant console scope mappings: %w", err)
	}
	log.Printf("INFO keycloak: granted console client %s scope for gateway client %s roles", consoleClientID, gatewayClientID)

	log.Printf("INFO keycloak: fetching client secret for console client %s", consoleClientID)
	clientSecret, err = c.getClientSecret(ctx, clientUUID)
	if err != nil {
		log.Printf("WARN keycloak: secret fetch failed for %s, rolling back client: %v", consoleClientID, err)
		if rollbackErr := c.deleteClientByUUID(ctx, clientUUID); rollbackErr != nil {
			log.Printf("WARN keycloak: failed to rollback console client %s after secret fetch failure: %v", consoleClientID, rollbackErr)
		}
		return "", "", fmt.Errorf("get console client secret: %w", err)
	}
	log.Printf("INFO keycloak: provisioned console client %s (uuid=%s)", consoleClientID, clientUUID)

	return clientUUID, clientSecret, nil
}

// DeleteConsoleClient removes the Keycloak client for a web console.
// Returns nil if the client does not exist.
func (c *Client) DeleteConsoleClient(ctx context.Context, consoleClientID string) error {
	log.Printf("INFO keycloak: looking up console client %s for deletion", consoleClientID)
	clientUUID, err := c.getClientUUID(ctx, consoleClientID)
	if err != nil {
		return err
	}
	if clientUUID == "" {
		log.Printf("INFO keycloak: console client %s not found, nothing to delete", consoleClientID)
		return nil
	}
	log.Printf("INFO keycloak: deleting console client %s (uuid=%s)", consoleClientID, clientUUID)
	if err := c.deleteClientByUUID(ctx, clientUUID); err != nil {
		return err
	}
	log.Printf("INFO keycloak: deleted console client %s", consoleClientID)
	return nil
}

// ConsoleClientExists reports whether a Keycloak console client with the given
// clientId currently exists in the realm. It underpins the health loop's
// converged teardown-settled check: the console client lives in the realm, not
// the gateway namespace, so a Kubernetes-only absence probe cannot observe a
// client a stale provisioning pass recreated. Returns an error when existence
// cannot be observed so callers treat unknown state as "not absent" (re-run
// teardown) rather than as settled.
func (c *Client) ConsoleClientExists(ctx context.Context, consoleClientID string) (bool, error) {
	clientUUID, err := c.getClientUUID(ctx, consoleClientID)
	if err != nil {
		return false, err
	}
	return clientUUID != "", nil
}

func (c *Client) createConsoleClient(ctx context.Context, consoleClientID, gatewayClientID, redirectURI, webOrigin string) (string, error) {
	payload := map[string]interface{}{
		"clientId":                  consoleClientID,
		"name":                      consoleClientID,
		"publicClient":              false,
		"standardFlowEnabled":       true,
		"directAccessGrantsEnabled": false,
		"serviceAccountsEnabled":    false,
		"fullScopeAllowed":          false,
		"redirectUris":              []string{redirectURI},
		"webOrigins":                []string{webOrigin},
		"attributes": map[string]string{
			"pkce.code.challenge.method": "S256",
		},
		"defaultClientScopes": []string{
			"profile", "email", "roles", "web-origins", "acr",
		},
	}

	body, _ := json.Marshal(payload)
	path := fmt.Sprintf("/admin/realms/%s/clients", c.realm)
	resp, err := c.doRequestRaw(ctx, http.MethodPost, path, body)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusConflict {
		return "", fmt.Errorf("keycloak client %s already exists", consoleClientID)
	}
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create console client returned %d: %s", resp.StatusCode, string(respBody))
	}

	location := resp.Header.Get("Location")
	parts := strings.Split(location, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("no client UUID in Location header")
	}
	return parts[len(parts)-1], nil
}

// desiredConsoleProtocolMappers is the canonical set of protocol mappers a
// console client must carry: the audience mapper (so the gateway accepts the
// token), the client-roles mapper (so hypershell.roles is populated), and the
// sub mapper. It is the single source of truth shared by the create and the
// reconcile (upsert) paths.
func desiredConsoleProtocolMappers(gatewayClientID string) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":           "audience",
			"protocol":       "openid-connect",
			"protocolMapper": "oidc-audience-mapper",
			"config": map[string]string{
				"included.client.audience": gatewayClientID,
				"id.token.claim":           "false",
				"access.token.claim":       "true",
			},
		},
		{
			"name":           "client-roles",
			"protocol":       "openid-connect",
			"protocolMapper": "oidc-usermodel-client-role-mapper",
			"config": map[string]string{
				"claim.name":                           "hypershell.roles",
				"multivalued":                          "true",
				"jsonType.label":                       "String",
				"id.token.claim":                       "true",
				"access.token.claim":                   "true",
				"usermodel.clientRoleMapping.clientId": gatewayClientID,
			},
		},
		{
			"name":           "sub",
			"protocol":       "openid-connect",
			"protocolMapper": "oidc-sub-mapper",
			"config": map[string]string{
				"access.token.claim": "true",
			},
		},
	}
}

func (c *Client) createConsoleProtocolMappers(ctx context.Context, clientUUID, gatewayClientID string) error {
	path := fmt.Sprintf("/admin/realms/%s/clients/%s/protocol-mappers/models", c.realm, clientUUID)
	for _, mapper := range desiredConsoleProtocolMappers(gatewayClientID) {
		body, _ := json.Marshal(mapper)
		if _, err := c.doRequest(ctx, http.MethodPost, path, body); err != nil {
			return fmt.Errorf("create console mapper %s: %w", mapper["name"], err)
		}
	}
	return nil
}

// ensureConsoleProtocolMappers upserts the desired protocol mappers on an
// existing console client: a mapper missing by name is created, and one present
// is updated in place so a stale audience or client-roles config is corrected.
// Idempotent, so it is safe to run on every reconcile.
func (c *Client) ensureConsoleProtocolMappers(ctx context.Context, clientUUID, gatewayClientID string) error {
	existing, err := c.listClientProtocolMappers(ctx, clientUUID)
	if err != nil {
		return fmt.Errorf("list console protocol mappers: %w", err)
	}
	base := fmt.Sprintf("/admin/realms/%s/clients/%s/protocol-mappers/models", c.realm, clientUUID)
	for _, mapper := range desiredConsoleProtocolMappers(gatewayClientID) {
		name, _ := mapper["name"].(string)
		if id, ok := existing[name]; ok {
			// PUT the desired representation over the existing mapper (Keycloak
			// requires the id in both the path and the body).
			mapper["id"] = id
			body, _ := json.Marshal(mapper)
			if _, err := c.doRequest(ctx, http.MethodPut, base+"/"+id, body); err != nil {
				return fmt.Errorf("update console mapper %s: %w", name, err)
			}
			continue
		}
		body, _ := json.Marshal(mapper)
		if _, err := c.doRequest(ctx, http.MethodPost, base, body); err != nil {
			return fmt.Errorf("create console mapper %s: %w", name, err)
		}
	}
	return nil
}

// listClientProtocolMappers returns the existing protocol mappers on a client,
// keyed by mapper name.
func (c *Client) listClientProtocolMappers(ctx context.Context, clientUUID string) (map[string]string, error) {
	path := fmt.Sprintf("/admin/realms/%s/clients/%s/protocol-mappers/models", c.realm, clientUUID)
	respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var mappers []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(respBody, &mappers); err != nil {
		return nil, fmt.Errorf("parse protocol mappers: %w", err)
	}
	out := make(map[string]string, len(mappers))
	for _, m := range mappers {
		out[m.Name] = m.ID
	}
	return out, nil
}

func (c *Client) getClientSecret(ctx context.Context, clientUUID string) (string, error) {
	path := fmt.Sprintf("/admin/realms/%s/clients/%s/client-secret", c.realm, clientUUID)
	respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}

	var secret struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(respBody, &secret); err != nil {
		return "", fmt.Errorf("parse client secret response: %w", err)
	}
	if secret.Value == "" {
		return "", fmt.Errorf("empty client secret returned for client uuid %s", clientUUID)
	}
	return secret.Value, nil
}
