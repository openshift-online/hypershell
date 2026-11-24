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

// TestClientAssertionRequestShape covers OBS-PRB-06.03: the default mode POSTs the
// projected service-account token to the realm token endpoint as a JWT client
// assertion with exactly the required form fields and no static client secret.
func TestClientAssertionRequestShape(t *testing.T) {
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

	a := NewClientAssertionAuthenticator(ClientAssertionConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "hypershell-prober",
		SATokenFile:   writeTokenFile(t, "projected-sa-jwt"),
	})

	token, err := a.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != "minted-token" {
		t.Fatalf("token = %q, want minted-token", token)
	}

	if got := gotForm.Get("grant_type"); got != grantTypeClientCredentials {
		t.Errorf("grant_type = %q, want %q", got, grantTypeClientCredentials)
	}
	// The request SHALL NOT carry a client_id form field even when ClientID is
	// configured: Keycloak 26.6 rejects a request whose client_id differs from the
	// assertion sub (OBS-PRB-06, OBS-PRB-06.06).
	if gotForm.Has("client_id") {
		t.Errorf("client_id must never be sent; form carried %q", gotForm.Get("client_id"))
	}
	if got := gotForm.Get("client_assertion_type"); got != clientAssertionTypeJWTBearer {
		t.Errorf("client_assertion_type = %q, want %q", got, clientAssertionTypeJWTBearer)
	}
	if got := gotForm.Get("client_assertion"); got != "projected-sa-jwt" {
		t.Errorf("client_assertion = %q, want the projected SA token", got)
	}
	// Exactly the three required fields; no static client secret (OBS-PRB-06.03).
	if _, ok := gotForm["client_secret"]; ok {
		t.Errorf("client_secret must never be sent; form carried one")
	}
	if _, ok := gotForm["subject_token"]; ok {
		t.Errorf("client-assertion mode must not send subject_token")
	}
}

// TestClientAssertionRereadsTokenEachRequest covers OBS-PRB-06.03: the projected
// service-account token is re-read from its file on every request, so a rotated
// token is always current.
func TestClientAssertionRereadsTokenEachRequest(t *testing.T) {
	var lastAssertion atomic.Value
	lastAssertion.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		lastAssertion.Store(r.PostForm.Get("client_assertion"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":300}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("first-token"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	a := NewClientAssertionAuthenticator(ClientAssertionConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "c",
		SATokenFile:   path,
	})

	if _, err := a.ExchangeFresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := lastAssertion.Load().(string); got != "first-token" {
		t.Fatalf("first assertion = %q, want first-token", got)
	}

	// Rotate the projected token on disk; the next request must carry the new value.
	if err := os.WriteFile(path, []byte("rotated-token"), 0o600); err != nil {
		t.Fatalf("rotate token: %v", err)
	}
	if _, err := a.ExchangeFresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := lastAssertion.Load().(string); got != "rotated-token" {
		t.Fatalf("second assertion = %q, want rotated-token (token file not re-read)", got)
	}
}

// TestClientAssertionCachesButFreshAlwaysHits covers OBS-PRB-06: cached reads reuse
// the token, while ExchangeFresh (the authentication probe) always contacts
// Keycloak.
func TestClientAssertionCachesButFreshAlwaysHits(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":300}`))
	}))
	defer srv.Close()

	a := NewClientAssertionAuthenticator(ClientAssertionConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "c",
		SATokenFile:   writeTokenFile(t, "sa"),
	})

	if _, err := a.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Token(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("cached Token did %d acquisitions, want 1", calls.Load())
	}

	if _, err := a.ExchangeFresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("ExchangeFresh did not contact Keycloak; calls = %d, want 2", calls.Load())
	}
}

// TestClientAssertionMapsNonSuccessToFailureWithoutLoggingBody covers OBS-PRB-06.05:
// a non-success token-endpoint response is an error (a failed authentication probe)
// and the error never contains the response body.
func TestClientAssertionMapsNonSuccessToFailureWithoutLoggingBody(t *testing.T) {
	const secretBody = "super-secret-assertion-echo"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(secretBody))
	}))
	defer srv.Close()

	a := NewClientAssertionAuthenticator(ClientAssertionConfig{
		TokenEndpoint: srv.URL,
		ClientID:      "c",
		SATokenFile:   writeTokenFile(t, "sa"),
	})
	_, err := a.ExchangeFresh(context.Background())
	if err == nil {
		t.Fatal("expected error on non-success token-endpoint response")
	}
	if strings.Contains(err.Error(), secretBody) {
		t.Fatalf("error leaked the token-endpoint response body: %v", err)
	}
}

// TestClientAssertionFailsWhenNotConfigured and on an empty token file cover the
// not-configured and empty-token cases (OBS-PRB-05.02, OBS-PRB-06).
func TestClientAssertionFailsWhenNotConfigured(t *testing.T) {
	a := NewClientAssertionAuthenticator(ClientAssertionConfig{})
	if _, err := a.Token(context.Background()); err == nil {
		t.Fatal("expected error when client-assertion authentication is not configured")
	}
}

func TestClientAssertionFailsOnEmptyTokenFile(t *testing.T) {
	a := NewClientAssertionAuthenticator(ClientAssertionConfig{
		TokenEndpoint: "http://unused",
		ClientID:      "c",
		SATokenFile:   writeTokenFile(t, "   "),
	})
	_, err := a.Token(context.Background())
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty-token error, got %v", err)
	}
}
