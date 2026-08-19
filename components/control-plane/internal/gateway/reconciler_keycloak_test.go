package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReconcileKeycloakClientUpdatesExistingClient(t *testing.T) {
	t.Parallel()

	const (
		clientID   = "gateway-id"
		clientUUID = "client-uuid"
	)

	updated := make(chan map[string]json.RawMessage, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/realms/hypershell/protocol/openid-connect/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"admin-token","expires_in":300}`))
		case r.URL.Path == "/admin/realms/hypershell/clients" && r.Method == http.MethodGet:
			if got := r.URL.Query().Get("clientId"); got != clientID {
				t.Errorf("clientId query = %q, want %q", got, clientID)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"client-uuid","clientId":"gateway-id"}]`))
		case r.URL.Path == "/admin/realms/hypershell/clients/"+clientUUID && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"client-uuid",
				"clientId":"gateway-id",
				"attributes":{"pkce.code.challenge.method":"S256"}
			}`))
		case r.URL.Path == "/admin/realms/hypershell/clients/"+clientUUID && r.Method == http.MethodPut:
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

	opts := ReconcileOpts{
		Keycloak: &KeycloakConfig{
			ServerURL:    server.URL,
			Realm:        "hypershell",
			ClientID:     "provisioner",
			ClientSecret: "secret",
		},
		GatewayName: "gateway",
		GatewayID:   "id",
	}
	var nsConfig NamespaceConfig
	if err := reconcileKeycloakClient(context.Background(), opts, &nsConfig); err != nil {
		t.Fatalf("reconcileKeycloakClient() error = %v", err)
	}

	var representation map[string]json.RawMessage
	select {
	case representation = <-updated:
	default:
		t.Fatal("reconcileKeycloakClient() did not update the existing client")
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
	if got := nsConfig.Gateway.OIDC.ClientID; got != clientID {
		t.Errorf("OIDC client ID = %q, want %q", got, clientID)
	}
}
