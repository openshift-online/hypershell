package keycloak

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	testIdpAlias      = "github-actions"
	testFederationID  = "ci-federation"
	testWorkspace     = "gh-acme"
	testGatewayRoleID = "role-openshell-user-uuid"

	// fgap-v1 fixtures (PLAT-CITR-11). The realm-management client hosts the
	// authorization resource server; the IdP and gateway client each have a
	// token-exchange scope permission the federation requester's client policy is
	// attached to.
	testRealmMgmtUUID   = "realm-management-uuid"
	testFederationUUID  = "ci-federation-uuid"
	testIdpTEPermID     = "idp-te-perm"
	testGatewayTEPermID = "gw-te-perm"
	testFedPolicyID     = "fed-te-policy"
)

// fakeCIKeycloak is a stateful httptest fake of the Keycloak admin REST surface
// the CI federation code drives. It records every write so tests can assert the
// exact artifacts created, and it serves reads from its own state so a second
// Ensure pass observes what the first created (idempotency).
type fakeCIKeycloak struct {
	mu sync.Mutex

	idpExists bool
	// idpConfig is the IdP instance's config block. disableTypeClaimCheck is owned
	// declaratively by the gitops Terraform IdP config (PLAT-CITR-12); a PUT to the
	// IdP instance would replace this map, so a test can prove the reconciler never
	// clobbers it.
	idpConfig map[string]string
	// idpMappers maps mapper name -> representation, keyed as Keycloak would.
	idpMappers map[string]idpMapper
	nextID     int

	// groups maps group name -> group id, as Keycloak would after create.
	groups map[string]string

	// gatewayClientExists controls whether the gateway clientId resolves.
	gatewayClientExists bool
	// gatewayAttrs is the current gateway client attribute map (PUT target).
	gatewayAttrs map[string]string
	// roleExists controls whether the role resolves on the gateway client.
	roleExists bool

	// groupRoleMappings records POSTed role bindings keyed by group id, so a test
	// can assert which group a role landed on (cross-trust isolation).
	groupRoleMappings map[string][][]keycloakRole

	// fgap-v1 state (PLAT-CITR-11).
	// federationClientExists / federationClientEnabled control the shared requester.
	federationClientExists  bool
	federationClientEnabled bool
	// fgapEnabled records every management/permissions resource that had
	// fine-grained authorization enabled (the IdP alias and the gateway client UUID).
	fgapEnabled []string
	// policyCreates counts federation client-policy POSTs (idempotency: want 1).
	policyCreates int
	policyExists  bool
	// permPolicies maps a token-exchange scope-permission id -> set of attached
	// policy ids, so a test can assert each grant carries the requester policy.
	permPolicies map[string]map[string]bool
	// permPutCount counts permission/scope PUTs per permission id (idempotency: a
	// re-run that finds the policy already attached must not PUT again).
	permPutCount map[string]int
	// v1PermAPIReturns501 makes the v1 management/permissions endpoints answer 501,
	// as a fleet Keycloak with fine-grained admin authorization v1 disabled would.
	// v2-attribute mode must never reach them (PLAT-CITR-08.02).
	v1PermAPIReturns501 bool

	requests []string // method+path trail, for ordering assertions
}

