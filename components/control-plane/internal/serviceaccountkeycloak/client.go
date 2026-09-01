// Package serviceaccountkeycloak contains the control plane's narrowly scoped
// Keycloak integration for OpenShell gateway service accounts. Credentials are
// handled only in memory and are never included in errors or logs.
package serviceaccountkeycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
)

const (
	RoleUser                       = "openshell-user"
	RoleAdmin                      = "openshell-admin"
	managedAttribute               = "hypershell.service-account"
	gatewayIDAttribute             = "hypershell.gateway-id"
	serviceAccountIDAttribute      = "hypershell.service-account-id"
	creatorUserIDAttribute         = "hypershell.creator-user-id"
	accessTokenLifespanAttribute   = "access.token.lifespan"
	clientRefreshTokenAttribute    = "client_credentials.use_refresh_token"
	deviceGrantAttribute           = "oauth2.device.authorization.grant.enabled"
	cibaGrantAttribute             = "oidc.ciba.grant.enabled"
	builtInServiceAccountScope     = "service_account"
	defaultAccessTokenLifetimeSecs = 300
)

// ErrNotFound means that the managed Keycloak client no longer exists.
var ErrNotFound = errors.New("keycloak client not found")

// ErrNotManaged means the target Keycloak client exists but is not a
// HyperShell-managed service account. Destructive operations reject these
// clients so the realm-admin credential can never disable or delete gateway,
// console, or otherwise unrelated clients.
var ErrNotManaged = errors.New("keycloak client is not a HyperShell-managed service account")

// ServiceAccountSpec is the complete desired Keycloak state for one
// OpenShellGatewayServiceAccount.
type ServiceAccountSpec struct {
	ClientID                   string
	DisplayName                string
	GatewayClientID            string
	GatewayID                  string
	ServiceAccountID           string
	CreatorUserID              string
	Role                       string
	ExpectedIssuer             string
	AccessTokenLifetimeSeconds int
}

// ProvisionedServiceAccount contains the identifiers that can be persisted and
// the one-time client secret that must only be returned to the caller.
type ProvisionedServiceAccount struct {
	ClientUUID   string
	ClientID     string
	ClientSecret string
	Subject      string
}

// ManagedClient is the non-secret identity of a HyperShell-managed Keycloak
// service-account client.
type ManagedClient struct {
	UUID             string
	ClientID         string
	GatewayID        string
	ServiceAccountID string
}

