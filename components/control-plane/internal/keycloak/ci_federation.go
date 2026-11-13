package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// CI federation realizes keyless CI (GitHub Actions / GitLab CI) against a
// gateway. The token-exchange authorization is config-driven (PLAT-CITR-08): the
// control plane selects by configuration, not code, between fgap-v1 mode (two
// fine-grained authorization v1 grants, PLAT-CITR-11) and v2-attribute mode (the
// external-to-internal token-exchange v2 client attribute). Each mode realizes
// ONLY its own artifacts and never calls the other mode's API, so neither fails
// because the other's API is unavailable on the running Keycloak (a v1 permission
// API returning 501 under v2-attribute mode, or absent v2 external-internal
// support under fgap-v1 mode). The production fleet runs fgap-v1 today.
//
// A CITrust is realized as three artifacts:
//
//  (a) a per-trust scoped claim-to-group IdP mapper on the (platform
//      provisioned, adopted) CIProvider IdP. It admits only brokered tokens
//      whose claims match the compiled subject predicate (repository /
//      repository_owner, plus project_path / environment / ref / ref_protected
//      when the subject sets them) and lands the matched identity in the trust's
//      OWN per-trust group (not a shared per-owner group). This is the trust
//      boundary (PLAT-CITR-04): audience pinning alone is insufficient because
//      any repository can mint a token carrying the pinned audience, so the
//      repository/owner claim match is the real gate. A token that does not
//      match lands in no group, holds no role, and is rejected default-deny.
//
//  (b) a token-exchange authorization enabling the public (keyless) federation
//      client to exchange a brokered token from the IdP onto the gateway client
//      (the exchange audience). Its mechanism is config-driven (PLAT-CITR-08): the
//      two fine-grained v1 grants scoped to the federation requester (PLAT-CITR-11)
//      in fgap-v1 mode, or the external-to-internal v2 attribute on the gateway
//      client in v2-attribute mode.
//
//  (c) a gateway RBAC binding mapping the gateway client role onto the trust's
//      OWN per-trust group, so a matched identity holds exactly the role its own
//      CITrust grants on its own gateway. The group is keyed on the CITrust id,
//      which uniquely identifies one (repository, gateway, role) tuple, so no two
//      distinct trusts ever aggregate their roles onto a shared group: a
//      low-privilege repo cannot inherit a sibling repo's higher role, and a role
//      granted for one gateway never leaks onto another gateway's client
//      (cross-repo / cross-gateway privilege-escalation boundary).
//
// The per-requester exchange scoping (b) is realized in fgap-v1 mode by a client
// policy that names exactly the federation requester client and is attached to both
// the IdP and the gateway-client token-exchange scope permissions (PLAT-CITR-11), so
// only that requester may exchange. The subject constraint (a) is an IdP
// advanced-claim group mapper, and the role grant (c) a group client-role mapping;
// every call below is idempotent and individually verifiable.

const (
	// ciTrustMapperPrefix names the per-trust IdP mapper deterministically from
	// the CITrust id, so repeated reconciles converge to the same mapper and a
	// delete targets exactly this trust without touching siblings.
	ciTrustMapperPrefix = "citrust-"

	// advancedGroupIdpMapper is the Keycloak IdP mapper type that joins a
	// brokered identity to a group only when every configured claim matches.
	advancedGroupIdpMapper = "oidc-advanced-group-idp-mapper"

	// standardTokenExchangeEnabledAttribute is the Keycloak 26.x client
	// attribute that opts a client in as a token-exchange audience under token
	// exchange v2. LIVE-VERIFY against 26.6.
	standardTokenExchangeEnabledAttribute = "standard.token.exchange.enabled"
)

