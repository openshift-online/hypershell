package keycloak

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
