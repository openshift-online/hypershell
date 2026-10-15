package driver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestOpenShiftAPITokenImpersonatesNamedUserForClientCredentials(t *testing.T) {
	var calls []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		calls = append(calls, r.Form)
		_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: "token"})
	}))
	defer server.Close()

	t.Setenv("E2E_OIDC_GRANT", grantClientCredentials)
	t.Setenv("E2E_OIDC_USERNAME", "admin")
	t.Setenv("E2E_OIDC_SA_CLIENT_ID", "hypershell-e2e")
	t.Setenv("E2E_OIDC_SA_CLIENT_SECRET", "test-secret")
	d := &openshiftDriver{
		oidcIssuer: server.URL + "/realms/hypershell",
		frontendID: "hypershell-frontend",
		httpClient: server.Client(),
	}

	_, err := d.AcquireOIDCToken(context.Background(), Credentials{Username: "developer"})
	if err != nil {
		t.Fatalf("acquire developer API token: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("token requests = %d, want client credentials then token exchange", len(calls))
	}
	if got := calls[1].Get("audience"); got != "hypershell-frontend" {
		t.Errorf("exchange audience = %q, want hypershell-frontend", got)
	}
	if got := calls[1].Get("requested_subject"); got != "developer" {
		t.Errorf("exchange requested_subject = %q, want developer", got)
	}
}

func TestOpenShiftGatewayTokenReportsRefusedExchange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token form: %v", err)
		}
		if r.Form.Get("grant_type") == grantTokenExchange {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(tokenResponse{
				Error:            "access_denied",
				ErrorDescription: "Client not allowed to exchange",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: "subject-token"})
	}))
	defer server.Close()

	t.Setenv("E2E_OIDC_GRANT", grantClientCredentials)
	t.Setenv("E2E_OIDC_SA_CLIENT_ID", "hypershell-release-check")
	t.Setenv("E2E_OIDC_SA_CLIENT_SECRET", "super-secret-value")
	t.Setenv("E2E_GATEWAY_TOKEN_TIMEOUT", "1")
	d := &openshiftDriver{oidcIssuer: server.URL + "/realms/hypershell", httpClient: server.Client()}

	_, err := d.AcquireGatewayTokenWithRole(context.Background(), Credentials{Username: "admin"}, "gateway-client", "openshell-admin")
	if err == nil {
		t.Fatal("refused token exchange was reported as success")
	}
	for _, want := range []string{`never gained role "openshell-admin"`, "status 403", "access_denied", "Client not allowed to exchange"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	for _, secret := range []string{"super-secret-value", "subject-token"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error leaks %q: %v", secret, err)
		}
	}
}

func TestOpenShiftGatewayTokenReportsMissingRole(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: testJWT(map[string]any{
			"hypershell": map[string]any{"roles": []string{"openshell-user"}},
		})})
	}))
	defer server.Close()

	t.Setenv("E2E_OIDC_GRANT", grantClientCredentials)
	t.Setenv("E2E_OIDC_SA_CLIENT_ID", "hypershell-e2e")
	t.Setenv("E2E_OIDC_SA_CLIENT_SECRET", "test-secret")
	t.Setenv("E2E_GATEWAY_TOKEN_TIMEOUT", "1")
	d := &openshiftDriver{oidcIssuer: server.URL + "/realms/hypershell", httpClient: server.Client()}

	_, err := d.AcquireGatewayTokenWithRole(context.Background(), Credentials{Username: "admin"}, "gateway-client", "openshell-admin")
	if err == nil {
		t.Fatal("token without the requested role was reported as success")
	}
	for _, want := range []string{`never gained role "openshell-admin"`, "openshell-user"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func testJWT(claims map[string]any) string {
	payload, err := json.Marshal(claims)
	if err != nil {
		panic(err)
	}
	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(payload) + "."
}