// TokenExchangeMode selects how the control plane authorizes the CI
// token-exchange grant (PLAT-CITR-08). The choice is a deployment configuration,
// not a code path a caller edits: different fleet Keycloaks run different
// token-exchange generations, so the mode is carried on the CIFederationSpec from
// the control-plane configuration (HYPERSHELL_CI_TOKEN_EXCHANGE_MODE) rather than
// hardcoded. Each mode realizes ONLY the artifacts its own mechanism needs and
// never calls the other mode's API, so neither mode fails because the other
// mode's API is unavailable (a v1 permission API returning 501 under v2-attribute
// mode, or absent v2 external-internal support under fgap-v1 mode).
type TokenExchangeMode string

const (
	// TokenExchangeModeFGAPv1 authorizes the exchange through fine-grained admin
	// authorization v1 (PLAT-CITR-11): two token-exchange grants, each scoped to
	// the shared federation requester client, one on the CIProvider IdP and one on
	// the target gateway client. This is the hyp0 fleet mode today.
	TokenExchangeModeFGAPv1 TokenExchangeMode = "fgap-v1"

	// TokenExchangeModeV2Attribute authorizes the exchange through external-to-internal
	// token-exchange v2 by setting the standard.token.exchange.enabled attribute on
	// the target gateway client. Usable once a Keycloak release (RHBK >= 26.7)
	// wires IdP token-exchange permissions into v2.
	TokenExchangeModeV2Attribute TokenExchangeMode = "v2-attribute"

	// defaultTokenExchangeMode is applied when a spec carries no explicit mode. It
	// matches the production fleet Keycloak today (fine-grained authorization v1)
	// and the control-plane configuration default, so an unconfigured reconcile
	// still realizes the grants the live fleet requires.
	defaultTokenExchangeMode = TokenExchangeModeFGAPv1
)

// CISubjectPredicate is the compiled claim predicate a brokered CI token must
// satisfy. Empty fields are not constrained. Repository/RepositoryOwner are the
// trust boundary for forge providers that carry them.
type CISubjectPredicate struct {
	Repository      string
	RepositoryOwner string
	ProjectPath     string
	Environment     string
	Ref             string
	RefProtected    bool
}

// CIFederationSpec is the desired Keycloak state for one CITrust.
type CIFederationSpec struct {
	// TrustID is the CITrust resource id; it names the per-trust IdP mapper.
	TrustID string
	// Forge selects the token claim names ("github", "gitlab", "forgejo").
	Forge string
	// IdpAlias is the adopted CIProvider IdP alias (the exchange subject_issuer),
	// e.g. "github-actions". This reconciler never creates or deletes the IdP; it
	// is owned by the gitops realm configuration (PLAT-CITR-01).
	IdpAlias string
	// FederationClientID is the shared public (keyless) exchange requester, e.g.
	// "ci-federation".
	FederationClientID string
	// GatewayClientID is the gateway's stored Keycloak client ID; the
	// exchange audience.
	GatewayClientID string
	// Workspace is the derived forge-namespaced workspace label, e.g. "gh-acme".
	// It is informational only (reported in CITrust status and the workflow
	// hint); it is NOT the Keycloak group the role is bound to. The role grant is
	// isolated to a per-trust group (see GroupName) so sibling trusts under the
	// same owner never aggregate their roles.
	Workspace string
	// Role is the gateway client role bound to this trust's per-trust group.
	Role string
	// Subject is the compiled claim predicate.
	Subject CISubjectPredicate
	// Mode selects how the token-exchange authorization is realized (PLAT-CITR-08).
	// It is populated from the control-plane configuration, not hardcoded. An empty
	// Mode defaults to defaultTokenExchangeMode (fgap-v1), the live fleet mode.
	Mode TokenExchangeMode
}

// GroupName is the Keycloak group a matched identity lands in and the group the
// gateway role is bound to. It is keyed on the CITrust id, which uniquely
// identifies one (repository, gateway, role) tuple, so each trust's role grant
// is isolated to its own subject and its own gateway. A shared per-owner group
// would let a low-privilege repo inherit a sibling repo's higher role and leak
// roles across gateways; keying on the trust id closes that boundary. The
// Workspace label prefixes the name only for legibility in the Keycloak admin
// UI; it never widens the scope.
func (s CIFederationSpec) GroupName() string {
	return fmt.Sprintf("%s-citrust-%s", s.Workspace, s.TrustID)
}

