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
	"sync"
	"time"
)

// grantTypeTokenExchange is the RFC 8693 token-exchange grant used to exchange
// the prober's projected Kubernetes service-account token for a HyperShell API
// access token (OBS-PRB-06).
const grantTypeTokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"

// TokenExchanger performs the keyless workload-identity token exchange
// (OBS-PRB-06). It reads the prober's projected Kubernetes service-account token
// from a file on every exchange (so a rotated projected token is always
// re-read) and exchanges it at Keycloak for a short-lived HyperShell API access
// token. No static client secret is read from configuration; the only credential
// is the projected token.
//
// The request parameters (client id, subject token type, subject issuer,
// requested token type, audience, scope) are all configuration, so the prober is
// not tied to one Keycloak token-exchange mechanism (PLAT-CITR-08).
type TokenExchanger struct {
	cfg        TokenExchangeConfig
	httpClient *http.Client

	// readFile is the projected-token reader; a seam for tests.
	readFile func(string) ([]byte, error)

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// NewTokenExchanger builds a TokenExchanger from the token-exchange configuration.
func NewTokenExchanger(cfg TokenExchangeConfig) *TokenExchanger {
	return &TokenExchanger{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		readFile:   os.ReadFile,
	}
}

// Token returns a cached access token, performing a fresh exchange only when the
// cached token is missing or within 20% of expiry. It is used for the gateway
// target listing (OBS-PRB-02) and the API availability probe (OBS-PRB-04), where
// a cached token is sufficient.
func (t *TokenExchanger) Token(ctx context.Context) (string, error) {
	t.mu.Lock()
	if t.token != "" && time.Now().Before(t.expiry) {
		cached := t.token
		t.mu.Unlock()
		return cached, nil
	}
	t.mu.Unlock()

	// Perform the network exchange without holding the lock so that concurrent
	// availability probes (apiReadCycle runs up to Workers at once) do not serialize
	// behind one in-flight exchange and block up to a full per-probe timeout
	// (OBS-PRB-08). A cold-cache race may run a few redundant exchanges; that is
	// cheap and self-correcting, and the cache below collapses them.
	token, ttl, err := t.exchange(ctx)
	if err != nil {
		return "", err
	}

	t.mu.Lock()
	t.token = token
	// Refresh at 80% of TTL so a cached token is never served past its usable life.
	t.expiry = time.Now().Add(ttl * 8 / 10)
	t.mu.Unlock()
	return token, nil
}

// ExchangeFresh always performs a token exchange against Keycloak, bypassing the
// cache, and updates the cache on success. The authentication probe (OBS-PRB-05)
// uses it so that a Keycloak outage is detected as a failed authentication probe
// within one interval (OBS-PRB-09.03) instead of being masked by a still-valid
// cached token.
func (t *TokenExchanger) ExchangeFresh(ctx context.Context) (string, error) {
	token, ttl, err := t.exchange(ctx)
	if err != nil {
		return "", err
	}
	t.mu.Lock()
	t.token = token
	t.expiry = time.Now().Add(ttl * 8 / 10)
	t.mu.Unlock()
	return token, nil
}

// exchange reads the projected service-account token and performs the token
// exchange. It returns the access token and its TTL.
func (t *TokenExchanger) exchange(ctx context.Context) (string, time.Duration, error) {
	if !t.cfg.Enabled() {
		return "", 0, fmt.Errorf("token exchange is not configured (token endpoint, client id, and service-account token file are required)")
	}

	subjectToken, err := t.readFile(t.cfg.SATokenFile)
	if err != nil {
		return "", 0, fmt.Errorf("read projected service-account token %q: %w", t.cfg.SATokenFile, err)
	}
	subject := strings.TrimSpace(string(subjectToken))
	if subject == "" {
		return "", 0, fmt.Errorf("projected service-account token %q is empty", t.cfg.SATokenFile)
	}

	form := url.Values{
		"grant_type":    {grantTypeTokenExchange},
		"client_id":     {t.cfg.ClientID},
		"subject_token": {subject},
	}
	if t.cfg.SubjectTokenType != "" {
		form.Set("subject_token_type", t.cfg.SubjectTokenType)
	}
	if t.cfg.RequestedTokenType != "" {
		form.Set("requested_token_type", t.cfg.RequestedTokenType)
	}
	if t.cfg.SubjectIssuer != "" {
		form.Set("subject_issuer", t.cfg.SubjectIssuer)
	}
	if t.cfg.Audience != "" {
		form.Set("audience", t.cfg.Audience)
	}
	if t.cfg.Scope != "" {
		form.Set("scope", t.cfg.Scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("build token-exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("POST token exchange to %q: %w", t.cfg.TokenEndpoint, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.WarnContext(ctx, "Failed to close token-exchange response body", "error", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read token-exchange response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// The response body can echo the subject token on some errors; never log it.
		return "", 0, fmt.Errorf("token-exchange endpoint returned %d", resp.StatusCode)
	}

	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", 0, fmt.Errorf("parse token-exchange response: %w", err)
	}
	if parsed.AccessToken == "" {
		return "", 0, fmt.Errorf("token-exchange response has no access_token")
	}

	ttl := time.Duration(parsed.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return parsed.AccessToken, ttl, nil
}
