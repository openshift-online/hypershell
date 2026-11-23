package prober

import (
	"context"
	"sync"
	"time"
)

// authenticator acquires a short-lived HyperShell API access token with the
// prober's keyless identity (OBS-PRB-06). Both authentication modes satisfy it:
// the default client-assertion mode (Keycloak federated client authentication) and
// the token-exchange mode (RFC 8693). Which one the prober uses is chosen by
// configuration (HYPERSHELL_PROBE_AUTH_MODE), not code. Neither mode reads a static
// client secret.
type authenticator interface {
	// Token returns a cached access token, acquiring a fresh one only near expiry.
	// It is used for the gateway target listing (OBS-PRB-02) and the API
	// availability probe (OBS-PRB-04), where a cached token is sufficient.
	Token(ctx context.Context) (string, error)
	// ExchangeFresh always acquires a new token from Keycloak, bypassing the cache.
	// The authentication probe (OBS-PRB-05) uses it so a Keycloak outage is detected
	// as a failed authentication probe within one interval (OBS-PRB-09.03) instead
	// of being masked by a still-valid cached token.
	ExchangeFresh(ctx context.Context) (string, error)
}

// cachedToken caches a minted access token and re-mints it only near expiry. Both
// authentication modes embed it; the mode-specific acquire func mints a fresh token
// from Keycloak and returns it with its TTL.
type cachedToken struct {
	// acquire mints a fresh token from Keycloak. It is set by the embedding mode.
	acquire func(ctx context.Context) (string, time.Duration, error)

	mu     sync.Mutex
	token  string
	expiry time.Time
}

// Token returns a cached access token, acquiring a fresh one only when the cached
// token is missing or within 20% of expiry.
func (c *cachedToken) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.expiry) {
		cached := c.token
		c.mu.Unlock()
		return cached, nil
	}
	c.mu.Unlock()

	// Acquire the token without holding the lock so concurrent availability probes
	// (apiReadCycle runs up to Workers at once) do not serialize behind one in-flight
	// acquisition and block up to a full per-probe timeout (OBS-PRB-08). A cold-cache
	// race may run a few redundant acquisitions; that is cheap and self-correcting,
	// and the cache below collapses them.
	token, ttl, err := c.acquire(ctx)
	if err != nil {
		return "", err
	}
	c.store(token, ttl)
	return token, nil
}

// ExchangeFresh always acquires a new token, bypassing the cache, and updates the
// cache on success.
func (c *cachedToken) ExchangeFresh(ctx context.Context) (string, error) {
	token, ttl, err := c.acquire(ctx)
	if err != nil {
		return "", err
	}
	c.store(token, ttl)
	return token, nil
}

// store records a freshly minted token, refreshing at 80% of its TTL so a cached
// token is never served past its usable life.
func (c *cachedToken) store(token string, ttl time.Duration) {
	c.mu.Lock()
	c.token = token
	c.expiry = time.Now().Add(ttl * 8 / 10)
	c.mu.Unlock()
}