type idpMapper struct {
	ID                     string            `json:"id,omitempty"`
	Name                   string            `json:"name"`
	IdentityProviderAlias  string            `json:"identityProviderAlias"`
	IdentityProviderMapper string            `json:"identityProviderMapper"`
	Config                 map[string]string `json:"config"`
}

type keycloakGroup struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

// EnsureCIFederation idempotently realizes artifacts (a), (b), and (c) for a
// CITrust. It is re-entrant: repeated calls converge to the same Keycloak state
// without duplicating artifacts (idempotent-reconcile requirement). A failed read of Keycloak state
// is surfaced as an error (treated as unknown, not absent) so the caller does
// not act on a transient admin-API failure (STD-RCON-08).
func (c *Client) EnsureCIFederation(ctx context.Context, spec CIFederationSpec) error {
	if err := spec.validate(); err != nil {
		return err
	}

	// The IdP must already exist (adopted, not created here). A missing IdP is a
	// configuration error, not something to paper over by creating a parallel one.
	exists, err := c.identityProviderExists(ctx, spec.IdpAlias)
	if err != nil {
		return fmt.Errorf("resolve CIProvider IdP %q: %w", spec.IdpAlias, err)
	}
	if !exists {
		return fmt.Errorf("CIProvider IdP %q not found; it must be provisioned by the gitops realm configuration before a CITrust can adopt it", spec.IdpAlias)
	}

	// Resolve the gateway client once; both the RBAC binding (c) and the
	// exchange authorization (b) target it, so a single lookup serves both.
	gatewayUUID, err := c.getClientUUID(ctx, spec.GatewayClientID)
	if err != nil {
		return fmt.Errorf("resolve gateway client %q: %w", spec.GatewayClientID, err)
	}
	if gatewayUUID == "" {
		return &ClientNotFoundError{ClientID: spec.GatewayClientID}
	}

	// (a) subject->workspace claim mapper on the IdP.
	if err := c.ensureCITrustIdpMapper(ctx, spec); err != nil {
		return fmt.Errorf("ensure CI trust subject mapper: %w", err)
	}

	// (c) workspace group + gateway role binding.
	if err := c.ensureWorkspaceRoleBinding(ctx, spec, gatewayUUID); err != nil {
		return fmt.Errorf("ensure workspace role binding: %w", err)
	}

	// (b) token-exchange authorization. The mechanism is config-driven
	// (PLAT-CITR-08): each mode realizes only its own artifacts and never calls the
	// other mode's API, so neither fails because the other's API is unavailable.
	mode := spec.Mode
	if mode == "" {
		mode = defaultTokenExchangeMode
	}
	switch mode {
	case TokenExchangeModeFGAPv1:
		// fgap-v1 mode: two fine-grained v1 grants scoped to the federation
		// requester client, one on the IdP and one on the gateway client
		// (PLAT-CITR-11). The v2 attribute is NOT set here.
		if err := c.ensureFGAPv1Grants(ctx, spec, gatewayUUID); err != nil {
			return fmt.Errorf("ensure fgap-v1 token-exchange grants: %w", err)
		}
	case TokenExchangeModeV2Attribute:
		// v2-attribute mode: opt the gateway client in as an external-to-internal
		// v2 audience. The v1 permission API is NOT called, so a 501 from it cannot
		// fail this path.
		if err := c.ensureGatewayExchangeAuthorization(ctx, spec, gatewayUUID); err != nil {
			return fmt.Errorf("ensure gateway token-exchange authorization: %w", err)
		}
	default:
		return fmt.Errorf("unknown token-exchange mode %q; want %q or %q", mode, TokenExchangeModeFGAPv1, TokenExchangeModeV2Attribute)
	}

	return nil
}

