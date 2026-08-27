package auth

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// --- generatePKCE ---

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge, err := generatePKCE()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verifier == "" {
		t.Error("verifier is empty")
	}

	// Verifier must be valid base64url-no-padding encoding of 32 bytes.
	vBytes, err := base64.RawURLEncoding.DecodeString(verifier)
	if err != nil {
		t.Fatalf("verifier is not valid base64url: %v", err)
	}
	if len(vBytes) != 32 {
		t.Errorf("verifier encodes %d bytes, want 32", len(vBytes))
	}

	// Challenge must be S256(verifier).
	sum := sha256.Sum256([]byte(verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if challenge != wantChallenge {
		t.Errorf("challenge = %q, want S256(verifier) = %q", challenge, wantChallenge)
	}
}

func TestGeneratePKCE_Uniqueness(t *testing.T) {
	v1, c1, err := generatePKCE()
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	v2, c2, err := generatePKCE()
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if v1 == v2 {
		t.Error("successive calls returned identical verifiers")
	}
	if c1 == c2 {
		t.Error("successive calls returned identical challenges")
	}
}

// --- tokenEndpoint ---

func TestTokenEndpoint(t *testing.T) {
	tests := []struct {
		issuerURL string
		want      string
	}{
		{"https://auth.example.com/realms/myrealm", "https://auth.example.com/realms/myrealm/protocol/openid-connect/token"},
		{"https://auth.example.com/realms/myrealm/", "https://auth.example.com/realms/myrealm/protocol/openid-connect/token"},
		{"https://auth.example.com/realms/myrealm///", "https://auth.example.com/realms/myrealm/protocol/openid-connect/token"},
	}
	for _, tc := range tests {
		got := tokenEndpoint(tc.issuerURL)
		if got != tc.want {
			t.Errorf("tokenEndpoint(%q) = %q, want %q", tc.issuerURL, got, tc.want)
		}
	}
}

// --- buildAuthURL ---

func TestBuildAuthURL(t *testing.T) {
	issuer := "https://auth.example.com/realms/myrealm"
	clientID := "hsctl"
	redirectURI := "http://127.0.0.1:9999/callback"
	state := "somestate"
	challenge := "somechallenge"

	raw := buildAuthURL(issuer, clientID, redirectURI, state, challenge)

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("buildAuthURL returned unparseable URL: %v", err)
	}

	// Base path must be correct.
	wantPath := "/realms/myrealm/protocol/openid-connect/auth"
	if u.Path != wantPath {
		t.Errorf("path = %q, want %q", u.Path, wantPath)
	}

	q := u.Query()
	params := map[string]string{
		"response_type":         "code",
		"client_id":             clientID,
		"redirect_uri":          redirectURI,
		"state":                 state,
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
	}
	for k, v := range params {
		if got := q.Get(k); got != v {
			t.Errorf("param %q = %q, want %q", k, got, v)
		}
	}

	// Scope must include openid.
	scope := q.Get("scope")
	if !strings.Contains(scope, "openid") {
		t.Errorf("scope %q does not include 'openid'", scope)
	}
}

func TestBuildAuthURL_TrailingSlash(t *testing.T) {
	issuer := "https://auth.example.com/realms/myrealm/"
	raw := buildAuthURL(issuer, "c", "r", "s", "ch")
	if strings.Contains(raw, "//protocol") {
		t.Errorf("trailing slash not trimmed: %s", raw)
	}
}

// --- parseTokenResponse ---

func makeResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func TestParseTokenResponse(t *testing.T) {
	successBody, _ := json.Marshal(TokenResponse{
		AccessToken:      "access",
		RefreshToken:     "refresh",
		ExpiresIn:        300,
		RefreshExpiresIn: 1800,
		TokenType:        "Bearer",
	})

	tests := []struct {
		name      string
		status    int
		body      string
		wantErr   bool
		wantToken string
	}{
		{
			name:      "success",
			status:    http.StatusOK,
			body:      string(successBody),
			wantToken: "access",
		},
		{
			name:    "non-200 status",
			status:  http.StatusUnauthorized,
			body:    `{"error":"invalid_client"}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			status:  http.StatusOK,
			body:    `not json`,
			wantErr: true,
		},
		{
			name:    "empty body non-200",
			status:  http.StatusBadGateway,
			body:    "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := makeResp(tc.status, tc.body)
			tr, err := parseTokenResponse(resp)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tr.AccessToken != tc.wantToken {
				t.Errorf("AccessToken = %q, want %q", tr.AccessToken, tc.wantToken)
			}
		})
	}
}

// --- pollDeviceToken ---

func TestPollDeviceToken_AuthorizationPending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"authorization_pending","error_description":"still waiting"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	client := srv.Client()
	_, err := pollDeviceToken(client, srv.URL, "hsctl", "device-code")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var tokenErr *deviceTokenError
	if !errors.As(err, &tokenErr) {
		t.Fatalf("expected *deviceTokenError, got %T: %v", err, err)
	}
	if tokenErr.Code != "authorization_pending" {
		t.Errorf("Code = %q, want %q", tokenErr.Code, "authorization_pending")
	}
}

func TestPollDeviceToken_SlowDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"slow_down"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	client := srv.Client()
	_, err := pollDeviceToken(client, srv.URL, "hsctl", "device-code")

	var tokenErr *deviceTokenError
	if !errors.As(err, &tokenErr) {
		t.Fatalf("expected *deviceTokenError, got %T: %v", err, err)
	}
	if tokenErr.Code != "slow_down" {
		t.Errorf("Code = %q, want %q", tokenErr.Code, "slow_down")
	}
}

func TestPollDeviceToken_Success(t *testing.T) {
	body, _ := json.Marshal(TokenResponse{
		AccessToken:  "tok",
		RefreshToken: "rtok",
		ExpiresIn:    300,
		TokenType:    "Bearer",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(body) //nolint:errcheck
	}))
	defer srv.Close()

	client := srv.Client()
	tr, err := pollDeviceToken(client, srv.URL, "hsctl", "device-code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.AccessToken != "tok" {
		t.Errorf("AccessToken = %q, want %q", tr.AccessToken, "tok")
	}
}

func TestPollDeviceToken_NonJSONError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error")) //nolint:errcheck
	}))
	defer srv.Close()

	client := srv.Client()
	_, err := pollDeviceToken(client, srv.URL, "hsctl", "device-code")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var tokenErr *deviceTokenError
	if errors.As(err, &tokenErr) {
		t.Errorf("unexpected *deviceTokenError for non-JSON response: %v", tokenErr)
	}
}

func TestPollDeviceToken_RequestPayload(t *testing.T) {
	var captured url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err == nil {
			captured = r.PostForm
		}
		body, _ := json.Marshal(TokenResponse{AccessToken: "a", TokenType: "Bearer"})
		w.WriteHeader(http.StatusOK)
		w.Write(body) //nolint:errcheck
	}))
	defer srv.Close()

	client := srv.Client()
	_, err := pollDeviceToken(client, srv.URL, "my-client", "my-device-code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if captured.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
		t.Errorf("grant_type = %q", captured.Get("grant_type"))
	}
	if captured.Get("client_id") != "my-client" {
		t.Errorf("client_id = %q", captured.Get("client_id"))
	}
	if captured.Get("device_code") != "my-device-code" {
		t.Errorf("device_code = %q", captured.Get("device_code"))
	}
}
