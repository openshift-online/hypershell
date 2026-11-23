package prober

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func writeTokenFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	return path
}

// TestExchangeUsesProjectedTokenNotStaticSecret covers OBS-PRB-06.01: the prober
// exchanges its projected service-account token for an access token and never
// reads a static client secret.
func TestExchangeUsesProjectedTokenNotStaticSecret(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"minted-token","expires_in":300}`))
	}))
	defer srv.Close()

	ex := NewTokenExchanger(TokenExchangeConfig{
		TokenEndpoint:    srv.URL,
		ClientID:         "ci-federation",
		SATokenFile:      writeTokenFile(t, "projected-sa-jwt"),
		SubjectTokenType: DefaultSubjectTokenType,
		SubjectIssuer:    "hypershell-workload",
		Audience:         "hypershell-api",
	})

	token, err := ex.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != "minted-token" {
		t.Fatalf("token = %q, want minted-token", token)
	}

	if got := gotForm.Get("grant_type"); got != grantTypeTokenExchange {
		t.Errorf("grant_type = %q, want %q", got, grantTypeTokenExchange)
	}
	if got := gotForm.Get("subject_token"); got != "projected-sa-jwt" {
		t.Errorf("subject_token = %q, want the projected SA token", got)
	}
	if got := gotForm.Get("client_id"); got != "ci-federation" {
		t.Errorf("client_id = %q, want ci-federation", got)
	}
	if got := gotForm.Get("audience"); got != "hypershell-api" {
		t.Errorf("audience = %q, want hypershell-api", got)
	}
	// No static client secret anywhere (OBS-PRB-06.01).
	if _, ok := gotForm["client_secret"]; ok {
		t.Errorf("client_secret must never be sent; form carried one")
	}
}

func TestTokenIsCachedButExchangeFreshAlwaysHitsKeycloak(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":300}`))
	}))
	defer srv.Close()

	ex := NewTokenExchanger(TokenExchangeConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "c",
		SATokenFile:   writeTokenFile(t, "sa"),
	})

	// Two cached reads perform a single exchange.
	if _, err := ex.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := ex.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("cached Token did %d exchanges, want 1", calls.Load())
	}

	// ExchangeFresh always contacts Keycloak so a Keycloak outage surfaces
	// promptly for the authentication probe (OBS-PRB-09.03).
	if _, err := ex.ExchangeFresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("ExchangeFresh did not contact Keycloak; calls = %d, want 2", calls.Load())
	}
}

func TestExchangeFailsOnKeycloakError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ex := NewTokenExchanger(TokenExchangeConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "c",
		SATokenFile:   writeTokenFile(t, "sa"),
	})
	if _, err := ex.ExchangeFresh(context.Background()); err == nil {
		t.Fatal("expected error on Keycloak 503")
	}
}

func TestExchangeFailsWhenNotConfigured(t *testing.T) {
	ex := NewTokenExchanger(TokenExchangeConfig{})
	if _, err := ex.Token(context.Background()); err == nil {
		t.Fatal("expected error when token exchange is not configured")
	}
}

func TestExchangeFailsOnEmptyTokenFile(t *testing.T) {
	ex := NewTokenExchanger(TokenExchangeConfig{
		TokenEndpoint: "http://unused",
		ClientID:      "c",
		SATokenFile:   writeTokenFile(t, "   "),
	})
	_, err := ex.Token(context.Background())
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty-token error, got %v", err)
	}
}