// DeleteCIFederation revokes a CITrust. It removes the per-trust subject mapper
// (a), which is the authoritative revocation: the deleted subject can no longer
// land in the trust's group, so it holds no role and is rejected default-deny
// (the CITrust-delete-revokes-exchange requirement). The per-trust group and its
// role binding (c) and the gateway client's exchange opt-in (b) are left in
// place: the empty group grants nothing without a matching subject mapper, and
// the exchange opt-in is shared by every trust on the gateway client.
//
// Tolerant delete: an empty idp_alias or trust id means there is nothing this
// reconciler can resolve to revoke (e.g. the CIProvider was already torn down, so
// its adopted alias is unknowable), and an already-gone mapper is a no-op. Since
// the WatchCITrusts stream does not resync deletes, a hard error here would wedge
// the delete forever, so these are treated as already-revoked rather than failed.
func (c *Client) DeleteCIFederation(ctx context.Context, spec CIFederationSpec) error {
	if spec.IdpAlias == "" || spec.TrustID == "" {
		slog.InfoContext(ctx, "Keycloak CI federation delete has no idp_alias to resolve, nothing to revoke", "trust_id", spec.TrustID)
		return nil
	}
	if err := c.deleteCITrustIdpMapper(ctx, spec.IdpAlias, spec.TrustID); err != nil {
		return fmt.Errorf("delete CI trust subject mapper: %w", err)
	}
	return nil
}

func (s CIFederationSpec) validate() error {
	switch {
	case s.TrustID == "":
		return fmt.Errorf("CI federation requires a trust id")
	case s.IdpAlias == "":
		return fmt.Errorf("CI federation requires an idp_alias")
	case s.GatewayClientID == "":
		return fmt.Errorf("CI federation requires a gateway client id")
	case s.Workspace == "":
		return fmt.Errorf("CI federation requires a workspace")
	case s.Role == "":
		return fmt.Errorf("CI federation requires a role")
	}
	if len(subjectClaims(s.Forge, s.Subject)) == 0 {
		return fmt.Errorf("CI federation requires at least one subject claim constraint (the trust boundary); refusing to authorize an unconstrained exchange")
	}
	// The identity claim (repository/owner for github/forgejo, project_path for
	// gitlab) is the real trust boundary: a trust constrained only by ref or
	// environment would admit ANY repository building that ref. Require it
	// explicitly so a ref/environment-only subject is rejected, not quietly
	// widened to every repo.
	switch s.Forge {
	case "gitlab":
		if s.Subject.ProjectPath == "" {
			return fmt.Errorf("gitlab CI federation requires a project_path subject (the trust boundary); a ref/environment-only trust is refused")
		}
	default: // github, forgejo
		if s.Subject.Repository == "" && s.Subject.RepositoryOwner == "" {
			return fmt.Errorf("%s CI federation requires a repository or repository_owner subject (the trust boundary); a ref/environment-only trust is refused", s.Forge)
		}
	}
	return nil
}

// subjectClaims compiles the forge token claims a brokered token must carry,
// as the {key,value} pairs the advanced-claim IdP mapper matches. GitHub and
// Forgejo expose repository / repository_owner; GitLab exposes project_path.
// Only set fields are constrained.
func subjectClaims(forge string, s CISubjectPredicate) []map[string]string {
	claims := make([]map[string]string, 0, 5)
	add := func(key, value string) {
		if value != "" {
			claims = append(claims, map[string]string{"key": key, "value": value})
		}
	}
	switch forge {
	case "gitlab":
		add("project_path", s.ProjectPath)
		add("ref", s.Ref)
		add("environment", s.Environment)
		if s.RefProtected {
			add("ref_protected", "true")
		}
	default: // github, forgejo
		add("repository", s.Repository)
		add("repository_owner", s.RepositoryOwner)
		add("ref", s.Ref)
		add("environment", s.Environment)
		if s.RefProtected {
			add("ref_protected", "true")
		}
	}
	return claims
}

