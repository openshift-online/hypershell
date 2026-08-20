package keycloak

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	testRealm           = "test-realm"
	testAdminClientID   = "admin-cli"
	testAdminSecret     = "admin-secret"
	testConsoleClientID = "my-console"
	testGatewayClientID = "my-gateway"
	testRedirectURI     = "https://console.example.com/callback"
	testWebOrigin       = "https://console.example.com"
	testFakeUUID        = "aaaabbbb-cccc-dddd-eeee-ffffffffffff"
	testGatewayUUID     = "11112222-3333-4444-5555-666677778888"
	testFakeSecret      = "test-secret"
)

// capturedMapper holds request bodies sent to the protocol-mappers endpoint.
type capturedMapper struct {
	Name   string                 `json:"name"`
	Config map[string]interface{} `json:"config"`
}

// fakeKeycloak records the mappers and scope-mapping grants the client sends.
type fakeKeycloak struct {
	mappers     []capturedMapper
	scopeMapped []keycloakRole // roles POSTed to the console client's scope-mappings
}

func newFakeKeycloak(t *testing.T) (*httptest.Server, *fakeKeycloak) {
	t.Helper()
	var mu sync.Mutex
	state := &fakeKeycloak{}

	mux := http.NewServeMux()

	// Token endpoint
	tokenPath := fmt.Sprintf("/realms/%s/protocol/openid-connect/token", testRealm)
	mux.HandleFunc(tokenPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "fake-token",
			"expires_in":   300,
		})
	})

	// Create client endpoint -- returns 201 with Location header
	clientsPath := fmt.Sprintf("/admin/realms/%s/clients", testRealm)
	mux.HandleFunc(clientsPath, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Location", fmt.Sprintf("/admin/realms/%s/clients/%s", testRealm, testFakeUUID))
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			// getClientUUID resolves the gateway client (for scope mappings);
			// every other clientId (e.g. the console client during delete
			// flows) resolves to an empty list.
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("clientId") == testGatewayClientID {
				_ = json.NewEncoder(w).Encode([]keycloakClient{
					{ID: testGatewayUUID, ClientID: testGatewayClientID},
				})
				return
			}
			_ = json.NewEncoder(w).Encode([]keycloakClient{})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Gateway client roles endpoint -- source roles for the scope-mapping grant.
	rolesPath := fmt.Sprintf("/admin/realms/%s/clients/%s/roles", testRealm, testGatewayUUID)
	mux.HandleFunc(rolesPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]keycloakRole{
			{ID: "role-admin-uuid", Name: "openshell-admin"},
			{ID: "role-user-uuid", Name: "openshell-user"},
		})
	})

	// Console client scope-mappings endpoint -- grants the gateway roles into
	// the console client's scope so fullScopeAllowed=false does not strip them.
	scopePath := fmt.Sprintf("/admin/realms/%s/clients/%s/scope-mappings/clients/%s", testRealm, testFakeUUID, testGatewayUUID)
	mux.HandleFunc(scopePath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var roles []keycloakRole
		if err := json.Unmarshal(body, &roles); err == nil {
			mu.Lock()
			state.scopeMapped = append(state.scopeMapped, roles...)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Protocol mappers endpoint
	mappersPath := fmt.Sprintf("/admin/realms/%s/clients/%s/protocol-mappers/models", testRealm, testFakeUUID)
	mux.HandleFunc(mappersPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var m capturedMapper
		if err := json.Unmarshal(body, &m); err == nil {
			mu.Lock()
			state.mappers = append(state.mappers, m)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusCreated)
	})

	// Client secret endpoint
	secretPath := fmt.Sprintf("/admin/realms/%s/clients/%s/client-secret", testRealm, testFakeUUID)
	mux.HandleFunc(secretPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"value": testFakeSecret})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, state
}

