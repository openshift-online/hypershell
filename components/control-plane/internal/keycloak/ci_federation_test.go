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
	testIdpAlias       = "github-actions"
	testFederationID   = "ci-federation"
	testWorkspace      = "gh-acme"
	testWorkspaceGrpID = "group-gh-acme-uuid"
	testGatewayRoleID  = "role-openshell-user-uuid"
)

// fakeCIKeycloak is a stateful httptest fake of the Keycloak admin REST surface
// the CI federation code drives. It records every write so tests can assert the
// exact artifacts created, and it serves reads from its own state so a second
// Ensure pass observes what the first created (idempotency).
type fakeCIKeycloak struct {
	mu sync.Mutex

	idpExists bool
	// idpMappers maps mapper name -> representation, keyed as Keycloak would.
	idpMappers map[string]idpMapper
	nextID     int

	groupExists bool

	// gatewayClientExists controls whether the gateway clientId resolves.
	gatewayClientExists bool
	// gatewayAttrs is the current gateway client attribute map (PUT target).
	gatewayAttrs map[string]string
	// roleExists controls whether the role resolves on the gateway client.
	roleExists bool

	// groupRoleMappings records POSTed {groupID}->roles.
	groupRoleMappings [][]keycloakRole

	requests []string // method+path trail, for ordering assertions
}

func newFakeCIKeycloak(t *testing.T) (*httptest.Server, *fakeCIKeycloak) {
	t.Helper()
	state := &fakeCIKeycloak{
		idpExists:           true,
		idpMappers:          map[string]idpMapper{},
		gatewayClientExists: true,
		gatewayAttrs:        map[string]string{},
		roleExists:          true,
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
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"alias": testIdpAlias})
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

	// Groups collection (GET search, POST create).
	groupsBase := fmt.Sprintf("/admin/realms/%s/groups", testRealm)
	mux.HandleFunc(groupsBase, func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			state.requests = append(state.requests, "GET groups")
			w.Header().Set("Content-Type", "application/json")
			if state.groupExists && r.URL.Query().Get("search") == testWorkspace {
				_ = json.NewEncoder(w).Encode([]keycloakGroup{{ID: testWorkspaceGrpID, Name: testWorkspace}})
				return
			}
			_ = json.NewEncoder(w).Encode([]keycloakGroup{})
		case http.MethodPost:
			state.requests = append(state.requests, "POST groups")
			state.groupExists = true
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Clients collection -- resolve gateway clientId -> uuid.
	mux.HandleFunc(fmt.Sprintf("/admin/realms/%s/clients", testRealm), func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("clientId") == testGatewayClientID && state.gatewayClientExists {
			_ = json.NewEncoder(w).Encode([]keycloakClient{{ID: testGatewayUUID, ClientID: testGatewayClientID}})
			return
		}
		_ = json.NewEncoder(w).Encode([]keycloakClient{})
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

	// Group role-mappings onto the gateway client.
	mux.HandleFunc(fmt.Sprintf("/admin/realms/%s/groups/%s/role-mappings/clients/%s", testRealm, testWorkspaceGrpID, testGatewayUUID), func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		state.requests = append(state.requests, "POST group role-mapping")
		body, _ := io.ReadAll(r.Body)
		var roles []keycloakRole
		_ = json.Unmarshal(body, &roles)
		state.groupRoleMappings = append(state.groupRoleMappings, roles)
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
		Subject: CISubjectPredicate{
			Repository:      "acme/app",
			RepositoryOwner: "acme",
		},
	}
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
	if got, want := mapper.Config["group"], "/"+testWorkspace; got != want {
		t.Errorf("mapper group = %q, want %q", got, want)
	}
	var claims []map[string]string
	if err := json.Unmarshal([]byte(mapper.Config["claims"]), &claims); err != nil {
		t.Fatalf("parse claims: %v", err)
	}
	assertClaim(t, claims, "repository", "acme/app")
	assertClaim(t, claims, "repository_owner", "acme")

	// (c) gateway role bound onto the workspace group.
	if len(state.groupRoleMappings) != 1 || len(state.groupRoleMappings[0]) != 1 ||
		state.groupRoleMappings[0][0].Name != "openshell-user" {
		t.Errorf("group role mappings = %v, want one openshell-user binding", state.groupRoleMappings)
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
	// The shared workspace group binding is deliberately preserved for sibling
	// trusts; the mapper removal is the authoritative revocation.
	if len(state.groupRoleMappings) == 0 {
		t.Error("workspace group binding should survive a single trust delete")
	}
}

func TestDeleteCIFederation_MissingMapperIsNoOp(t *testing.T) {
	server, _ := newFakeCIKeycloak(t)
	client := ciTestClient(server)

	if err := client.DeleteCIFederation(t.Context(), githubSpec()); err != nil {
		t.Fatalf("DeleteCIFederation on absent mapper should be a no-op, got %v", err)
	}
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