// Client implements the small subset of Keycloak's Admin REST API needed for
// service-account lifecycle management.
type Client struct {
	serverURL    string
	realm        string
	clientID     string
	clientSecret string
	httpClient   *http.Client

	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

func NewClient(serverURL, realm, clientID, clientSecret string) *Client {
	return &Client{
		serverURL:    strings.TrimRight(serverURL, "/"),
		realm:        realm,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Configured reports whether all required administrator credentials exist.
func (c *Client) Configured() bool {
	return c != nil && c.serverURL != "" && c.realm != "" && c.clientID != "" && c.clientSecret != ""
}

func (c *Client) issuer() string { return fmt.Sprintf("%s/realms/%s", c.serverURL, c.realm) }

type kcClient struct {
	ID                           string            `json:"id,omitempty"`
	ClientID                     string            `json:"clientId"`
	Name                         string            `json:"name,omitempty"`
	Protocol                     string            `json:"protocol,omitempty"`
	ClientAuthenticatorType      string            `json:"clientAuthenticatorType,omitempty"`
	Enabled                      bool              `json:"enabled"`
	PublicClient                 bool              `json:"publicClient"`
	ServiceAccountsEnabled       bool              `json:"serviceAccountsEnabled"`
	StandardFlowEnabled          bool              `json:"standardFlowEnabled"`
	ImplicitFlowEnabled          bool              `json:"implicitFlowEnabled"`
	DirectAccessGrantsEnabled    bool              `json:"directAccessGrantsEnabled"`
	AuthorizationServicesEnabled bool              `json:"authorizationServicesEnabled"`
	FullScopeAllowed             bool              `json:"fullScopeAllowed"`
	RedirectURIs                 []string          `json:"redirectUris"`
	WebOrigins                   []string          `json:"webOrigins"`
	DefaultClientScopes          []string          `json:"defaultClientScopes"`
	OptionalClientScopes         []string          `json:"optionalClientScopes"`
	Attributes                   map[string]string `json:"attributes,omitempty"`
}

type kcRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type kcUser struct {
	ID string `json:"id"`
}

// ProvisionServiceAccount creates the client disabled, grants exactly the
// selected gateway roles, obtains the secret, enables the client, and verifies
// a client-credentials access token before returning the secret.
func (c *Client) ProvisionServiceAccount(ctx context.Context, spec ServiceAccountSpec) (_ *ProvisionedServiceAccount, err error) {
	if !c.Configured() {
		return nil, errors.New("keycloak service-account provisioner is not configured")
	}
	if err := validateSpec(spec); err != nil {
		return nil, err
	}

	gatewayUUID, roles, err := c.resolveGatewayRoles(ctx, spec.GatewayClientID, spec.Role)
	if err != nil {
		return nil, fmt.Errorf("resolve gateway authorization: %w", err)
	}
	clientUUID, err := c.createServiceAccountClient(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("create service-account client: %w", err)
	}
	rollback := true
	defer func() {
		if rollback {
			_ = c.deleteClient(ctx, clientUUID)
		}
	}()

	subject, err := c.serviceAccountUserID(ctx, clientUUID)
	if err != nil {
		return nil, fmt.Errorf("resolve service-account subject: %w", err)
	}
	if err := c.replaceRoleMappings(ctx, clientUUID, subject, gatewayUUID, roles); err != nil {
		return nil, fmt.Errorf("set gateway role mappings: %w", err)
	}
	if err := c.replaceProtocolMappers(ctx, clientUUID, spec.GatewayClientID); err != nil {
		return nil, fmt.Errorf("set token claims: %w", err)
	}
	secret, err := c.getClientSecret(ctx, clientUUID)
	if err != nil {
		return nil, fmt.Errorf("obtain one-time client credential: %w", err)
	}
	if err := c.setEnabled(ctx, clientUUID, true); err != nil {
		return nil, fmt.Errorf("enable service-account client: %w", err)
	}
	if err := c.verifyClientCredentials(ctx, spec, secret, subject); err != nil {
		return nil, fmt.Errorf("verify service-account token: %w", err)
	}

	rollback = false
	return &ProvisionedServiceAccount{
		ClientUUID: clientUUID, ClientID: spec.ClientID, ClientSecret: secret, Subject: subject,
	}, nil
}

// ReconcileServiceAccount disables the client while it restores its structural
// configuration, then enables it only when desired. It never reads or rotates
// the client secret and never mints a token.
func (c *Client) ReconcileServiceAccount(ctx context.Context, spec ServiceAccountSpec, clientUUID, expectedSubject string, enabled bool) error {
	if !c.Configured() {
		return errors.New("keycloak service-account provisioner is not configured")
	}
	client, err := c.getClient(ctx, clientUUID)
	if err != nil {
		return err
	}
	if client.ClientID != spec.ClientID || client.Attributes[managedAttribute] != "true" ||
		client.Attributes[gatewayIDAttribute] != spec.GatewayID || client.Attributes[serviceAccountIDAttribute] != spec.ServiceAccountID {
		return errors.New("keycloak client ownership metadata does not match")
	}
	gatewayUUID, roles, err := c.resolveGatewayRoles(ctx, spec.GatewayClientID, spec.Role)
	if err != nil {
		return err
	}
	subject, err := c.serviceAccountUserID(ctx, clientUUID)
	if err != nil {
		return err
	}
	if expectedSubject != "" && subject != expectedSubject {
		return errors.New("keycloak service-account subject changed")
	}
	// Diff before mutating. A converged client is left completely untouched so a
	// routine reconciliation sweep never disables and rebuilds a healthy client,
	// which would otherwise cause a token-grant outage on every pass and, on a
	// later failure, strand the client disabled.
	converged, err := c.reconcileConverged(ctx, spec, client, subject, gatewayUUID, roles, enabled)
	if err != nil {
		return err
	}
	if converged {
		return nil
	}
	// Repair requires the client disabled so a partially applied role or mapper
	// change can never mint a token mid-reconcile.
	if err := c.setEnabled(ctx, clientUUID, false); err != nil {
		return err
	}
	if err := c.updateRepresentation(ctx, clientUUID, spec, false); err != nil {
		return err
	}
	if err := c.replaceRoleMappings(ctx, clientUUID, subject, gatewayUUID, roles); err != nil {
		return err
	}
	if err := c.replaceProtocolMappers(ctx, clientUUID, spec.GatewayClientID); err != nil {
		return err
	}
	if enabled {
		return c.setEnabled(ctx, clientUUID, true)
	}
	return nil
}

// reconcileConverged reports whether the live Keycloak client already matches
// the desired structural state (enabled flag, ownership/config attributes, role
// mappings, scope mappings, and protocol mappers). When it returns true the
// caller performs zero writes.
func (c *Client) reconcileConverged(ctx context.Context, spec ServiceAccountSpec, client *kcClient, subject, gatewayUUID string, roles []kcRole, enabled bool) (bool, error) {
	if client.ClientID != spec.ClientID || client.Name != spec.DisplayName || client.Enabled != enabled {
		return false, nil
	}
	// protocol and clientAuthenticatorType are behavior-bearing and pinned by
	// repair: a switch away from openid-connect changes token issuance entirely,
	// and a changed authenticator type (for example to client-jwt or client-x509)
	// changes how the client authenticates and can strand the issued secret. Repair
	// controls both, so drift on either must fail closed to the repair path.
	if client.Protocol != "openid-connect" || client.ClientAuthenticatorType != "client-secret" {
		return false, nil
	}
	// Every security-relevant field that createServiceAccountClient and
	// updateRepresentation pin to a least-privilege value must be inspected here.
	// A field that repair controls but this predicate ignores would let
	// security-broadening drift survive as "converged" and skip repair: a flipped
	// fullScopeAllowed leaks every realm role into the token, an enabled
	// interactive/device grant or public-client downgrade widens the authorized
	// flows, and a rogue redirect origin or injected client scope broadens token
	// contents. Any divergence therefore fails closed toward the repair path.
	if client.PublicClient || !client.ServiceAccountsEnabled || client.StandardFlowEnabled ||
		client.ImplicitFlowEnabled || client.DirectAccessGrantsEnabled ||
		client.AuthorizationServicesEnabled || client.FullScopeAllowed {
		return false, nil
	}
	if len(client.RedirectURIs) > 0 || len(client.WebOrigins) > 0 ||
		!defaultClientScopesConverged(client.DefaultClientScopes) || len(client.OptionalClientScopes) > 0 {
		return false, nil
	}
	lifetime := spec.AccessTokenLifetimeSeconds
	if lifetime == 0 {
		lifetime = defaultAccessTokenLifetimeSecs
	}
	wantAttributes := map[string]string{
		managedAttribute: "true", gatewayIDAttribute: spec.GatewayID,
		serviceAccountIDAttribute: spec.ServiceAccountID, creatorUserIDAttribute: spec.CreatorUserID,
		accessTokenLifespanAttribute: fmt.Sprint(lifetime), clientRefreshTokenAttribute: "false",
		deviceGrantAttribute: "false", cibaGrantAttribute: "false",
	}
	for key, value := range wantAttributes {
		if client.Attributes[key] != value {
			return false, nil
		}
	}
	userConverged, err := c.userRoleMappingsConverged(ctx, subject, gatewayUUID, roles)
	if err != nil || !userConverged {
		return false, err
	}
	scopeConverged, err := c.scopeMappingsConverged(ctx, client.ID, gatewayUUID, roles)
	if err != nil || !scopeConverged {
		return false, err
	}
	return c.protocolMappersConverged(ctx, client.ID, spec.GatewayClientID)
}

// defaultClientScopesConverged accepts the built-in scope that Keycloak adds
// when service accounts are enabled. The repair payload stays empty because
// Keycloak owns this scope. All other scopes are drift.
func defaultClientScopesConverged(scopes []string) bool {
	return len(scopes) == 0 || (len(scopes) == 1 && scopes[0] == builtInServiceAccountScope)
}

// roleMappingSet is the shape Keycloak returns for both user role-mappings and
// client scope-mappings.
type roleMappingSet struct {
	RealmMappings  []kcRole `json:"realmMappings"`
	ClientMappings map[string]struct {
		ID       string   `json:"id"`
		Mappings []kcRole `json:"mappings"`
	} `json:"clientMappings"`
}

func mappingsConverged(mappings roleMappingSet, gatewayUUID string, roles []kcRole) bool {
	if len(mappings.RealmMappings) > 0 {
		return false
	}
	foundGateway := false
	for _, mapping := range mappings.ClientMappings {
		if mapping.ID == gatewayUUID {
			foundGateway = true
			if !sameRoleSet(mapping.Mappings, roles) {
				return false
			}
			continue
		}
		// Any role on another client is cross-gateway leakage that must be repaired.
		if len(mapping.Mappings) > 0 {
			return false
		}
	}
	return foundGateway
}

func (c *Client) userRoleMappingsConverged(ctx context.Context, subject, gatewayUUID string, roles []kcRole) (bool, error) {
	body, status, err := c.admin(ctx, http.MethodGet, fmt.Sprintf("/admin/realms/%s/users/%s/role-mappings", c.realm, url.PathEscape(subject)), nil)
	if err != nil {
		return false, err
	}
	if status != http.StatusOK {
		return false, statusError("list service-account role mappings", status)
	}
	var current roleMappingSet
	if err := json.Unmarshal(body, &current); err != nil {
		return false, errors.New("parse service-account role mappings")
	}
	return mappingsConverged(current, gatewayUUID, roles), nil
}

func (c *Client) scopeMappingsConverged(ctx context.Context, clientUUID, gatewayUUID string, roles []kcRole) (bool, error) {
	body, status, err := c.admin(ctx, http.MethodGet, fmt.Sprintf("/admin/realms/%s/clients/%s/scope-mappings", c.realm, url.PathEscape(clientUUID)), nil)
	if err != nil {
		return false, err
	}
	if status != http.StatusOK {
		return false, statusError("list client scope mappings", status)
	}
	var current roleMappingSet
	if err := json.Unmarshal(body, &current); err != nil {
		return false, errors.New("parse client scope mappings")
	}
	return mappingsConverged(current, gatewayUUID, roles), nil
}

// managedProtocolMapper is one protocol mapper HyperShell pins on a managed
// service-account client. protocol and protocolMapper select the mapper behavior;
// every config entry is a required value.
type managedProtocolMapper struct {
	name           string
	protocol       string
	protocolMapper string
	config         map[string]string
}

// managedProtocolMappers returns the exact protocol mappers HyperShell pins on a
// managed service-account client. Repair (replaceProtocolMappers) writes these
// and convergence (protocolMappersConverged) compares against them, so a single
// definition keeps the two in lockstep: no behavior-bearing mapper config value
// can drift while still reading as converged.
func managedProtocolMappers(gatewayClientID string) []managedProtocolMapper {
	return []managedProtocolMapper{
		{
			name:           "gateway-audience",
			protocol:       "openid-connect",
			protocolMapper: "oidc-audience-mapper",
			// included.custom.audience must stay empty: an audience mapper carrying a
			// free-form custom audience adds an unrelated gateway to the token, which is
			// exactly the cross-gateway leakage repair must remove.
			config: map[string]string{
				"included.client.audience": gatewayClientID,
				"included.custom.audience": "",
				"id.token.claim":           "false",
				"access.token.claim":       "true",
			},
		},
		{
			name:           "gateway-client-roles",
			protocol:       "openid-connect",
			protocolMapper: "oidc-usermodel-client-role-mapper",
			// access.token.claim, multivalued, and jsonType.label are all
			// behavior-bearing: dropping the claim strips every role from the access
			// token, collapsing to a single value drops all but one role, and a changed
			// JSON type corrupts the roles the gateway authorizes on.
			config: map[string]string{
				"usermodel.clientRoleMapping.clientId": gatewayClientID,
				"claim.name":                           "hypershell.roles",
				"multivalued":                          "true",
				"jsonType.label":                       "String",
				"id.token.claim":                       "false",
				"access.token.claim":                   "true",
			},
		},
	}
}

// parsedProtocolMapper is the shape Keycloak returns for a client protocol mapper.
type parsedProtocolMapper struct {
	Name           string            `json:"name"`
	Protocol       string            `json:"protocol"`
	ProtocolMapper string            `json:"protocolMapper"`
	Config         map[string]string `json:"config"`
}

func (c *Client) protocolMappersConverged(ctx context.Context, clientUUID, gatewayClientID string) (bool, error) {
	body, status, err := c.admin(ctx, http.MethodGet, fmt.Sprintf("/admin/realms/%s/clients/%s/protocol-mappers/models", c.realm, url.PathEscape(clientUUID)), nil)
	if err != nil {
		return false, err
	}
	if status != http.StatusOK {
		return false, statusError("list service-account protocol mappers", status)
	}
	var current []parsedProtocolMapper
	if err := json.Unmarshal(body, &current); err != nil {
		return false, errors.New("parse service-account protocol mappers")
	}
	desired := managedProtocolMappers(gatewayClientID)
	// Exactly the managed mappers, no more: any extra mapper can inject claims.
	if len(current) != len(desired) {
		return false, nil
	}
	byName := make(map[string]parsedProtocolMapper, len(current))
	for _, mapper := range current {
		byName[mapper.Name] = mapper
	}
	for _, want := range desired {
		live, ok := byName[want.name]
		// protocol and protocolMapper both select the mapper behavior: a mapper that
		// keeps the name but switches protocol no longer emits the required claim, so
		// it must fail closed to repair even when the config still matches.
		if !ok || live.Protocol != want.protocol || live.ProtocolMapper != want.protocolMapper {
			return false, nil
		}
		// Compare every required config value repair writes. A missing key reads as
		// the empty string, so an omitted required value fails closed to repair.
		for key, value := range want.config {
			if live.Config[key] != value {
				return false, nil
			}
		}
	}
	return true, nil
}

func sameRoleSet(current, desired []kcRole) bool {
	if len(current) != len(desired) {
		return false
	}
	want := make(map[string]bool, len(desired))
	for _, role := range desired {
		want[role.Name] = true
	}
	for _, role := range current {
		if !want[role.Name] {
			return false
		}
	}
	return true
}

func (c *Client) DisableServiceAccount(ctx context.Context, clientUUID, gatewayID, serviceAccountID string) error {
	if _, err := c.requireManagedClient(ctx, clientUUID, gatewayID, serviceAccountID); errors.Is(err, ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	return c.setEnabled(ctx, clientUUID, false)
}

func (c *Client) DeleteServiceAccount(ctx context.Context, clientUUID, gatewayID, serviceAccountID string) error {
	if _, err := c.requireManagedClient(ctx, clientUUID, gatewayID, serviceAccountID); errors.Is(err, ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if err := c.setEnabled(ctx, clientUUID, false); err != nil {
		return err
	}
	return c.deleteClient(ctx, clientUUID)
}

// requireManagedClient fetches a client by UUID and confirms it carries the
// HyperShell managed marker and, when the caller supplies them, the exact
// gateway and service-account ownership attributes. Because Disable and Delete
// accept a caller-supplied UUID, this guard is what prevents the privileged
// provisioner credential from being turned into a delete primitive against
// non-managed clients or against a managed client owned by a different gateway
// or service account.
func (c *Client) requireManagedClient(ctx context.Context, clientUUID, gatewayID, serviceAccountID string) (*kcClient, error) {
	client, err := c.getClient(ctx, clientUUID)
	if err != nil {
		return nil, err
	}
	if client.Attributes[managedAttribute] != "true" {
		return nil, ErrNotManaged
	}
	if gatewayID != "" && client.Attributes[gatewayIDAttribute] != gatewayID {
		return nil, ErrNotManaged
	}
	if serviceAccountID != "" && client.Attributes[serviceAccountIDAttribute] != serviceAccountID {
		return nil, ErrNotManaged
	}
	return client, nil
}

// DeleteManagedServiceAccount removes a partially provisioned client by its
// immutable ownership attributes. It is the crash-recovery path for the narrow
// window before the Keycloak UUID is persisted.
func (c *Client) DeleteManagedServiceAccount(ctx context.Context, gatewayID, serviceAccountID string) error {
	clients, err := c.ListManagedClients(ctx, gatewayID)
	if err != nil {
		return err
	}
	for _, client := range clients {
		if client.ServiceAccountID != serviceAccountID {
			continue
		}
		if err := c.DeleteServiceAccount(ctx, client.UUID, gatewayID, serviceAccountID); err != nil {
			return err
		}
	}
	return nil
}

// DeleteGatewayServiceAccounts disables all managed clients for a gateway
// before deleting any of them. It also catches clients orphaned from the API
// database because selection uses immutable Keycloak attributes.
func (c *Client) DeleteGatewayServiceAccounts(ctx context.Context, gatewayID string) error {
	clients, err := c.ListManagedClients(ctx, gatewayID)
	if err != nil {
		return err
	}
	for _, client := range clients {
		if err := c.setEnabled(ctx, client.UUID, false); err != nil {
			return err
		}
	}
	for _, client := range clients {
		if err := c.deleteClient(ctx, client.UUID); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) ListManagedClients(ctx context.Context, gatewayID string) ([]ManagedClient, error) {
	const pageSize = 100
	clients := make([]kcClient, 0)
	for first := 0; ; first += pageSize {
		path := fmt.Sprintf("/admin/realms/%s/clients?first=%d&max=%d", c.realm, first, pageSize)
		body, status, err := c.admin(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, statusError("list managed clients", status)
		}
		var page []kcClient
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, errors.New("parse Keycloak client list")
		}
		clients = append(clients, page...)
		if len(page) < pageSize {
			break
		}
	}
	out := make([]ManagedClient, 0)
	for _, listed := range clients {
		client, getErr := c.getClient(ctx, listed.ID)
		if getErr != nil {
			return nil, getErr
		}
		if client.Attributes[managedAttribute] != "true" || (gatewayID != "" && client.Attributes[gatewayIDAttribute] != gatewayID) {
			continue
		}
		out = append(out, ManagedClient{UUID: client.ID, ClientID: client.ClientID, GatewayID: client.Attributes[gatewayIDAttribute], ServiceAccountID: client.Attributes[serviceAccountIDAttribute]})
	}
	return out, nil
}

func validateSpec(spec ServiceAccountSpec) error {
	if spec.ClientID == "" || spec.GatewayClientID == "" || spec.GatewayID == "" || spec.ServiceAccountID == "" || spec.CreatorUserID == "" || spec.ExpectedIssuer == "" {
		return errors.New("incomplete service-account specification")
	}
	if spec.Role != RoleUser && spec.Role != RoleAdmin {
		return errors.New("unsupported service-account role")
	}
	if spec.AccessTokenLifetimeSeconds == 0 {
		spec.AccessTokenLifetimeSeconds = defaultAccessTokenLifetimeSecs
	}
	if spec.AccessTokenLifetimeSeconds < 1 || spec.AccessTokenLifetimeSeconds > 900 {
		return errors.New("access-token lifetime must be between 1 and 900 seconds")
	}
	return nil
}

func desiredRoleNames(role string) []string {
	if role == RoleAdmin {
		return []string{RoleAdmin, RoleUser}
	}
	return []string{RoleUser}
}

func (c *Client) resolveGatewayRoles(ctx context.Context, gatewayClientID, role string) (string, []kcRole, error) {
	gatewayUUID, err := c.clientUUID(ctx, gatewayClientID)
	if err != nil {
		return "", nil, err
	}
	if gatewayUUID == "" {
		return "", nil, ErrNotFound
	}
	body, status, err := c.admin(ctx, http.MethodGet, fmt.Sprintf("/admin/realms/%s/clients/%s/roles", c.realm, url.PathEscape(gatewayUUID)), nil)
	if err != nil {
		return "", nil, err
	}
	if status != http.StatusOK {
		return "", nil, statusError("list gateway roles", status)
	}
	var available []kcRole
	if err := json.Unmarshal(body, &available); err != nil {
		return "", nil, errors.New("parse gateway roles")
	}
	byName := make(map[string]kcRole, len(available))
	for _, item := range available {
		byName[item.Name] = item
	}
	wanted := desiredRoleNames(role)
	roles := make([]kcRole, 0, len(wanted))
	for _, name := range wanted {
		item, ok := byName[name]
		if !ok {
			return "", nil, fmt.Errorf("required gateway role %s not found", name)
		}
		roles = append(roles, item)
	}
	return gatewayUUID, roles, nil
}

func (c *Client) createServiceAccountClient(ctx context.Context, spec ServiceAccountSpec) (string, error) {
	lifetime := spec.AccessTokenLifetimeSeconds
	if lifetime == 0 {
		lifetime = defaultAccessTokenLifetimeSecs
	}
	payload := map[string]any{
		"clientId": spec.ClientID, "name": spec.DisplayName, "enabled": false,
		"protocol": "openid-connect", "publicClient": false,
		"clientAuthenticatorType": "client-secret", "serviceAccountsEnabled": true,
		"standardFlowEnabled": false, "implicitFlowEnabled": false,
		"directAccessGrantsEnabled": false, "authorizationServicesEnabled": false,
		"fullScopeAllowed": false, "redirectUris": []string{}, "webOrigins": []string{},
		"defaultClientScopes": []string{}, "optionalClientScopes": []string{},
		"attributes": map[string]string{
			managedAttribute: "true", gatewayIDAttribute: spec.GatewayID,
			serviceAccountIDAttribute: spec.ServiceAccountID, creatorUserIDAttribute: spec.CreatorUserID,
			accessTokenLifespanAttribute: fmt.Sprint(lifetime), clientRefreshTokenAttribute: "false",
			deviceGrantAttribute: "false", cibaGrantAttribute: "false",
		},
	}
	body, _ := json.Marshal(payload)
	_, status, headers, err := c.adminRaw(ctx, http.MethodPost, fmt.Sprintf("/admin/realms/%s/clients", c.realm), body)
	if err != nil {
		return "", err
	}
	if status == http.StatusConflict {
		return "", errors.New("keycloak client identifier already exists")
	}
	if status != http.StatusCreated {
		return "", statusError("create service-account client", status)
	}
	location := headers.Get("Location")
	parts := strings.Split(strings.TrimRight(location, "/"), "/")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return "", errors.New("keycloak create response omitted the client identifier")
	}
	return parts[len(parts)-1], nil
}

func (c *Client) updateRepresentation(ctx context.Context, uuid string, spec ServiceAccountSpec, enabled bool) error {
	lifetime := spec.AccessTokenLifetimeSeconds
	if lifetime == 0 {
		lifetime = defaultAccessTokenLifetimeSecs
	}
	payload := map[string]any{
		"id": uuid, "clientId": spec.ClientID, "name": spec.DisplayName, "enabled": enabled,
		"protocol": "openid-connect", "publicClient": false, "clientAuthenticatorType": "client-secret",
		"serviceAccountsEnabled": true, "standardFlowEnabled": false, "implicitFlowEnabled": false,
		"directAccessGrantsEnabled": false, "authorizationServicesEnabled": false,
		"fullScopeAllowed": false, "redirectUris": []string{}, "webOrigins": []string{},
		"defaultClientScopes": []string{}, "optionalClientScopes": []string{},
		"attributes": map[string]string{
			managedAttribute: "true", gatewayIDAttribute: spec.GatewayID,
			serviceAccountIDAttribute: spec.ServiceAccountID, creatorUserIDAttribute: spec.CreatorUserID,
			accessTokenLifespanAttribute: fmt.Sprint(lifetime), clientRefreshTokenAttribute: "false",
			deviceGrantAttribute: "false", cibaGrantAttribute: "false",
		},
	}
	body, _ := json.Marshal(payload)
	_, status, err := c.admin(ctx, http.MethodPut, fmt.Sprintf("/admin/realms/%s/clients/%s", c.realm, url.PathEscape(uuid)), body)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return ErrNotFound
	}
	if status >= 300 {
		return statusError("update service-account client", status)
	}
	return nil
}

func (c *Client) setEnabled(ctx context.Context, uuid string, enabled bool) error {
	path := fmt.Sprintf("/admin/realms/%s/clients/%s", c.realm, url.PathEscape(uuid))
	body, status, err := c.admin(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return ErrNotFound
	}
	if status != http.StatusOK {
		return statusError("get service-account client state", status)
	}
	var representation map[string]json.RawMessage
	if err := json.Unmarshal(body, &representation); err != nil {
		return errors.New("parse Keycloak client state")
	}
	representation["enabled"], _ = json.Marshal(enabled)
	payload, _ := json.Marshal(representation)
	_, status, err = c.admin(ctx, http.MethodPut, path, payload)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return ErrNotFound
	}
	if status >= 300 {
		return statusError("change service-account client state", status)
	}
	return nil
}

func (c *Client) getClient(ctx context.Context, uuid string) (*kcClient, error) {
	body, status, err := c.admin(ctx, http.MethodGet, fmt.Sprintf("/admin/realms/%s/clients/%s", c.realm, url.PathEscape(uuid)), nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if status != http.StatusOK {
		return nil, statusError("get service-account client", status)
	}
	var client kcClient
	if err := json.Unmarshal(body, &client); err != nil {
		return nil, errors.New("parse Keycloak client")
	}
	return &client, nil
}

func (c *Client) clientUUID(ctx context.Context, clientID string) (string, error) {
	body, status, err := c.admin(ctx, http.MethodGet, fmt.Sprintf("/admin/realms/%s/clients?clientId=%s", c.realm, url.QueryEscape(clientID)), nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", statusError("resolve Keycloak client", status)
	}
	var clients []kcClient
	if err := json.Unmarshal(body, &clients); err != nil {
		return "", errors.New("parse Keycloak client lookup")
	}
	for _, client := range clients {
		if client.ClientID == clientID {
			return client.ID, nil
		}
	}
	return "", nil
}

func (c *Client) serviceAccountUserID(ctx context.Context, clientUUID string) (string, error) {
	body, status, err := c.admin(ctx, http.MethodGet, fmt.Sprintf("/admin/realms/%s/clients/%s/service-account-user", c.realm, url.PathEscape(clientUUID)), nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", statusError("resolve service-account user", status)
	}
	var user kcUser
	if err := json.Unmarshal(body, &user); err != nil || user.ID == "" {
		return "", errors.New("parse Keycloak service-account user")
	}
	return user.ID, nil
}

func (c *Client) replaceRoleMappings(ctx context.Context, clientUUID, subject, gatewayUUID string, roles []kcRole) error {
	// Remove every client-role assignment currently held by the service-account
	// user. This is what makes drift repair revoke cross-gateway role leakage.
	body, status, err := c.admin(ctx, http.MethodGet, fmt.Sprintf("/admin/realms/%s/users/%s/role-mappings", c.realm, url.PathEscape(subject)), nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return statusError("list service-account role mappings", status)
	}
	var current struct {
		RealmMappings  []kcRole `json:"realmMappings"`
		ClientMappings map[string]struct {
			ID       string   `json:"id"`
			Mappings []kcRole `json:"mappings"`
		} `json:"clientMappings"`
	}
	if err := json.Unmarshal(body, &current); err != nil {
		return errors.New("parse service-account role mappings")
	}
	if len(current.RealmMappings) > 0 {
		payload, _ := json.Marshal(current.RealmMappings)
		_, deleteStatus, deleteErr := c.admin(ctx, http.MethodDelete, fmt.Sprintf("/admin/realms/%s/users/%s/role-mappings/realm", c.realm, url.PathEscape(subject)), payload)
		if deleteErr != nil {
			return deleteErr
		}
		if deleteStatus >= 300 {
			return statusError("remove unexpected service-account realm roles", deleteStatus)
		}
	}
	for _, mapping := range current.ClientMappings {
		if mapping.ID == "" || len(mapping.Mappings) == 0 {
			continue
		}
		payload, _ := json.Marshal(mapping.Mappings)
		_, deleteStatus, deleteErr := c.admin(ctx, http.MethodDelete, fmt.Sprintf("/admin/realms/%s/users/%s/role-mappings/clients/%s", c.realm, url.PathEscape(subject), url.PathEscape(mapping.ID)), payload)
		if deleteErr != nil {
			return deleteErr
		}
		if deleteStatus >= 300 {
			return statusError("remove unexpected service-account roles", deleteStatus)
		}
	}
	payload, _ := json.Marshal(roles)
	userPath := fmt.Sprintf("/admin/realms/%s/users/%s/role-mappings/clients/%s", c.realm, url.PathEscape(subject), url.PathEscape(gatewayUUID))
	if _, status, err = c.admin(ctx, http.MethodPost, userPath, payload); err != nil || status >= 300 {
		if err != nil {
			return err
		}
		return statusError("assign service-account roles", status)
	}

	// Replace the client's role scope mappings as well. fullScopeAllowed=false
	// then limits the mapper output to exactly this gateway and role set.
	body, status, err = c.admin(ctx, http.MethodGet, fmt.Sprintf("/admin/realms/%s/clients/%s/scope-mappings", c.realm, url.PathEscape(clientUUID)), nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return statusError("list client scope mappings", status)
	}
	var scoped struct {
		RealmMappings  []kcRole `json:"realmMappings"`
		ClientMappings map[string]struct {
			ID       string   `json:"id"`
			Mappings []kcRole `json:"mappings"`
		} `json:"clientMappings"`
	}
	if err := json.Unmarshal(body, &scoped); err != nil {
		return errors.New("parse client scope mappings")
	}
	if len(scoped.RealmMappings) > 0 {
		mapped, _ := json.Marshal(scoped.RealmMappings)
		_, deleteStatus, deleteErr := c.admin(ctx, http.MethodDelete, fmt.Sprintf("/admin/realms/%s/clients/%s/scope-mappings/realm", c.realm, url.PathEscape(clientUUID)), mapped)
		if deleteErr != nil {
			return deleteErr
		}
		if deleteStatus >= 300 {
			return statusError("remove unexpected realm role scope", deleteStatus)
		}
	}
	for _, mapping := range scoped.ClientMappings {
		if len(mapping.Mappings) == 0 || mapping.ID == "" {
			continue
		}
		mapped, _ := json.Marshal(mapping.Mappings)
		_, deleteStatus, deleteErr := c.admin(ctx, http.MethodDelete, fmt.Sprintf("/admin/realms/%s/clients/%s/scope-mappings/clients/%s", c.realm, url.PathEscape(clientUUID), url.PathEscape(mapping.ID)), mapped)
		if deleteErr != nil {
			return deleteErr
		}
		if deleteStatus >= 300 {
			return statusError("remove unexpected client scope", deleteStatus)
		}
	}
	scopePath := fmt.Sprintf("/admin/realms/%s/clients/%s/scope-mappings/clients/%s", c.realm, url.PathEscape(clientUUID), url.PathEscape(gatewayUUID))
	if _, status, err = c.admin(ctx, http.MethodPost, scopePath, payload); err != nil || status >= 300 {
		if err != nil {
			return err
		}
		return statusError("assign client role scope", status)
	}
	return nil
}

func (c *Client) replaceProtocolMappers(ctx context.Context, clientUUID, gatewayClientID string) error {
	path := fmt.Sprintf("/admin/realms/%s/clients/%s/protocol-mappers/models", c.realm, url.PathEscape(clientUUID))
	body, status, err := c.admin(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return statusError("list service-account protocol mappers", status)
	}
	var current []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &current); err != nil {
		return errors.New("parse service-account protocol mappers")
	}
	for _, mapper := range current {
		if mapper.ID == "" {
			continue
		}
		_, deleteStatus, deleteErr := c.admin(ctx, http.MethodDelete, path+"/"+url.PathEscape(mapper.ID), nil)
		if deleteErr != nil {
			return deleteErr
		}
		if deleteStatus >= 300 {
			return statusError("remove unexpected protocol mapper", deleteStatus)
		}
	}
	// Build the payloads from the same definition convergence compares against, so
	// repair can never write a value convergence does not check (or vice versa).
	for _, mapper := range managedProtocolMappers(gatewayClientID) {
		payload, _ := json.Marshal(map[string]any{
			"name":           mapper.name,
			"protocol":       mapper.protocol,
			"protocolMapper": mapper.protocolMapper,
			"config":         mapper.config,
		})
		_, createStatus, createErr := c.admin(ctx, http.MethodPost, path, payload)
		if createErr != nil {
			return createErr
		}
		if createStatus >= 300 {
			return statusError("create service-account protocol mapper", createStatus)
		}
	}
	return nil
}

func (c *Client) getClientSecret(ctx context.Context, clientUUID string) (string, error) {
	body, status, err := c.admin(ctx, http.MethodGet, fmt.Sprintf("/admin/realms/%s/clients/%s/client-secret", c.realm, url.PathEscape(clientUUID)), nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", statusError("read generated client credential", status)
	}
	var credential struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &credential); err != nil || credential.Value == "" {
		return "", errors.New("keycloak returned an empty client credential")
	}
	return credential.Value, nil
}

func (c *Client) deleteClient(ctx context.Context, clientUUID string) error {
	_, status, err := c.admin(ctx, http.MethodDelete, fmt.Sprintf("/admin/realms/%s/clients/%s", c.realm, url.PathEscape(clientUUID)), nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || status == http.StatusNoContent {
		return nil
	}
	if status >= 300 {
		return statusError("delete service-account client", status)
	}
	return nil
}

func (c *Client) verifyClientCredentials(ctx context.Context, spec ServiceAccountSpec, secret, subject string) error {
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {spec.ClientID}, "client_secret": {secret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.issuer()+"/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("build token verification request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.New("call token verification endpoint")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return statusError("token verification endpoint", resp.StatusCode)
	}
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil || result.AccessToken == "" {
		return errors.New("parse token verification response")
	}
	if result.RefreshToken != "" {
		return errors.New("client-credentials grant unexpectedly returned a refresh token")
	}
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(result.AccessToken, jwt.MapClaims{})
	if err != nil {
		return errors.New("parse service-account access token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return errors.New("service-account access token has invalid claims")
	}
	if stringClaim(claims, "iss") != spec.ExpectedIssuer || stringClaim(claims, "sub") != subject || stringClaim(claims, "azp") != spec.ClientID {
		return errors.New("service-account access token identity claims do not match")
	}
	if !equalStrings(audienceClaim(claims["aud"]), []string{spec.GatewayClientID}) {
		return errors.New("service-account access token audience does not match")
	}
	actualRoles := stringSliceClaimAtPath(claims, "hypershell.roles")
	expectedRoles := desiredRoleNames(spec.Role)
	if !equalStrings(actualRoles, expectedRoles) {
		return fmt.Errorf("service-account access token roles do not match: got %q, want %q", actualRoles, expectedRoles)
	}
	iat, iatOK := numberClaim(claims["iat"])
	exp, expOK := numberClaim(claims["exp"])
	wantLifetime := int64(spec.AccessTokenLifetimeSeconds)
	if wantLifetime == 0 {
		wantLifetime = defaultAccessTokenLifetimeSecs
	}
	lifetime := exp - iat
	if !iatOK || !expOK || exp <= iat || lifetime < wantLifetime-5 || lifetime > wantLifetime+5 || result.ExpiresIn < wantLifetime-5 || result.ExpiresIn > wantLifetime+5 {
		return errors.New("service-account access token lifetime does not match")
	}
	return nil
}

func stringClaim(claims jwt.MapClaims, name string) string {
	value, _ := claims[name].(string)
	return value
}

func audienceClaim(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	case []string:
		return typed
	default:
		return nil
	}
}

func stringSliceClaim(value any) []string { return audienceClaim(value) }

func stringSliceClaimAtPath(claims jwt.MapClaims, path string) []string {
	var value any = map[string]any(claims)
	for _, part := range strings.Split(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value, ok = object[part]
		if !ok {
			return nil
		}
	}
	return stringSliceClaim(value)
}

func numberClaim(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case json.Number:
		result, err := typed.Int64()
		return result, err == nil
	default:
		return 0, false
	}
}

func equalStrings(left, right []string) bool {
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (c *Client) ensureAdminToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		return nil
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {c.clientID}, "client_secret": {c.clientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.issuer()+"/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("build Keycloak administrator authentication request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.New("authenticate Keycloak administrator")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return statusError("authenticate Keycloak administrator", resp.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token); err != nil || token.AccessToken == "" {
		return errors.New("parse Keycloak administrator authentication response")
	}
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = 300
	}
	c.token = token.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(float64(token.ExpiresIn)*0.8) * time.Second)
	return nil
}

func (c *Client) admin(ctx context.Context, method, path string, body []byte) ([]byte, int, error) {
	bodyOut, status, _, err := c.adminRaw(ctx, method, path, body)
	return bodyOut, status, err
}

func (c *Client) adminRaw(ctx context.Context, method, path string, body []byte) ([]byte, int, http.Header, error) {
	if err := c.ensureAdminToken(ctx); err != nil {
		return nil, 0, nil, err
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.serverURL+path, reader)
	if err != nil {
		return nil, 0, nil, errors.New("build Keycloak administrator request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, nil, errors.New("call Keycloak administrator API")
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, resp.StatusCode, resp.Header.Clone(), errors.New("read Keycloak administrator response")
	}
	return responseBody, resp.StatusCode, resp.Header.Clone(), nil
}

func statusError(operation string, status int) error {
	return fmt.Errorf("%s returned HTTP %d", operation, status)
}
