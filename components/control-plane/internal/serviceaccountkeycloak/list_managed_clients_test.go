package serviceaccountkeycloak

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// attributeSearchKeycloak serves the clients list endpoint with attributes in
// the response, the way Keycloak 26 answers a full-representation search. It
// records each list query and counts per-client GETs.
type attributeSearchKeycloak struct {
	mu      sync.Mutex
	clients []kcClient
	queries []string
	gets    int
}

func (f *attributeSearchKeycloak) serve(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/realms/realm/protocol/openid-connect/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "admin-token", "expires_in": 300})
		case r.URL.Path == "/admin/realms/realm/clients" && r.Method == http.MethodGet:
			f.queries = append(f.queries, r.URL.RawQuery)
			first, _ := strconv.Atoi(r.URL.Query().Get("first"))
			max, _ := strconv.Atoi(r.URL.Query().Get("max"))
			end := min(first+max, len(f.clients))
			page := []kcClient{}
			if first < end {
				page = f.clients[first:end]
			}
			_ = json.NewEncoder(w).Encode(page)
		case strings.HasPrefix(r.URL.Path, "/admin/realms/realm/clients/") && r.Method == http.MethodGet:
			f.gets++
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func managed(id, gateway string) kcClient {
	return kcClient{ID: id, ClientID: id, Attributes: map[string]string{
		managedAttribute: "true", gatewayIDAttribute: gateway, serviceAccountIDAttribute: "resource-" + id,
	}}
}

func TestListManagedClientsSearchesByAttributes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		gatewayID string
		wantQ     string
	}{
		{name: "one gateway", gatewayID: "gw-1", wantQ: "hypershell.service-account:true hypershell.gateway-id:gw-1"},
		{name: "all gateways", gatewayID: "", wantQ: "hypershell.service-account:true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &attributeSearchKeycloak{clients: []kcClient{managed("a", "gw-1")}}
			server := fake.serve(t)
			if _, err := NewClient(server.URL, "realm", "provisioner", "admin-secret").ListManagedClients(t.Context(), tc.gatewayID); err != nil {
				t.Fatalf("ListManagedClients() error = %v", err)
			}
			if len(fake.queries) != 1 {
				t.Fatalf("list requests = %v, want one", fake.queries)
			}
			if !strings.Contains(fake.queries[0], "+") && strings.Contains(tc.wantQ, " ") {
				t.Fatalf("raw query %q does not URL-encode the space separator", fake.queries[0])
			}
			params, err := parseQuery(fake.queries[0])
			if err != nil {
				t.Fatal(err)
			}
			if got := params.Get("q"); got != tc.wantQ {
				t.Fatalf("q = %q, want %q", got, tc.wantQ)
			}
			if params.Get("first") != "0" || params.Get("max") != "100" {
				t.Fatalf("paging = first %q max %q, want 0/100", params.Get("first"), params.Get("max"))
			}
			if fake.gets != 0 {
				t.Fatalf("per-client GETs = %d, want 0 when the search returns attributes", fake.gets)
			}
		})
	}
}

func TestListManagedClientsPaginatesSearchResults(t *testing.T) {
	fake := &attributeSearchKeycloak{}
	for i := range 250 {
		fake.clients = append(fake.clients, managed(fmt.Sprintf("c-%03d", i), "gw-1"))
	}
	server := fake.serve(t)
	clients, err := NewClient(server.URL, "realm", "provisioner", "admin-secret").ListManagedClients(t.Context(), "gw-1")
	if err != nil {
		t.Fatalf("ListManagedClients() error = %v", err)
	}
	if len(clients) != 250 {
		t.Fatalf("clients = %d, want 250", len(clients))
	}
	var firsts []string
	for _, raw := range fake.queries {
		params, _ := parseQuery(raw)
		firsts = append(firsts, params.Get("first"))
	}
	if strings.Join(firsts, ",") != "0,100,200" {
		t.Fatalf("pages requested at first = %v, want 0,100,200", firsts)
	}
}

func TestListManagedClientsRechecksAttributesOfSearchResults(t *testing.T) {
	// Defense in depth: even if Keycloak ignored the filter, only clients that
	// carry the managed marker and the requested gateway ID are returned.
	fake := &attributeSearchKeycloak{clients: []kcClient{
		managed("mine", "gw-1"),
		managed("other-gateway", "gw-2"),
		{ID: "unmanaged", ClientID: "unmanaged", Attributes: map[string]string{gatewayIDAttribute: "gw-1"}},
	}}
	server := fake.serve(t)
	clients, err := NewClient(server.URL, "realm", "provisioner", "admin-secret").ListManagedClients(t.Context(), "gw-1")
	if err != nil {
		t.Fatalf("ListManagedClients() error = %v", err)
	}
	if len(clients) != 1 || clients[0].UUID != "mine" || clients[0].ServiceAccountID != "resource-mine" {
		t.Fatalf("ListManagedClients() = %+v, want only the gw-1 managed client", clients)
	}
}

func TestListManagedClientsRejectsGatewayIDWithWhitespace(t *testing.T) {
	fake := &attributeSearchKeycloak{}
	server := fake.serve(t)
	if _, err := NewClient(server.URL, "realm", "provisioner", "admin-secret").ListManagedClients(t.Context(), "gw-1 hypershell.service-account:true"); err == nil {
		t.Fatal("ListManagedClients() error = nil, want rejection of a gateway ID with a space")
	}
	if len(fake.queries) != 0 {
		t.Fatalf("list requests = %v, want none", fake.queries)
	}
}

func parseQuery(raw string) (interface{ Get(string) string }, error) {
	return url.ParseQuery(raw)
}

func TestListManagedClientsFallsBackToScanWhenSearchIsRefused(t *testing.T) {
	// Keycloak 26 answers 400 until searchable-attributes lists our keys; the
	// cleanup must still find the gateway's clients by scanning.
	gets := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/realms/realm/protocol/openid-connect/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "admin-token", "expires_in": 300})
		case r.URL.Path == "/admin/realms/realm/clients" && r.URL.Query().Has("q"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_request","error_description":"Attributes [hypershell.service-account] not allowed for search"}`))
		case r.URL.Path == "/admin/realms/realm/clients":
			_ = json.NewEncoder(w).Encode([]kcClient{{ID: "a"}, {ID: "b"}})
		case strings.HasPrefix(r.URL.Path, "/admin/realms/realm/clients/"):
			gets++
			id := strings.TrimPrefix(r.URL.Path, "/admin/realms/realm/clients/")
			gateway := map[string]string{"a": "gw-1", "b": "gw-2"}[id]
			_ = json.NewEncoder(w).Encode(managed(id, gateway))
		}
	}))
	t.Cleanup(server.Close)
	clients, err := NewClient(server.URL, "realm", "provisioner", "admin-secret").ListManagedClients(t.Context(), "gw-1")
	if err != nil {
		t.Fatalf("ListManagedClients() error = %v", err)
	}
	if len(clients) != 1 || clients[0].UUID != "a" || gets != 2 {
		t.Fatalf("clients = %+v after %d GETs, want only a after scanning both", clients, gets)
	}
}

func TestListManagedClientsPropagatesOtherSearchErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/realms/realm/protocol/openid-connect/token" {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "admin-token", "expires_in": 300})
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	if _, err := NewClient(server.URL, "realm", "provisioner", "admin-secret").ListManagedClients(t.Context(), "gw-1"); err == nil {
		t.Fatal("ListManagedClients() error = nil, want the 403 to propagate")
	}
}
