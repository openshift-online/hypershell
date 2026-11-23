package prober

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client-assertion (Keycloak federated client authentication) OAuth constants
// (OBS-PRB-06).
const (
	// grantTypeClientCredentials is the OAuth grant the client-assertion request
	// uses.
	grantTypeClientCredentials = "client_credentials"
	// clientAssertionTypeJWTBearer is the RFC 7523 client-assertion type naming the
	// projected service-account token as a JWT bearer assertion.
	clientAssertionTypeJWTBearer = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
)

// ClientAssertionAuthenticator authenticates to Keycloak with federated client
// authentication (OBS-PRB-06, the default mode): it POSTs the prober's projected
// Kubernetes service-account token to the realm token endpoint as a JWT client
// assertion (grant_type=client_credentials,
// client_assertion_type=...jwt-bearer). Keycloak validates the assertion against
// the cluster's service-account issuer with its own in-cluster service account, so
// this mode needs no static client secret, no pinned signing key, and no publicly
// reachable issuer. That is why the prober runs on the instance hub, next to that
// instance's Keycloak (OBS-PRB-01).
//
// The projected service-account token is re-read from its file on every request so
// a rotated token is always current (OBS-PRB-06.03). No static client secret is
// read from configuration; the only credential is the projected token.
type ClientAssertionAuthenticator struct {
	*cachedToken

	cfg        ClientAssertionConfig
	httpClient *http.Client

	// readFile is the projected-token reader; a seam for tests.
	readFile func(string) ([]byte, error)
}

// NewClientAssertionAuthenticator builds a ClientAssertionAuthenticator from the
// client-assertion configuration.
func NewClientAssertionAuthenticator(cfg ClientAssertionConfig) *ClientAssertionAuthenticator {
	a := &ClientAssertionAuthenticator{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		readFile:   os.ReadFile,
	}
	// Token (cached) and ExchangeFresh (bypass cache) are provided by cachedToken;
	// acquire mints a fresh token on each miss.
	a.cachedToken = &cachedToken{acquire: a.acquire}
	return a
}

// acquire re-reads the projected service-account token and POSTs it to the token
// endpoint as a client assertion (OBS-PRB-06.03). It returns the access token and
// its TTL.
func (a *ClientAssertionAuthenticator) acquire(ctx context.Context) (string, time.Duration, error) {
	if !a.cfg.Enabled() {
		return "", 0, fmt.Errorf("client-assertion authentication is not configured (token endpoint, client id, and service-account token file are required)")
	}

	// Re-read the projected service-account token on every request so a rotated
	// token is always current (OBS-PRB-06.03).
	assertionToken, err := a.readFile(a.cfg.SATokenFile)
	if err != nil {
		return "", 0, fmt.Errorf("read projected service-account token %q: %w", a.cfg.SATokenFile, err)
	}
	assertion := strings.TrimSpace(string(assertionToken))
	if assertion == "" {
		return "", 0, fmt.Errorf("projected service-account token %q is empty", a.cfg.SATokenFile)
	}

	form := url.Values{
		"grant_type":            {grantTypeClientCredentials},
		"client_id":             {a.cfg.ClientID},
		"client_assertion_type": {clientAssertionTypeJWTBearer},
		"client_assertion":      {assertion},
	}
	if a.cfg.Scope != "" {
		form.Set("scope", a.cfg.Scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("build client-assertion request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("POST client assertion to %q: %w", a.cfg.TokenEndpoint, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.WarnContext(ctx, "Failed to close token-endpoint response body", "error", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read token-endpoint response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Never log the token-endpoint response body; it can echo the assertion or
		// other sensitive material. A non-success response maps to a failed
		// authentication probe (OBS-PRB-05, OBS-PRB-06.05) without the body.
		return "", 0, fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", 0, fmt.Errorf("parse token-endpoint response: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", 0, fmt.Errorf("token-endpoint response has no access_token")
	}

	ttl := time.Duration(parsed.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return parsed.AccessToken, ttl, nil
}
