package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenProvider obtains and caches OAuth2 tokens using the client_credentials
// grant. It is safe for concurrent use and proactively refreshes the token
// when 80% of its TTL has elapsed.
type TokenProvider struct {
	issuer       string
	clientID     string
	clientSecret string

	mu            sync.Mutex
	token         string
	expiry        time.Time
	tokenEndpoint string
}

// NewTokenProvider creates a TokenProvider that will discover the token
// endpoint from the issuer's OpenID Connect discovery document and obtain
// tokens via the client_credentials grant.
//
// The discovery request is deferred to the first call to Token() so that an
// unreachable issuer at startup does not crash the process.
func NewTokenProvider(issuer, clientID, clientSecret string) *TokenProvider {
	return &TokenProvider{
		issuer:       issuer,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// SetTokenEndpoint overrides the discovered token endpoint. Use this when the
// OIDC discovery document advertises an external HTTPS URL that the pod cannot
// reach (e.g. a gateway-fronted Keycloak with a self-signed CA).
func (tp *TokenProvider) SetTokenEndpoint(endpoint string) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	tp.tokenEndpoint = endpoint
}

// Token returns a valid access token, refreshing it if necessary. It returns
// an empty string when the provider is not configured (no issuer).
func (tp *TokenProvider) Token() (string, error) {
	if tp.issuer == "" {
		return "", nil
	}

	tp.mu.Lock()
	defer tp.mu.Unlock()

	// Return cached token if still within 80% of its lifetime.
	if tp.token != "" && time.Now().Before(tp.expiry) {
		return tp.token, nil
	}

	if tp.tokenEndpoint == "" {
		endpoint, err := tp.discover()
		if err != nil {
			return "", fmt.Errorf("discover token endpoint: %w", err)
		}
		tp.tokenEndpoint = endpoint
	}

	token, expiresIn, err := tp.fetchToken()
	if err != nil {
		// If the discovery cache is stale, retry once with a fresh endpoint.
		endpoint, discoverErr := tp.discover()
		if discoverErr != nil {
			return "", fmt.Errorf("fetch token: %w (rediscovery also failed: %v)", err, discoverErr)
		}
		tp.tokenEndpoint = endpoint
		token, expiresIn, err = tp.fetchToken()
		if err != nil {
			return "", fmt.Errorf("fetch token after rediscovery: %w", err)
		}
	}

	tp.token = token
	// Refresh at 80% of TTL to avoid using an expired token.
	tp.expiry = time.Now().Add(time.Duration(expiresIn) * time.Second * 8 / 10)

	return tp.token, nil
}

type openIDConfiguration struct {
	TokenEndpoint string `json:"token_endpoint"`
}

func (tp *TokenProvider) discover() (string, error) {
	discoveryURL := strings.TrimRight(tp.issuer, "/") + "/.well-known/openid-configuration"

	resp, err := http.Get(discoveryURL) //nolint:gosec // URL is operator-configured
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", discoveryURL, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("WARN closing discovery response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s returned %d", discoveryURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read discovery response: %w", err)
	}

	var cfg openIDConfiguration
	if err := json.Unmarshal(body, &cfg); err != nil {
		return "", fmt.Errorf("parse discovery response: %w", err)
	}

	if cfg.TokenEndpoint == "" {
		return "", fmt.Errorf("discovery document has no token_endpoint")
	}

	return cfg.TokenEndpoint, nil
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func (tp *TokenProvider) fetchToken() (string, int, error) {
	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {tp.clientID},
		"client_secret": {tp.clientSecret},
	}

	resp, err := http.PostForm(tp.tokenEndpoint, data) //nolint:gosec // URL is discovered from operator config
	if err != nil {
		return "", 0, fmt.Errorf("POST %s: %w", tp.tokenEndpoint, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("WARN closing token response body: %v", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", 0, fmt.Errorf("parse token response: %w", err)
	}

	if tok.AccessToken == "" {
		return "", 0, fmt.Errorf("token response has no access_token")
	}

	if tok.ExpiresIn <= 0 {
		tok.ExpiresIn = 300 // default to 5 minutes
	}

	return tok.AccessToken, tok.ExpiresIn, nil
}