func TestProvisionConsoleClient_HappyPath(t *testing.T) {
	srv, captured := newFakeKeycloak(t)

	kc := NewClient(srv.URL, testRealm, testAdminClientID, testAdminSecret)

	uuid, secret, err := kc.ProvisionConsoleClient(
		t.Context(),
		testConsoleClientID,
		testGatewayClientID,
		testRedirectURI,
		testWebOrigin,
	)
	if err != nil {
		t.Fatalf("ProvisionConsoleClient: unexpected error: %v", err)
	}
	if uuid != testFakeUUID {
		t.Errorf("uuid: got %q, want %q", uuid, testFakeUUID)
	}
	if secret != testFakeSecret {
		t.Errorf("secret: got %q, want %q", secret, testFakeSecret)
	}

	// The console client must be granted scope for the gateway client's roles,
	// or Keycloak (fullScopeAllowed=false) strips hypershell.roles from the token
	// and the gateway denies every request.
	gotScope := make(map[string]bool)
	for _, role := range captured.scopeMapped {
		gotScope[role.Name] = true
	}
	for _, want := range []string{"openshell-admin", "openshell-user"} {
		if !gotScope[want] {
			t.Errorf("expected scope mapping to grant gateway role %q, got %v", want, captured.scopeMapped)
		}
	}

	// Verify that both audience and client-roles mappers use the GATEWAY client id.
	mappers := captured.mappers
	if len(mappers) != 3 {
		t.Fatalf("expected 3 protocol mappers, got %d", len(mappers))
	}

	for _, m := range mappers {
		switch m.Name {
		case "audience":
			aud, _ := m.Config["included.client.audience"].(string)
			if aud != testGatewayClientID {
				t.Errorf("audience mapper: included.client.audience = %q, want %q", aud, testGatewayClientID)
			}
			if strings.Contains(aud, testConsoleClientID) {
				t.Errorf("audience mapper must NOT reference console client, got %q", aud)
			}
		case "client-roles":
			clientRef, _ := m.Config["usermodel.clientRoleMapping.clientId"].(string)
			if clientRef != testGatewayClientID {
				t.Errorf("client-roles mapper: usermodel.clientRoleMapping.clientId = %q, want %q", clientRef, testGatewayClientID)
			}
			if strings.Contains(clientRef, testConsoleClientID) {
				t.Errorf("client-roles mapper must NOT reference console client, got %q", clientRef)
			}
		case "sub":
			// nothing extra to assert
		default:
			t.Errorf("unexpected mapper name: %q", m.Name)
		}
	}
}