func (c *Client) ensureCITrustIdpMapper(ctx context.Context, spec CIFederationSpec) error {
	name := ciTrustMapperPrefix + spec.TrustID

	claims, err := json.Marshal(subjectClaims(spec.Forge, spec.Subject))
	if err != nil {
		return fmt.Errorf("marshal subject claims: %w", err)
	}

	desired := idpMapper{
		Name:                   name,
		IdentityProviderAlias:  spec.IdpAlias,
		IdentityProviderMapper: advancedGroupIdpMapper,
		Config: map[string]string{
			"claims":                 string(claims),
			"are.claim.values.regex": "false",
			// Land the matched identity in this trust's OWN per-trust group, not a
			// shared per-owner workspace group, so the role binding (c) cannot leak
			// across sibling repos or gateways.
			"group":    "/" + spec.GroupName(),
			"syncMode": "FORCE",
		},
	}

	existing, err := c.findIdpMapper(ctx, spec.IdpAlias, name)
	if err != nil {
		return err
	}
	base := fmt.Sprintf("/admin/realms/%s/identity-provider/instances/%s/mappers",
		c.realm, url.PathEscape(spec.IdpAlias))
	if existing != nil {
		// PUT the desired representation over the existing mapper so a changed
		// subject predicate or workspace is corrected in place (Keycloak requires
		// the id in both the path and the body).
		desired.ID = existing.ID
		body, _ := json.Marshal(desired)
		if _, err := c.doRequest(ctx, http.MethodPut, base+"/"+url.PathEscape(existing.ID), body); err != nil {
			return fmt.Errorf("update IdP mapper %q: %w", name, err)
		}
		return nil
	}
	body, _ := json.Marshal(desired)
	if _, err := c.doRequest(ctx, http.MethodPost, base, body); err != nil {
		return fmt.Errorf("create IdP mapper %q: %w", name, err)
	}
	return nil
}

func (c *Client) deleteCITrustIdpMapper(ctx context.Context, idpAlias, trustID string) error {
	name := ciTrustMapperPrefix + trustID
	existing, err := c.findIdpMapper(ctx, idpAlias, name)
	if err != nil {
		return err
	}
	if existing == nil {
		slog.InfoContext(ctx, "Keycloak CI trust mapper not found on IdP, nothing to delete", "name", name, "idp_alias", idpAlias)
		return nil
	}
	path := fmt.Sprintf("/admin/realms/%s/identity-provider/instances/%s/mappers/%s",
		c.realm, url.PathEscape(idpAlias), url.PathEscape(existing.ID))
	if _, err := c.doRequest(ctx, http.MethodDelete, path, nil); err != nil {
		return fmt.Errorf("delete IdP mapper %q: %w", name, err)
	}
	return nil
}

func (c *Client) findIdpMapper(ctx context.Context, idpAlias, name string) (*idpMapper, error) {
	path := fmt.Sprintf("/admin/realms/%s/identity-provider/instances/%s/mappers",
		c.realm, url.PathEscape(idpAlias))
	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("list IdP mappers on %q: %w", idpAlias, err)
	}
	var mappers []idpMapper
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &mappers); err != nil {
			return nil, fmt.Errorf("parse IdP mappers on %q: %w", idpAlias, err)
		}
	}
	for i := range mappers {
		if mappers[i].Name == name {
			return &mappers[i], nil
		}
	}
	return nil, nil
}

func (c *Client) identityProviderExists(ctx context.Context, idpAlias string) (bool, error) {
	path := fmt.Sprintf("/admin/realms/%s/identity-provider/instances/%s",
		c.realm, url.PathEscape(idpAlias))
	resp, err := c.doRequestRaw(ctx, http.MethodGet, path, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("get IdP %q returned %d", idpAlias, resp.StatusCode)
	}
}