func newFakeCIKeycloak(t *testing.T) (*httptest.Server, *fakeCIKeycloak) {
	t.Helper()
	state := &fakeCIKeycloak{
		idpExists:           true,
		idpConfig:           map[string]string{"disableTypeClaimCheck": "true"},
		idpMappers:          map[string]idpMapper{},
		groups:              map[string]string{},
		gatewayClientExists: true,
		gatewayAttrs:        map[string]string{},
		roleExists:          true,
		groupRoleMappings:   map[string][][]keycloakRole{},

		federationClientExists:  true,
		federationClientEnabled: true,
		permPolicies:            map[string]map[string]bool{},
		permPutCount:            map[string]int{},
	}

	mux := http.NewServeMux()

	mux.HandleFunc(fmt.Sprintf("/realms/%s/protocol/openid-connect/token", testRealm), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fake-token", "expires_in": 300})
	})

	// IdP instance existence.
	mux.HandleFunc(fmt.Sprintf("/admin/realms/%s/identity-provider/instances/%s", testRealm, testIdpAlias), func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		state.requests = append(state.requests, r.Method+" idp")
		if !state.idpExists {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method == http.MethodPut {
			// Model a clobbering IdP write: replace the config with the PUT body's
			// config. The reconciler must never do this, so this branch should never
			// run during a CITrust reconcile (asserted by PLAT-CITR-12 test).
			var rep struct {
				Config map[string]string `json:"config"`
			}
			_ = json.NewDecoder(r.Body).Decode(&rep)
			state.idpConfig = rep.Config
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"alias": testIdpAlias, "config": state.idpConfig})
	})

	// IdP mappers collection (GET list, POST create).
	mappersBase := fmt.Sprintf("/admin/realms/%s/identity-provider/instances/%s/mappers", testRealm, testIdpAlias)
	mux.HandleFunc(mappersBase, func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			state.requests = append(state.requests, "GET mappers")
			out := make([]idpMapper, 0, len(state.idpMappers))
			for _, m := range state.idpMappers {
				out = append(out, m)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(out)
		case http.MethodPost:
			state.requests = append(state.requests, "POST mappers")
			var m idpMapper
			_ = json.NewDecoder(r.Body).Decode(&m)
			state.nextID++
			m.ID = fmt.Sprintf("mapper-%d", state.nextID)
			state.idpMappers[m.Name] = m
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// IdP mapper by id (PUT update, DELETE).
	mux.HandleFunc(mappersBase+"/", func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		id := strings.TrimPrefix(r.URL.Path, mappersBase+"/")
		switch r.Method {
		case http.MethodPut:
			state.requests = append(state.requests, "PUT mapper")
			var m idpMapper
			_ = json.NewDecoder(r.Body).Decode(&m)
			state.idpMappers[m.Name] = m
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			state.requests = append(state.requests, "DELETE mapper")
			for name, m := range state.idpMappers {
				if m.ID == id {
					delete(state.idpMappers, name)
				}
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Groups collection (GET search, POST create). Group ids are tracked by name
	// so a test can observe which per-trust group a role is bound to.
	groupsBase := fmt.Sprintf("/admin/realms/%s/groups", testRealm)
	mux.HandleFunc(groupsBase, func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			state.requests = append(state.requests, "GET groups")
			w.Header().Set("Content-Type", "application/json")
			search := r.URL.Query().Get("search")
			if id, ok := state.groups[search]; ok {
				_ = json.NewEncoder(w).Encode([]keycloakGroup{{ID: id, Name: search}})
				return
			}
			_ = json.NewEncoder(w).Encode([]keycloakGroup{})
		case http.MethodPost:
			state.requests = append(state.requests, "POST groups")
			var g keycloakGroup
			_ = json.NewDecoder(r.Body).Decode(&g)
			state.nextID++
			state.groups[g.Name] = fmt.Sprintf("group-%d", state.nextID)
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Clients collection -- resolve gateway / realm-management / federation
	// requester clientId -> uuid.
	mux.HandleFunc(fmt.Sprintf("/admin/realms/%s/clients", testRealm), func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("clientId") {
		case testGatewayClientID:
			if state.gatewayClientExists {
				_ = json.NewEncoder(w).Encode([]keycloakClient{{ID: testGatewayUUID, ClientID: testGatewayClientID}})
				return
			}
		case realmManagementClientID:
			_ = json.NewEncoder(w).Encode([]keycloakClient{{ID: testRealmMgmtUUID, ClientID: realmManagementClientID}})
			return
		case testFederationID:
			if state.federationClientExists {
				_ = json.NewEncoder(w).Encode([]keycloakClient{{ID: testFederationUUID, ClientID: testFederationID, Enabled: state.federationClientEnabled}})
				return
			}
		}
		_ = json.NewEncoder(w).Encode([]keycloakClient{})
	})

	// fgap-v1: management/permissions on the IdP and on the gateway client (enable
	// fine-grained authorization, return the token-exchange scope permission id).
	idpPermPath := fmt.Sprintf("/admin/realms/%s/identity-provider/instances/%s/management/permissions", testRealm, testIdpAlias)
	mux.HandleFunc(idpPermPath, func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		state.requests = append(state.requests, r.Method+" idp mgmt-permissions")
		if state.v1PermAPIReturns501 {
			http.Error(w, "not implemented", http.StatusNotImplemented)
			return
		}
		if r.Method == http.MethodPut {
			state.fgapEnabled = append(state.fgapEnabled, testIdpAlias)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(managementPermissions{
			Enabled:          true,
			ScopePermissions: map[string]string{tokenExchangeScope: testIdpTEPermID},
		})
	})
	gwPermPath := fmt.Sprintf("/admin/realms/%s/clients/%s/management/permissions", testRealm, testGatewayUUID)
	mux.HandleFunc(gwPermPath, func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		state.requests = append(state.requests, r.Method+" gw mgmt-permissions")
		if state.v1PermAPIReturns501 {
			http.Error(w, "not implemented", http.StatusNotImplemented)
			return
		}
		if r.Method == http.MethodPut {
			state.fgapEnabled = append(state.fgapEnabled, testGatewayUUID)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(managementPermissions{
			Enabled:          true,
			ScopePermissions: map[string]string{tokenExchangeScope: testGatewayTEPermID},
		})
	})

	// fgap-v1: realm-management authorization resource server -- client policy
	// find/create and token-exchange scope-permission read/attach.
	authzBase := fmt.Sprintf("/admin/realms/%s/clients/%s/authz/resource-server/", testRealm, testRealmMgmtUUID)
	mux.HandleFunc(authzBase, func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		rest := strings.TrimPrefix(r.URL.Path, authzBase)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case rest == "policy/search" && r.Method == http.MethodGet:
			if !state.policyExists {
				// Keycloak returns 404 from policy search-by-name when none exists.
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "HTTP 404 Not Found"})
				return
			}
			_ = json.NewEncoder(w).Encode(authzPolicy{ID: testFedPolicyID, Name: tokenExchangePolicyName(testFederationID)})
		case rest == "policy/client" && r.Method == http.MethodPost:
			var payload struct {
				Name    string   `json:"name"`
				Clients []string `json:"clients"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload.Name != tokenExchangePolicyName(testFederationID) {
				t.Errorf("policy name = %q, want %q", payload.Name, tokenExchangePolicyName(testFederationID))
			}
			if len(payload.Clients) != 1 || payload.Clients[0] != testFederationUUID {
				t.Errorf("policy clients = %v, want [%s] (scoped to the requester)", payload.Clients, testFederationUUID)
			}
			state.policyCreates++
			state.policyExists = true
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(authzPolicy{ID: testFedPolicyID, Name: tokenExchangePolicyName(testFederationID)})
		case strings.HasSuffix(rest, "/associatedPolicies") && r.Method == http.MethodGet:
			permID := strings.TrimSuffix(strings.TrimPrefix(rest, "policy/"), "/associatedPolicies")
			out := make([]authzPolicy, 0)
			for id := range state.permPolicies[permID] {
				out = append(out, authzPolicy{ID: id, Name: tokenExchangePolicyName(testFederationID)})
			}
			_ = json.NewEncoder(w).Encode(out)
		case strings.HasPrefix(rest, "permission/scope/") && r.Method == http.MethodGet:
			permID := strings.TrimPrefix(rest, "permission/scope/")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": permID, "name": "token-exchange.permission", "type": "scope",
				"logic": "POSITIVE", "decisionStrategy": "UNANIMOUS",
			})
		case strings.HasPrefix(rest, "permission/scope/") && r.Method == http.MethodPut:
			permID := strings.TrimPrefix(rest, "permission/scope/")
			var rep struct {
				Policies []string `json:"policies"`
			}
			_ = json.NewDecoder(r.Body).Decode(&rep)
			if state.permPolicies[permID] == nil {
				state.permPolicies[permID] = map[string]bool{}
			}
			for _, id := range rep.Policies {
				state.permPolicies[permID][id] = true
			}
			state.permPutCount[permID]++
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "unexpected authz request "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	})

	// Gateway client role resolution.
	mux.HandleFunc(fmt.Sprintf("/admin/realms/%s/clients/%s/roles/", testRealm, testGatewayUUID), func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		if !state.roleExists {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		roleName := strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/admin/realms/%s/clients/%s/roles/", testRealm, testGatewayUUID))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keycloakRole{ID: testGatewayRoleID, Name: roleName})
	})

	// Gateway client representation GET/PUT (exchange authorization attribute).
	mux.HandleFunc(fmt.Sprintf("/admin/realms/%s/clients/%s", testRealm, testGatewayUUID), func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			state.requests = append(state.requests, "GET gateway client")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":         testGatewayUUID,
				"clientId":   testGatewayClientID,
				"attributes": state.gatewayAttrs,
			})
		case http.MethodPut:
			state.requests = append(state.requests, "PUT gateway client")
			var rep map[string]json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&rep)
			if raw, ok := rep["attributes"]; ok {
				_ = json.Unmarshal(raw, &state.gatewayAttrs)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Group role-mappings onto the gateway client. Registered as a prefix handler
	// so any per-trust group id is accepted; the group id is parsed from the path
	// and the binding recorded against it (cross-trust isolation assertions).
	groupsPrefix := groupsBase + "/"
	mux.HandleFunc(groupsPrefix, func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		rest := strings.TrimPrefix(r.URL.Path, groupsPrefix)
		marker := "/role-mappings/clients/"
		idx := strings.Index(rest, marker)
		if idx < 0 || r.Method != http.MethodPost {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		groupID := rest[:idx]
		state.requests = append(state.requests, "POST group role-mapping")
		body, _ := io.ReadAll(r.Body)
		var roles []keycloakRole
		_ = json.Unmarshal(body, &roles)
		state.groupRoleMappings[groupID] = append(state.groupRoleMappings[groupID], roles)
		w.WriteHeader(http.StatusNoContent)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, state
}

func ciTestClient(server *httptest.Server) *Client {
	return NewClient(server.URL, testRealm, testAdminClientID, testAdminSecret)
}

func githubSpec() CIFederationSpec {
	return CIFederationSpec{
		TrustID:            "trust-123",
		Forge:              "github",
		IdpAlias:           testIdpAlias,
		FederationClientID: testFederationID,
		GatewayClientID:    testGatewayClientID,
		Workspace:          testWorkspace,
		Role:               "openshell-user",
		// The base spec exercises v2-attribute mode; the fgap-v1 tests override Mode.
		Mode: TokenExchangeModeV2Attribute,
		Subject: CISubjectPredicate{
			Repository:      "acme/app",
			RepositoryOwner: "acme",
		},
	}
}

// githubFGAPSpec is the base github spec in fgap-v1 token-exchange mode.
func githubFGAPSpec() CIFederationSpec {
	s := githubSpec()
	s.Mode = TokenExchangeModeFGAPv1
	return s
}

func TestEnsureCIFederation_CreatesAllArtifacts(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	if err := client.EnsureCIFederation(t.Context(), githubSpec()); err != nil {
		t.Fatalf("EnsureCIFederation: %v", err)
	}

	// (a) per-trust subject mapper created with the compiled claim predicate.
	mapper, ok := state.idpMappers["citrust-trust-123"]
	if !ok {
		t.Fatalf("subject IdP mapper not created; mappers=%v", state.idpMappers)
	}
	if mapper.IdentityProviderMapper != advancedGroupIdpMapper {
		t.Errorf("mapper type = %q, want %q", mapper.IdentityProviderMapper, advancedGroupIdpMapper)
	}
	// The mapper lands the identity in this trust's OWN per-trust group, not the
	// shared per-owner workspace label.
	wantGroup := "/" + githubSpec().GroupName()
	if got := mapper.Config["group"]; got != wantGroup {
		t.Errorf("mapper group = %q, want per-trust group %q", got, wantGroup)
	}
	if mapper.Config["group"] == "/"+testWorkspace {
		t.Errorf("mapper must not target the shared per-owner group %q", testWorkspace)
	}
	var claims []map[string]string
	if err := json.Unmarshal([]byte(mapper.Config["claims"]), &claims); err != nil {
		t.Fatalf("parse claims: %v", err)
	}
	assertClaim(t, claims, "repository", "acme/app")
	assertClaim(t, claims, "repository_owner", "acme")

	// (c) gateway role bound onto exactly one per-trust group.
	if len(state.groupRoleMappings) != 1 {
		t.Fatalf("role bound to %d groups, want exactly one per-trust group; got %v", len(state.groupRoleMappings), state.groupRoleMappings)
	}
	for _, bindings := range state.groupRoleMappings {
		if len(bindings) != 1 || len(bindings[0]) != 1 || bindings[0][0].Name != "openshell-user" {
			t.Errorf("group role bindings = %v, want one openshell-user binding", bindings)
		}
	}

	// (b) gateway client opted in as a token-exchange audience.
	if state.gatewayAttrs[standardTokenExchangeEnabledAttribute] != "true" {
		t.Errorf("gateway exchange attribute = %q, want true", state.gatewayAttrs[standardTokenExchangeEnabledAttribute])
	}
}

func TestEnsureCIFederation_Idempotent(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	for pass := 1; pass <= 2; pass++ {
		if err := client.EnsureCIFederation(t.Context(), githubSpec()); err != nil {
			t.Fatalf("EnsureCIFederation pass %d: %v", pass, err)
		}
	}

	// Exactly one mapper, created once then updated in place (no duplicate).
	if len(state.idpMappers) != 1 {
		t.Errorf("idpMappers = %d, want 1 after two passes", len(state.idpMappers))
	}
	posts, puts := 0, 0
	for _, req := range state.requests {
		switch req {
		case "POST mappers":
			posts++
		case "PUT mapper":
			puts++
		}
	}
	if posts != 1 {
		t.Errorf("mapper POSTs = %d, want 1 (create once)", posts)
	}
	if puts != 1 {
		t.Errorf("mapper PUTs = %d, want 1 (update on second pass)", puts)
	}
	// The group is created once; the second pass finds it.
	groupPosts := 0
	for _, req := range state.requests {
		if req == "POST groups" {
			groupPosts++
		}
	}
	if groupPosts != 1 {
		t.Errorf("group POSTs = %d, want 1", groupPosts)
	}
	// The gateway client is PUT only on the first pass; the second finds the
	// attribute already set and skips the write.
	gwPuts := 0
	for _, req := range state.requests {
		if req == "PUT gateway client" {
			gwPuts++
		}
	}
	if gwPuts != 1 {
		t.Errorf("gateway client PUTs = %d, want 1 (no rewrite when already enabled)", gwPuts)
	}
}

func TestEnsureCIFederation_MissingIdPFailsClosed(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	state.idpExists = false
	client := ciTestClient(server)

	err := client.EnsureCIFederation(t.Context(), githubSpec())
	if err == nil {
		t.Fatal("expected error when the CIProvider IdP is absent")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want it to name the missing IdP", err)
	}
	if len(state.idpMappers) != 0 {
		t.Errorf("no mapper should be created when the IdP is absent; got %v", state.idpMappers)
	}
}

func TestEnsureCIFederation_MissingGatewayClientIsRetryable(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	state.gatewayClientExists = false
	client := ciTestClient(server)

	err := client.EnsureCIFederation(t.Context(), githubSpec())
	if err == nil {
		t.Fatal("expected error when the gateway client is not provisioned")
	}
	var notFound *ClientNotFoundError
	if !errors.As(err, &notFound) {
		t.Errorf("error = %v, want a ClientNotFoundError so the caller retries", err)
	}
}

func TestEnsureCIFederation_RejectsUnconstrainedSubject(t *testing.T) {
	server, _ := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	spec := githubSpec()
	spec.Subject = CISubjectPredicate{} // no claims -> no trust boundary
	if err := client.EnsureCIFederation(t.Context(), spec); err == nil {
		t.Fatal("expected an unconstrained subject (no claims) to be rejected")
	}
}

func TestEnsureCIFederation_GitLabUsesProjectPathClaim(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	spec := githubSpec()
	spec.Forge = "gitlab"
	spec.Subject = CISubjectPredicate{ProjectPath: "acme/app"}
	if err := client.EnsureCIFederation(t.Context(), spec); err != nil {
		t.Fatalf("EnsureCIFederation: %v", err)
	}
	mapper := state.idpMappers["citrust-trust-123"]
	var claims []map[string]string
	_ = json.Unmarshal([]byte(mapper.Config["claims"]), &claims)
	assertClaim(t, claims, "project_path", "acme/app")
	for _, c := range claims {
		if c["key"] == "repository" {
			t.Errorf("gitlab mapper must not carry a repository claim; got %v", claims)
		}
	}
}

func TestEnsureCIFederation_TighterClaimsIncluded(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	spec := githubSpec()
	spec.Subject = CISubjectPredicate{
		Repository:      "acme/app",
		RepositoryOwner: "acme",
		Environment:     "production",
		Ref:             "refs/heads/main",
		RefProtected:    true,
	}
	if err := client.EnsureCIFederation(t.Context(), spec); err != nil {
		t.Fatalf("EnsureCIFederation: %v", err)
	}
	mapper := state.idpMappers["citrust-trust-123"]
	var claims []map[string]string
	_ = json.Unmarshal([]byte(mapper.Config["claims"]), &claims)
	assertClaim(t, claims, "environment", "production")
	assertClaim(t, claims, "ref", "refs/heads/main")
	assertClaim(t, claims, "ref_protected", "true")
}

func TestDeleteCIFederation_RemovesSubjectMapper(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	if err := client.EnsureCIFederation(t.Context(), githubSpec()); err != nil {
		t.Fatalf("EnsureCIFederation: %v", err)
	}
	if _, ok := state.idpMappers["citrust-trust-123"]; !ok {
		t.Fatal("precondition: mapper should exist")
	}

	if err := client.DeleteCIFederation(t.Context(), githubSpec()); err != nil {
		t.Fatalf("DeleteCIFederation: %v", err)
	}
	if _, ok := state.idpMappers["citrust-trust-123"]; ok {
		t.Error("subject mapper should be removed on delete")
	}
	// The per-trust group binding is left in place (inert without the mapper); the
	// mapper removal is the authoritative revocation.
	if len(state.groupRoleMappings) == 0 {
		t.Error("per-trust group binding should survive the mapper delete")
	}
}

func TestDeleteCIFederation_MissingMapperIsNoOp(t *testing.T) {
	server, _ := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	if err := client.DeleteCIFederation(t.Context(), githubSpec()); err != nil {
		t.Fatalf("DeleteCIFederation on absent mapper should be a no-op, got %v", err)
	}
}

// TestEnsureCIFederation_SameOwnerDifferentReposDoNotCrossInherit is the
// regression lock for finding 1: two CITrusts under the same owner but for
// different repositories with different roles must each land their role on their
// OWN per-trust group, so the lower-privileged repo never inherits the sibling's
// higher role.
func TestEnsureCIFederation_SameOwnerDifferentReposDoNotCrossInherit(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	specUser := githubSpec()
	specUser.TrustID = "trust-user"
	specUser.Subject = CISubjectPredicate{Repository: "acme/low", RepositoryOwner: "acme"}
	specUser.Role = "openshell-user"

	specAdmin := githubSpec()
	specAdmin.TrustID = "trust-admin"
	specAdmin.Subject = CISubjectPredicate{Repository: "acme/high", RepositoryOwner: "acme"}
	specAdmin.Role = "openshell-admin"

	for _, s := range []CIFederationSpec{specUser, specAdmin} {
		if err := client.EnsureCIFederation(t.Context(), s); err != nil {
			t.Fatalf("EnsureCIFederation(%s): %v", s.TrustID, err)
		}
	}

	// Each trust owns a distinct group.
	if specUser.GroupName() == specAdmin.GroupName() {
		t.Fatalf("both trusts share group %q; per-trust isolation lost", specUser.GroupName())
	}

	assertOnlyRole := func(spec CIFederationSpec, wantRole string) {
		t.Helper()
		groupID, ok := state.groups[spec.GroupName()]
		if !ok {
			t.Fatalf("group %q never created", spec.GroupName())
		}
		bindings := state.groupRoleMappings[groupID]
		for _, roles := range bindings {
			for _, role := range roles {
				if role.Name != wantRole {
					t.Errorf("group %q carries role %q, want only %q (cross-repo inheritance)", spec.GroupName(), role.Name, wantRole)
				}
			}
		}
		if len(bindings) == 0 {
			t.Errorf("group %q got no role binding", spec.GroupName())
		}
	}
	assertOnlyRole(specUser, "openshell-user")
	assertOnlyRole(specAdmin, "openshell-admin")

	// Each per-trust mapper admits only its own repository.
	assertRepoClaim := func(trustID, repo string) {
		t.Helper()
		var claims []map[string]string
		_ = json.Unmarshal([]byte(state.idpMappers["citrust-"+trustID].Config["claims"]), &claims)
		assertClaim(t, claims, "repository", repo)
	}
	assertRepoClaim("trust-user", "acme/low")
	assertRepoClaim("trust-admin", "acme/high")
}

// TestGroupName_PerTrustAndPerGateway locks the structural isolation invariant:
// the Keycloak group is a function of the trust id, so distinct trusts (distinct
// repos, or the same repo on a different gateway - each is a separate CITrust with
// its own id) never share a group, and the role grant cannot leak across repos or
// gateways.
func TestGroupName_PerTrustAndPerGateway(t *testing.T) {
	base := githubSpec()

	sameOwnerRepoA := base
	sameOwnerRepoA.TrustID = "trust-a"
	sameOwnerRepoB := base
	sameOwnerRepoB.TrustID = "trust-b"
	if sameOwnerRepoA.GroupName() == sameOwnerRepoB.GroupName() {
		t.Errorf("distinct trusts share a group: %q", sameOwnerRepoA.GroupName())
	}

	// Same repository/owner, but a second gateway is modeled as a second CITrust
	// with its own id; the groups must still differ so a role on gateway A's
	// client never binds onto a group gateway B's trust also uses.
	gatewayA := base
	gatewayA.TrustID = "trust-gw-a"
	gatewayA.GatewayClientID = "prod-gw-a"
	gatewayB := base
	gatewayB.TrustID = "trust-gw-b"
	gatewayB.GatewayClientID = "prod-gw-b"
	if gatewayA.GroupName() == gatewayB.GroupName() {
		t.Errorf("cross-gateway trusts share a group: %q", gatewayA.GroupName())
	}
}

// TestEnsureCIFederation_RejectsIdentityClaimlessSubject locks finding 2: a
// github/forgejo trust constrained only by ref or environment (no repository or
// owner) is rejected, and likewise a gitlab trust without project_path.
func TestEnsureCIFederation_RejectsIdentityClaimlessSubject(t *testing.T) {
	cases := []struct {
		name  string
		forge string
		subj  CISubjectPredicate
	}{
		{"github ref only", "github", CISubjectPredicate{Ref: "refs/heads/main"}},
		{"github environment only", "github", CISubjectPredicate{Environment: "production"}},
		{"forgejo ref only", "forgejo", CISubjectPredicate{Ref: "refs/heads/main"}},
		{"gitlab ref only", "gitlab", CISubjectPredicate{Ref: "refs/heads/main"}},
		{"gitlab environment only", "gitlab", CISubjectPredicate{Environment: "production"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, state := newFakeCIKeycloak(t)
			client := ciTestClient(server)
			spec := githubSpec()
			spec.Forge = tc.forge
			spec.Subject = tc.subj
			if err := client.EnsureCIFederation(t.Context(), spec); err == nil {
				t.Fatal("expected a trust without a repository/owner (or project_path) identity claim to be rejected")
			}
			if len(state.idpMappers) != 0 {
				t.Errorf("no mapper should be created for a rejected trust; got %v", state.idpMappers)
			}
		})
	}
}

// TestEnsureCIFederation_GitLabRefProtectedIncluded locks finding 3: a GitLab
// trust with RefProtected set carries the ref_protected claim.
func TestEnsureCIFederation_GitLabRefProtectedIncluded(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	spec := githubSpec()
	spec.Forge = "gitlab"
	spec.Subject = CISubjectPredicate{ProjectPath: "acme/app", RefProtected: true}
	if err := client.EnsureCIFederation(t.Context(), spec); err != nil {
		t.Fatalf("EnsureCIFederation: %v", err)
	}
	var claims []map[string]string
	_ = json.Unmarshal([]byte(state.idpMappers["citrust-trust-123"].Config["claims"]), &claims)
	assertClaim(t, claims, "ref_protected", "true")
}

// TestEnsureCIFederation_MissingRoleIsRetryable locks finding 5: when the gateway
// client exists but its role 404s (provisioning race), the error is a retryable
// ClientNotFoundError, not a terminal failure.
func TestEnsureCIFederation_MissingRoleIsRetryable(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	state.roleExists = false
	client := ciTestClient(server)

	err := client.EnsureCIFederation(t.Context(), githubSpec())
	if err == nil {
		t.Fatal("expected an error when the gateway client role is not provisioned")
	}
	var notFound *ClientNotFoundError
	if !errors.As(err, &notFound) {
		t.Errorf("error = %v, want a ClientNotFoundError so the caller retries", err)
	}
}

// TestDeleteCIFederation_EmptyIdPAliasIsNoOp locks finding 4: a delete with no
// resolvable idp_alias (the CIProvider is already gone) is a tolerant no-op, not
// a hard error that would wedge the delete given WatchCITrusts does not resync.
func TestDeleteCIFederation_EmptyIdPAliasIsNoOp(t *testing.T) {
	server, _ := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	spec := githubSpec()
	spec.IdpAlias = ""
	if err := client.DeleteCIFederation(t.Context(), spec); err != nil {
		t.Fatalf("delete with empty idp_alias should be a no-op, got %v", err)
	}
}

// TestEnsureCIFederation_FGAPv1AttachesBothGrants locks PLAT-CITR-11.01: in
// fgap-v1 mode the reconciler enables fine-grained authorization on BOTH the IdP
// and the target gateway client, and attaches the federation requester's client
// policy to each one's token-exchange scope permission. The v2 attribute is NOT set.
func TestEnsureCIFederation_FGAPv1AttachesBothGrants(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	if err := client.EnsureCIFederation(t.Context(), githubFGAPSpec()); err != nil {
		t.Fatalf("EnsureCIFederation (fgap-v1): %v", err)
	}

	// Fine-grained authorization enabled on both the IdP and the gateway client.
	if !containsString(state.fgapEnabled, testIdpAlias) {
		t.Errorf("fine-grained authorization not enabled on the IdP; fgapEnabled=%v", state.fgapEnabled)
	}
	if !containsString(state.fgapEnabled, testGatewayUUID) {
		t.Errorf("fine-grained authorization not enabled on the gateway client; fgapEnabled=%v", state.fgapEnabled)
	}

	// The requester client policy is attached to BOTH token-exchange scope permissions.
	if !state.permPolicies[testIdpTEPermID][testFedPolicyID] {
		t.Errorf("IdP token-exchange permission %q does not carry the requester policy; have %v", testIdpTEPermID, state.permPolicies[testIdpTEPermID])
	}
	if !state.permPolicies[testGatewayTEPermID][testFedPolicyID] {
		t.Errorf("gateway-client token-exchange permission %q does not carry the requester policy; have %v", testGatewayTEPermID, state.permPolicies[testGatewayTEPermID])
	}

	// fgap-v1 mode does NOT set the v2 attribute (mode separation).
	if state.gatewayAttrs[standardTokenExchangeEnabledAttribute] == "true" {
		t.Error("fgap-v1 mode must not set the v2 standard.token.exchange.enabled attribute")
	}
}

// TestEnsureCIFederation_FGAPv1Idempotent locks PLAT-CITR-11.02: a repeated
// reconcile reuses the existing policy and permission associations and creates no
// duplicate policy or permission attach.
func TestEnsureCIFederation_FGAPv1Idempotent(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	for pass := 1; pass <= 2; pass++ {
		if err := client.EnsureCIFederation(t.Context(), githubFGAPSpec()); err != nil {
			t.Fatalf("EnsureCIFederation (fgap-v1) pass %d: %v", pass, err)
		}
	}

	if state.policyCreates != 1 {
		t.Errorf("federation policy creates = %d, want 1 (reused on the second pass)", state.policyCreates)
	}
	// Each permission is attached once; the second pass finds the policy already
	// associated and skips the PUT.
	if got := state.permPutCount[testIdpTEPermID]; got != 1 {
		t.Errorf("IdP permission PUTs = %d, want 1 (no duplicate attach)", got)
	}
	if got := state.permPutCount[testGatewayTEPermID]; got != 1 {
		t.Errorf("gateway permission PUTs = %d, want 1 (no duplicate attach)", got)
	}
}

// TestEnsureCIFederation_DefaultModeIsFGAPv1 locks the config default: a spec with
// no explicit mode is realized as fgap-v1 (the live fleet mode), so an unconfigured
// reconcile still attaches the grants the fleet requires.
func TestEnsureCIFederation_DefaultModeIsFGAPv1(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	spec := githubSpec()
	spec.Mode = "" // unset -> default
	if err := client.EnsureCIFederation(t.Context(), spec); err != nil {
		t.Fatalf("EnsureCIFederation (default mode): %v", err)
	}
	if !state.permPolicies[testIdpTEPermID][testFedPolicyID] || !state.permPolicies[testGatewayTEPermID][testFedPolicyID] {
		t.Errorf("default mode did not attach the fgap-v1 grants; idp=%v gw=%v", state.permPolicies[testIdpTEPermID], state.permPolicies[testGatewayTEPermID])
	}
	if state.gatewayAttrs[standardTokenExchangeEnabledAttribute] == "true" {
		t.Error("default mode must be fgap-v1, not v2-attribute")
	}
}

// TestEnsureCIFederation_V2ModeSkipsV1PermissionAPI locks PLAT-CITR-08.02: in
// v2-attribute mode the reconciler authorizes the exchange via the client attribute
// and never calls the v1 management/permissions API, so a Keycloak whose v1 API
// returns 501 does not fail the reconcile.
func TestEnsureCIFederation_V2ModeSkipsV1PermissionAPI(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	if err := client.EnsureCIFederation(t.Context(), githubSpec()); err != nil {
		t.Fatalf("EnsureCIFederation (v2-attribute): %v", err)
	}
	// The v2 attribute is set and no fine-grained authorization was enabled (the v1
	// management/permissions API was never called).
	if state.gatewayAttrs[standardTokenExchangeEnabledAttribute] != "true" {
		t.Errorf("v2-attribute mode did not set the exchange attribute; attrs=%v", state.gatewayAttrs)
	}
	if len(state.fgapEnabled) != 0 {
		t.Errorf("v2-attribute mode must not call the v1 management/permissions API; fgapEnabled=%v", state.fgapEnabled)
	}
	if state.policyCreates != 0 {
		t.Errorf("v2-attribute mode must not create fgap-v1 client policies; got %d", state.policyCreates)
	}
}

// TestEnsureCIFederation_V2ModeToleratesV1Returning501 is the direct
// PLAT-CITR-08.02 regression: with the v1 permission API answering 501 (as a fleet
// Keycloak with fine-grained admin authorization v1 disabled would), v2-attribute
// reconciliation still succeeds because it never calls that API.
func TestEnsureCIFederation_V2ModeToleratesV1Returning501(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	state.v1PermAPIReturns501 = true
	client := ciTestClient(server)

	if err := client.EnsureCIFederation(t.Context(), githubSpec()); err != nil {
		t.Fatalf("EnsureCIFederation (v2-attribute, v1 API 501) must not fail: %v", err)
	}
	if state.gatewayAttrs[standardTokenExchangeEnabledAttribute] != "true" {
		t.Errorf("v2-attribute mode did not set the exchange attribute; attrs=%v", state.gatewayAttrs)
	}
	if len(state.fgapEnabled) != 0 {
		t.Errorf("v2-attribute mode reached the v1 permission API: %v", state.fgapEnabled)
	}
}

// TestEnsureCIFederation_FGAPv1RequiresFederationClient locks that a missing shared
// federation requester client fails closed (nothing to scope the grant to) rather
// than silently authorizing no one.
func TestEnsureCIFederation_FGAPv1RequiresFederationClient(t *testing.T) {
	server, state := newFakeCIKeycloak(t)
	state.federationClientExists = false
	client := ciTestClient(server)

	err := client.EnsureCIFederation(t.Context(), githubFGAPSpec())
	if err == nil {
		t.Fatal("expected an error when the federation requester client is absent")
	}
	if state.policyCreates != 0 {
		t.Errorf("no policy should be created without the requester client; got %d", state.policyCreates)
	}
}

// TestEnsureCIFederation_PreservesDisableTypeClaimCheck locks PLAT-CITR-12.02: the
// CITrust reconcile adopts the gitops-owned IdP and never issues a clobbering PUT to
// the IdP instance, so disableTypeClaimCheck survives reconciliation in every mode.
func TestEnsureCIFederation_PreservesDisableTypeClaimCheck(t *testing.T) {
	for _, spec := range []CIFederationSpec{githubFGAPSpec(), githubSpec()} {
		t.Run(string(spec.Mode), func(t *testing.T) {
			server, state := newFakeCIKeycloak(t)
			client := ciTestClient(server)

			if err := client.EnsureCIFederation(t.Context(), spec); err != nil {
				t.Fatalf("EnsureCIFederation: %v", err)
			}
			if state.idpConfig["disableTypeClaimCheck"] != "true" {
				t.Errorf("disableTypeClaimCheck = %q after reconcile, want it preserved as \"true\"", state.idpConfig["disableTypeClaimCheck"])
			}
			// Belt and suspenders: the reconciler must not PUT the IdP instance at all.
			for _, req := range state.requests {
				if req == "PUT idp" {
					t.Error("reconciler issued a clobbering PUT to the IdP instance; it must adopt the gitops-owned IdP, not overwrite it")
				}
			}
		})
	}
}

// containsString reports whether s is in xs.
func containsString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func assertClaim(t *testing.T, claims []map[string]string, key, value string) {
	t.Helper()
	for _, c := range claims {
		if c["key"] == key {
			if c["value"] != value {
				t.Errorf("claim %q = %q, want %q", key, c["value"], value)
			}
			return
		}
	}
	t.Errorf("claim %q not present in %v", key, claims)
}
