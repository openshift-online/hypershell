package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		t.Fatalf("generatePKCE: %v", err)
	}
	if verifier == "" || challenge == "" {
		t.Fatal("expected non-empty verifier and challenge")
	}

	// RFC 7636: challenge = BASE64URL(SHA256(ASCII(verifier)))
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != want {
		t.Errorf("challenge mismatch: got %q want %q", challenge, want)
	}

	// Each call produces a distinct verifier
	v2, _, err := generatePKCE()
	if err != nil {
		t.Fatalf("generatePKCE second call: %v", err)
	}
	if verifier == v2 {
		t.Error("expected different verifiers on successive calls")
	}
}

func TestBuildAuthURL(t *testing.T) {
	u := buildAuthURL("https://sso.example.com", "my-client", "http://127.0.0.1:9999/callback", "state123", "challenge456")

	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatalf("invalid URL: %v", err)
	}
	q := parsed.Query()

	checks := map[string]string{
		"response_type":         "code",
		"client_id":             "my-client",
		"redirect_uri":          "http://127.0.0.1:9999/callback",
		"state":                 "state123",
		"code_challenge":        "challenge456",
		"code_challenge_method": "S256",
		"scope":                 "openid email profile",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("param %q: got %q want %q", k, got, want)
		}
	}

	if !strings.HasPrefix(u, "https://sso.example.com/") {
		t.Errorf("URL should be rooted at issuer, got %s", u)
	}
}

func TestTokenEndpoint(t *testing.T) {
	cases := []struct{ issuer, want string }{
		{"https://sso.example.com", "https://sso.example.com/protocol/openid-connect/token"},
		{"https://sso.example.com/", "https://sso.example.com/protocol/openid-connect/token"},
	}
	for _, tc := range cases {
		if got := tokenEndpoint(tc.issuer); got != tc.want {
			t.Errorf("tokenEndpoint(%q) = %q, want %q", tc.issuer, got, tc.want)
		}
	}
}

func TestParseTokenResponse(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusOK)
		rec.WriteString(`{"access_token":"acc","refresh_token":"ref","expires_in":300}`)
		tr, err := parseTokenResponse(rec.Result())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tr.AccessToken != "acc" || tr.RefreshToken != "ref" || tr.ExpiresIn != 300 {
			t.Errorf("unexpected token response: %+v", tr)
		}
	})

	t.Run("non-200", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusUnauthorized)
		rec.WriteString(`{"error":"invalid_client"}`)
		_, err := parseTokenResponse(rec.Result())
		if err == nil {
			t.Fatal("expected error for non-200 status")
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusOK)
		rec.WriteString(`not-json`)
		_, err := parseTokenResponse(rec.Result())
		if err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})
}