// ensureWorkspaceRoleBinding realizes artifact (c): the trust's OWN per-trust
// group exists and carries the gateway client role. Idempotent: an existing
// group and an already-present role mapping are no-ops. The group is keyed on the
// CITrust id (GroupName), so the role is granted only to the identity this
// trust's mapper admits, on this trust's gateway - never aggregated onto a shared
// per-owner group. gatewayUUID is the already-resolved Keycloak UUID of the
// gateway client.
func (c *Client) ensureWorkspaceRoleBinding(ctx context.Context, spec CIFederationSpec, gatewayUUID string) error {
	group := spec.GroupName()
	groupID, err := c.ensureGroup(ctx, group)
	if err != nil {
		return err
	}

	roleUUID, err := c.getFederationClientRoleUUID(ctx, gatewayUUID, spec.GatewayClientID, spec.Role)
	if err != nil {
		return fmt.Errorf("resolve role %q on gateway client %q: %w", spec.Role, spec.GatewayClientID, err)
	}
	if roleUUID == "" {
		// A 200 with no id means the role record is not usable yet (provisioning
		// race); surface the retryable not-found path rather than a terminal error.
		return &ClientNotFoundError{ClientID: spec.GatewayClientID}
	}

	roles := []keycloakRole{{ID: roleUUID, Name: spec.Role}}
	body, _ := json.Marshal(roles)
	// POSTing an already-present group role mapping is a Keycloak no-op, so this
	// converges without a pre-read.
	path := fmt.Sprintf("/admin/realms/%s/groups/%s/role-mappings/clients/%s",
		c.realm, url.PathEscape(groupID), url.PathEscape(gatewayUUID))
	if _, err := c.doRequest(ctx, http.MethodPost, path, body); err != nil {
		return fmt.Errorf("bind role %q to per-trust group %q on gateway client %q: %w",
			spec.Role, group, spec.GatewayClientID, err)
	}
	return nil
}

// getFederationClientRoleUUID resolves a gateway client role UUID, mapping a 404
// to the retryable ClientNotFoundError path. The client can exist before its
// roles are provisioned (a reconcile race), and doRequest collapses every status
// >= 400 - including 404 - into a generic error, which would surface the race as
// a terminal "reconcile keycloak federation" failure. Inspecting the raw status
// here lets the caller reschedule via errors.As(*ClientNotFoundError).
func (c *Client) getFederationClientRoleUUID(ctx context.Context, clientUUID, clientID, roleName string) (string, error) {
	path := fmt.Sprintf("/admin/realms/%s/clients/%s/roles/%s",
		c.realm, url.PathEscape(clientUUID), url.PathEscape(roleName))
	resp, err := c.doRequestRaw(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return "", &ClientNotFoundError{ClientID: clientID}
	}
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("get role %q on client %s returned %d: %s", roleName, clientID, resp.StatusCode, string(respBody))
	}
	var role keycloakRole
	if err := json.Unmarshal(respBody, &role); err != nil {
		return "", fmt.Errorf("parse role %q: %w", roleName, err)
	}
	return role.ID, nil
}

