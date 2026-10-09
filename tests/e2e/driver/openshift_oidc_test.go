package driver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestOpenShiftGatewayTokenUsesExchangeForClientCredentials(t *testing.T) {
	var calls []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		calls = append(calls, r.Form)
		_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: testJWT(map[string]any{
			"hypershell": map[string]any{"roles": []string{"openshell-admin"}},
		})})
	}))
	defer server.Close()

	t.Setenv("E2E_OIDC_GRANT", grantClientCredentials)
	t.Setenv("E2E_OIDC_SA_CLIENT_ID", "hypershell-e2e")
	t.Setenv("E2E_OIDC_SA_CLIENT_SECRET", "test-secret")
	d := &openshiftDriver{oidcIssuer: server.URL + "/realms/hypershell", httpClient: server.Client()}

	_, err := d.AcquireGatewayTokenWithRole(context.Background(), Credentials{Username: "admin"}, "gateway-client", "openshell-admin")
	if err != nil {
		t.Fatalf("acquire gateway token: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("token requests = %d, want client credentials then token exchange", len(calls))
	}
	if got := calls[0].Get("grant_type"); got != grantClientCredentials {
		t.Errorf("first grant_type = %q, want %q", got, grantClientCredentials)
	}
	if got := calls[1].Get("grant_type"); got != grantTokenExchange {
		t.Errorf("second grant_type = %q, want %q", got, grantTokenExchange)
	}
	if got := calls[1].Get("audience"); got != "gateway-client" {
		t.Errorf("exchange audience = %q, want gateway-client", got)
	}
}

func testJWT(claims map[string]any) string {
	payload, err := json.Marshal(claims)
	if err != nil {
		panic(err)
	}
	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(payload) + "."
}
