package keycloak

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	testTokenPath  = "/realms/hypershell/protocol/openid-connect/token"
	testClientPath = "/admin/realms/hypershell/clients/client-uuid"
)

func TestCreateClientEnablesDeviceAuthorizationGrant(t *testing.T) {
	t.Parallel()

	var received struct {
		ClientID   string            `json:"clientId"`
		Attributes map[string]string `json:"attributes"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/realms/hypershell/protocol/openid-connect/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"admin-token","expires_in":300}`))
		case "/admin/realms/hypershell/clients":
			if got := r.Header.Get("Authorization"); got != "Bearer admin-token" {
				t.Errorf("Authorization header = %q, want Bearer admin-token", got)
			}
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Errorf("decode client payload: %v", err)
				http.Error(w, "invalid payload", http.StatusBadRequest)
				return
			}
			w.Header().Set("Location", "/admin/realms/hypershell/clients/client-uuid")
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "hypershell", "provisioner", "secret")
	uuid, err := client.createClient(context.Background(), "gateway-id")
	if err != nil {
		t.Fatalf("createClient() error = %v", err)
	}
	if uuid != "client-uuid" {
		t.Fatalf("createClient() UUID = %q, want client-uuid", uuid)
	}
	if received.ClientID != "gateway-id" {
		t.Errorf("clientId = %q, want gateway-id", received.ClientID)
	}
	if got := received.Attributes["oauth2.device.authorization.grant.enabled"]; got != "true" {
		t.Errorf("device authorization grant attribute = %q, want true", got)
	}
	if got := received.Attributes["pkce.code.challenge.method"]; got != "S256" {
		t.Errorf("PKCE challenge method = %q, want S256", got)
	}
}

func TestEnsureDeviceAuthorizationGrantUpdatesExistingClient(t *testing.T) {
	t.Parallel()

	updated := make(chan map[string]json.RawMessage, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == testTokenPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"admin-token","expires_in":300}`))
		case r.URL.Path == testClientPath && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"client-uuid",
				"clientId":"gateway-id",
				"standardFlowEnabled":true,
				"attributes":{"pkce.code.challenge.method":"S256"}
			}`))
		case r.URL.Path == testClientPath && r.Method == http.MethodPut:
			var representation map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&representation); err != nil {
				t.Errorf("decode updated client: %v", err)
				http.Error(w, "invalid payload", http.StatusBadRequest)
				return
			}
			updated <- representation
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "hypershell", "provisioner", "secret")
	if err := client.EnsureDeviceAuthorizationGrant(context.Background(), "client-uuid"); err != nil {
		t.Fatalf("EnsureDeviceAuthorizationGrant() error = %v", err)
	}

	var representation map[string]json.RawMessage
	select {
	case representation = <-updated:
	default:
		t.Fatal("EnsureDeviceAuthorizationGrant() did not update the existing client")
	}

	var attributes map[string]string
	if err := json.Unmarshal(representation["attributes"], &attributes); err != nil {
		t.Fatalf("decode updated attributes: %v", err)
	}
	if got := attributes["oauth2.device.authorization.grant.enabled"]; got != "true" {
		t.Errorf("device authorization grant attribute = %q, want true", got)
	}
	if got := attributes["pkce.code.challenge.method"]; got != "S256" {
		t.Errorf("PKCE challenge method = %q, want S256", got)
	}

	var standardFlowEnabled bool
	if err := json.Unmarshal(representation["standardFlowEnabled"], &standardFlowEnabled); err != nil {
		t.Fatalf("decode standardFlowEnabled: %v", err)
	}
	if !standardFlowEnabled {
		t.Error("standardFlowEnabled was not preserved")
	}
}

func TestEnsureDeviceAuthorizationGrantSkipsEnabledClient(t *testing.T) {
	t.Parallel()

	updated := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == testTokenPath:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"admin-token","expires_in":300}`))
		case r.URL.Path == testClientPath && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"client-uuid",
				"clientId":"gateway-id",
				"attributes":{"oauth2.device.authorization.grant.enabled":"true"}
			}`))
		case r.URL.Path == testClientPath && r.Method == http.MethodPut:
			updated <- struct{}{}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "hypershell", "provisioner", "secret")
	if err := client.EnsureDeviceAuthorizationGrant(context.Background(), "client-uuid"); err != nil {
		t.Fatalf("EnsureDeviceAuthorizationGrant() error = %v", err)
	}

	select {
	case <-updated:
		t.Fatal("EnsureDeviceAuthorizationGrant() updated an already-enabled client")
	default:
	}
}