// ensureGroup returns the id of a top-level realm group with the given name,
// creating it if absent. A 409 on create means a concurrent reconcile won the
// race, so it re-reads rather than failing.
func (c *Client) ensureGroup(ctx context.Context, name string) (string, error) {
	if id, err := c.findGroup(ctx, name); err != nil {
		return "", err
	} else if id != "" {
		return id, nil
	}
	payload, _ := json.Marshal(keycloakGroup{Name: name})
	path := fmt.Sprintf("/admin/realms/%s/groups", c.realm)
	resp, err := c.doRequestRaw(ctx, http.MethodPost, path, payload)
	if err != nil {
		return "", fmt.Errorf("create group %q: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusConflict {
		id, err := c.findGroup(ctx, name)
		if err != nil {
			return "", err
		}
		if id == "" {
			return "", fmt.Errorf("create group %q returned 409 but the group was not found", name)
		}
		return id, nil
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create group %q returned %d", name, resp.StatusCode)
	}
	id, err := c.findGroup(ctx, name)
	if err != nil {
		return "", err
	}
	if id == "" {
		return "", fmt.Errorf("group %q not found immediately after create", name)
	}
	return id, nil
}

func (c *Client) findGroup(ctx context.Context, name string) (string, error) {
	path := fmt.Sprintf("/admin/realms/%s/groups?search=%s&exact=true", c.realm, url.QueryEscape(name))
	body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("search group %q: %w", name, err)
	}
	var groups []keycloakGroup
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &groups); err != nil {
			return "", fmt.Errorf("parse group search for %q: %w", name, err)
		}
	}
	for _, g := range groups {
		if g.Name == name {
			return g.ID, nil
		}
	}
	return "", nil
}

// ensureGatewayExchangeAuthorization realizes artifact (b) in v2-attribute mode: it
// opts the gateway client in as a token-exchange audience so the public federation
// client may exchange a brokered IdP token onto it (external-to-internal v2) by
// setting the standard.token.exchange.enabled client attribute. It GET-merges the
// attribute onto the existing client representation so unrelated settings are
// preserved, and is a no-op when already enabled. It never calls the v1 permission
// API, so a Keycloak whose v1 API returns 501 does not fail this path
// (PLAT-CITR-08.02). The per-requester scoping that v2-attribute mode cannot express
// is provided instead by fgap-v1 mode (ensureFGAPv1Grants, PLAT-CITR-11).
func (c *Client) ensureGatewayExchangeAuthorization(ctx context.Context, spec CIFederationSpec, gatewayUUID string) error {
	path := fmt.Sprintf("/admin/realms/%s/clients/%s", c.realm, url.PathEscape(gatewayUUID))
	respBody, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return fmt.Errorf("get gateway client %q: %w", spec.GatewayClientID, err)
	}
	var representation map[string]json.RawMessage
	if err := json.Unmarshal(respBody, &representation); err != nil {
		return fmt.Errorf("parse gateway client %q: %w", spec.GatewayClientID, err)
	}

	attributes := make(map[string]string)
	if raw, ok := representation["attributes"]; ok &&
		len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err := json.Unmarshal(raw, &attributes); err != nil {
			return fmt.Errorf("parse attributes for gateway client %q: %w", spec.GatewayClientID, err)
		}
	}
	if attributes[standardTokenExchangeEnabledAttribute] == "true" {
		return nil
	}
	attributes[standardTokenExchangeEnabledAttribute] = "true"

	rawAttributes, err := json.Marshal(attributes)
	if err != nil {
		return fmt.Errorf("marshal attributes for gateway client %q: %w", spec.GatewayClientID, err)
	}
	representation["attributes"] = rawAttributes
	body, err := json.Marshal(representation)
	if err != nil {
		return fmt.Errorf("marshal gateway client %q: %w", spec.GatewayClientID, err)
	}
	if _, err := c.doRequest(ctx, http.MethodPut, path, body); err != nil {
		return fmt.Errorf("enable token exchange on gateway client %q: %w", spec.GatewayClientID, err)
	}
	return nil
}