// EnsureConsoleClientConfig must reconcile an existing console client's drifted
// configuration on every pass: reset fullScopeAllowed to false, rewrite the
// redirect URIs / web origins to the desired console host, upsert the protocol
// mappers (create the ones missing, update the ones present), and re-grant the
// gateway-client scope mappings.
func TestEnsureConsoleClientConfig_ReconcilesDrift(t *testing.T) {
	const consoleUUID = testFakeUUID
	var mu sync.Mutex
	var putRep map[string]interface{}
	createdMappers := map[string]bool{}
	updatedMappers := map[string]bool{}
	var scopeGranted []keycloakRole

	mux := http.NewServeMux()
	mux.HandleFunc(fmt.Sprintf("/realms/%s/protocol/openid-connect/token", testRealm), func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "fake-token", "expires_in": 300})
	})

	// Client-list lookups: resolve the console and gateway clients by clientId.
	mux.HandleFunc(fmt.Sprintf("/admin/realms/%s/clients", testRealm), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("clientId") {
		case testConsoleClientID:
			_ = json.NewEncoder(w).Encode([]keycloakClient{{ID: consoleUUID, ClientID: testConsoleClientID}})
		case testGatewayClientID:
			_ = json.NewEncoder(w).Encode([]keycloakClient{{ID: testGatewayUUID, ClientID: testGatewayClientID}})
		default:
			_ = json.NewEncoder(w).Encode([]keycloakClient{})
		}
	})

	// Everything under /clients/ (single-client rep, mappers, scope-mappings, roles).
	prefix := fmt.Sprintf("/admin/realms/%s/clients/", testRealm)
	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		sub := strings.TrimPrefix(r.URL.Path, prefix)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case sub == consoleUUID && r.Method == http.MethodGet:
			// The current (drifted) representation: fullScopeAllowed true and a
			// stale redirect URI that must be corrected.
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":               consoleUUID,
				"clientId":         testConsoleClientID,
				"fullScopeAllowed": true,
				"redirectUris":     []string{"https://old-host.example.com/oauth2/callback"},
				"webOrigins":       []string{"https://old-host.example.com"},
			})
		case sub == consoleUUID && r.Method == http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			_ = json.Unmarshal(body, &putRep)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case sub == consoleUUID+"/protocol-mappers/models" && r.Method == http.MethodGet:
			// Only the audience mapper exists; client-roles and sub are missing.
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "audience-id", "name": "audience"},
			})
		case sub == consoleUUID+"/protocol-mappers/models" && r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			var m capturedMapper
			_ = json.Unmarshal(body, &m)
			mu.Lock()
			createdMappers[m.Name] = true
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		case strings.HasPrefix(sub, consoleUUID+"/protocol-mappers/models/") && r.Method == http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			var m capturedMapper
			_ = json.Unmarshal(body, &m)
			mu.Lock()
			updatedMappers[m.Name] = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case sub == testGatewayUUID+"/roles" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode([]keycloakRole{
				{ID: "role-user-uuid", Name: "openshell-user"},
			})
		case sub == consoleUUID+"/scope-mappings/clients/"+testGatewayUUID && r.Method == http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			var roles []keycloakRole
			_ = json.Unmarshal(body, &roles)
			mu.Lock()
			scopeGranted = append(scopeGranted, roles...)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	kc := NewClient(srv.URL, testRealm, testAdminClientID, testAdminSecret)
	if err := kc.EnsureConsoleClientConfig(t.Context(), testConsoleClientID, testGatewayClientID, testRedirectURI, testWebOrigin); err != nil {
		t.Fatalf("EnsureConsoleClientConfig: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if putRep == nil {
		t.Fatal("expected the console client representation to be PUT")
	}
	if fsa, _ := putRep["fullScopeAllowed"].(bool); fsa {
		t.Error("fullScopeAllowed must be reset to false")
	}
	if uris, _ := putRep["redirectUris"].([]interface{}); len(uris) != 1 || uris[0] != testRedirectURI {
		t.Errorf("redirectUris = %v, want [%q]", putRep["redirectUris"], testRedirectURI)
	}
	if origins, _ := putRep["webOrigins"].([]interface{}); len(origins) != 1 || origins[0] != testWebOrigin {
		t.Errorf("webOrigins = %v, want [%q]", putRep["webOrigins"], testWebOrigin)
	}

	if !updatedMappers["audience"] {
		t.Error("expected the existing audience mapper to be updated in place")
	}
	for _, name := range []string{"client-roles", "sub"} {
		if !createdMappers[name] {
			t.Errorf("expected missing mapper %q to be created", name)
		}
	}

	if len(scopeGranted) != 1 || scopeGranted[0].Name != "openshell-user" {
		t.Errorf("expected the gateway role to be granted into the console scope, got %v", scopeGranted)
	}
}

func TestDeleteConsoleClient_NotFound(t *testing.T) {
	srv, _ := newFakeKeycloak(t)
	kc := NewClient(srv.URL, testRealm, testAdminClientID, testAdminSecret)

	// The fake server returns an empty client list, so delete should be a no-op.
	if err := kc.DeleteConsoleClient(t.Context(), testConsoleClientID); err != nil {
		t.Fatalf("DeleteConsoleClient: unexpected error: %v", err)
	}
}
