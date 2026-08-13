package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client wraps the Keycloak Admin REST API for gateway OIDC provisioning.
type Client struct {
	serverURL    string
	realm        string
	clientID     string
	clientSecret string

	mu            sync.Mutex
	token         string
	tokenExpiry   time.Time
	httpClient    *http.Client
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
	ID       string `json:"id,omitempty"`
	ClientID string `json:"clientId"`
}

type keycloakRole struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

type keycloakUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// ProvisionGatewayClient creates a Keycloak OIDC client for a gateway with
// roles and protocol mappers. Returns the Keycloak-internal client UUID.
func (c *Client) ProvisionGatewayClient(ctx context.Context, gatewayName string) (string, error) {
	clientUUID, err := c.createClient(ctx, gatewayName)
	if err != nil {
		return "", fmt.Errorf("create keycloak client: %w", err)
	}

	if err := c.createClientRoles(ctx, clientUUID); err != nil {
		if rollbackErr := c.deleteClientByUUID(ctx, clientUUID); rollbackErr != nil {
			log.Printf("WARN failed to rollback keycloak client %s after role creation failure: %v", gatewayName, rollbackErr)
		}
		return "", fmt.Errorf("create client roles: %w", err)
	}

	if err := c.createProtocolMappers(ctx, clientUUID, gatewayName); err != nil {
		if rollbackErr := c.deleteClientByUUID(ctx, clientUUID); rollbackErr != nil {
			log.Printf("WARN failed to rollback keycloak client %s after mapper creation failure: %v", gatewayName, rollbackErr)
		}
		return "", fmt.Errorf("create protocol mappers: %w", err)
	}

	return clientUUID, nil
}

// DeleteGatewayClient removes the Keycloak client for a gateway.
// Returns nil if the client does not exist.
func (c *Client) DeleteGatewayClient(ctx context.Context, gatewayName string) error {
	clientUUID, err := c.getClientUUID(ctx, gatewayName)
	if err != nil {
		return err
	}
	if clientUUID == "" {
		log.Printf("DEBUG keycloak client %s not found, nothing to delete", gatewayName)
		return nil
	}
	return c.deleteClientByUUID(ctx, clientUUID)
}

// AssignClientRole assigns a Keycloak client role to a user on a gateway.
func (c *Client) AssignClientRole(ctx context.Context, gatewayName, username, roleName string) error {
	clientUUID, err := c.getClientUUID(ctx, gatewayName)
	if err != nil {
		return fmt.Errorf("get client UUID for %s: %w", gatewayName, err)
	}
	if clientUUID == "" {
		return fmt.Errorf("keycloak client %s not found", gatewayName)
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
	_, err = c.doRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return fmt.Errorf("assign role %s to %s on %s: %w", roleName, username, gatewayName, err)
	}

	log.Printf("INFO assigned keycloak role %s to user %s on client %s", roleName, username, gatewayName)
	return nil
}

// RemoveClientRole removes a Keycloak client role from a user on a gateway.
func (c *Client) RemoveClientRole(ctx context.Context, gatewayName, username, roleName string) error {
	clientUUID, err := c.getClientUUID(ctx, gatewayName)
	if err != nil {
		return fmt.Errorf("get client UUID for %s: %w", gatewayName, err)
	}
	if clientUUID == "" {
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

	log.Printf("INFO removed keycloak role %s from user %s on client %s", roleName, username, gatewayName)
	return nil
}

// GetClientUUID returns the Keycloak-internal UUID for a client by clientId,
// or empty string if not found.
func (c *Client) GetClientUUID(ctx context.Context, gatewayName string) (string, error) {
	return c.getClientUUID(ctx, gatewayName)
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
			"pkce.code.challenge.method": "S256",
		},
		"defaultClientScopes": []string{
			"openid", "profile", "email", "roles", "gateway-roles", "web-origins", "acr",
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
	defer resp.Body.Close()

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
				"claim.name":                             "hypershell.roles",
				"multivalued":                            "true",
				"jsonType.label":                         "String",
				"id.token.claim":                         "true",
				"access.token.claim":                     "true",
				"usermodel.clientRoleMapping.clientId":    gatewayName,
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
	defer resp.Body.Close()

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
	return nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	resp, err := c.doRequestRaw(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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