// ensureFGAPv1Grants realizes artifact (b) in fgap-v1 mode (PLAT-CITR-11): the two
// fine-grained admin authorization v1 token-exchange grants that gate an
// external-to-internal exchange, each scoped to the shared public federation
// requester client. Both grants reuse the same generalized building blocks the e2e
// impersonation helper uses (enable management permissions -> ensure a client
// policy naming the requester -> attach the policy to the token-exchange scope
// permission on realm-management), generalized from the fixed hypershell-e2e
// requester to the CI federation requester client:
//
//  1. IdP token-exchange grant: enable fine-grained authorization on the
//     CIProvider IdP and attach the requester's client policy to the IdP's
//     token-exchange scope permission. Without it the IdP-level exchange check
//     denies the exchange.
//  2. Target-client token-exchange grant: enable fine-grained authorization on the
//     target gateway client and attach the SAME requester client policy to that
//     client's token-exchange scope permission. Without it the target-client-level
//     exchange check denies the exchange.
//
// The grant is scoped to the requester client: only the named federation client,
// not any client, is authorized to exchange onto the IdP and onto the gateway
// client. This resolves the per-requester exchange-scoping the reconciler left
// open. Every step is idempotent (find-or-create a single policy, a management
// permission, and a scope permission association), so a repeated reconcile makes
// no duplicate policy or permission (PLAT-CITR-09). gatewayUUID is the
// already-resolved Keycloak UUID of the gateway client.
func (c *Client) ensureFGAPv1Grants(ctx context.Context, spec CIFederationSpec, gatewayUUID string) error {
	// The federation requester must exist and be enabled; it is the shared public
	// (keyless) client the gitops realm configuration provisions, and the policy
	// names it so only it may exchange. A missing or disabled requester is a
	// configuration error, surfaced so it is logged and retried on the next watch
	// reseed rather than silently granting nothing.
	requester, err := c.getClient(ctx, spec.FederationClientID)
	if err != nil {
		return fmt.Errorf("look up federation requester client %q: %w", spec.FederationClientID, err)
	}
	if requester == nil || requester.ID == "" {
		return fmt.Errorf("federation requester client %q not found; the gitops realm configuration must provision it before a CITrust can grant token-exchange", spec.FederationClientID)
	}
	if !requester.Enabled {
		return fmt.Errorf("federation requester client %q is disabled; cannot grant token-exchange", spec.FederationClientID)
	}

	rmUUID, err := c.getClientUUID(ctx, realmManagementClientID)
	if err != nil {
		return fmt.Errorf("look up %s client: %w", realmManagementClientID, err)
	}
	if rmUUID == "" {
		return fmt.Errorf("keycloak client %s not found", realmManagementClientID)
	}

	// A single client policy names the requester and is reused for both grants, so a
	// re-run finds it rather than creating a duplicate.
	policyID, err := c.ensureTokenExchangeClientPolicy(ctx, rmUUID, requester.ID, tokenExchangePolicyName(spec.FederationClientID))
	if err != nil {
		return err
	}

	// (b.1) IdP token-exchange grant.
	idpPermID, err := c.enableIdpTokenExchangePermissions(ctx, spec.IdpAlias)
	if err != nil {
		return fmt.Errorf("enable fine-grained authorization on IdP %q: %w", spec.IdpAlias, err)
	}
	if err := c.attachPolicyToPermission(ctx, rmUUID, idpPermID, policyID); err != nil {
		return fmt.Errorf("grant token-exchange on IdP %q: %w", spec.IdpAlias, err)
	}

	// (b.2) target gateway client token-exchange grant.
	gatewayPermID, err := c.enableTokenExchangePermissions(ctx, gatewayUUID)
	if err != nil {
		return fmt.Errorf("enable fine-grained authorization on gateway client %q: %w", spec.GatewayClientID, err)
	}
	if err := c.attachPolicyToPermission(ctx, rmUUID, gatewayPermID, policyID); err != nil {
		return fmt.Errorf("grant token-exchange on gateway client %q: %w", spec.GatewayClientID, err)
	}
	return nil
}

// OwnerFromRepository returns the owner segment of an "owner/repo" repository
// string (the GitHub/Forgejo repository_owner claim), or the input unchanged
// when it carries no slash.
func OwnerFromRepository(repository string) string {
	if idx := strings.Index(repository, "/"); idx >= 0 {
		return repository[:idx]
	}
	return repository
}
