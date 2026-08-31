package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenReusesCachedToken(t *testing.T) {
	t.Parallel()

	var grants atomic.Int32
	server := newTokenServer(t, &grants)

	provider := NewTokenProvider("https://issuer.invalid", "client-id", "client-secret")
	provider.SetTokenEndpoint(server.URL)

	first, err := provider.Token()
	if err != nil {
		t.Fatalf("first Token() call failed: %v", err)
	}
	second, err := provider.Token()
	if err != nil {
		t.Fatalf("second Token() call failed: %v", err)
	}

	if first != second {
		t.Fatalf("Token() returned %q and %q; both calls must return the same token", first, second)
	}
	if got := grants.Load(); got != 1 {
		t.Fatalf("token endpoint received %d grants; it must receive 1", got)
	}
}

func TestConcurrentTokenCallsShareCachedToken(t *testing.T) {
	t.Parallel()

	var grants atomic.Int32
	server := newTokenServer(t, &grants)

	provider := NewTokenProvider("https://issuer.invalid", "client-id", "client-secret")
	provider.SetTokenEndpoint(server.URL)

	const callCount = 16
	type result struct {
		token string
		err   error
	}

	start := make(chan struct{})
	results := make(chan result, callCount)
	for range callCount {
		go func() {
			<-start
			token, err := provider.Token()
			results <- result{token: token, err: err}
		}()
	}
	close(start)

	for range callCount {
		result := <-results
		if result.err != nil {
			t.Fatalf("Token() failed: %v", result.err)
		}
		if result.token != "token-1" {
			t.Fatalf("Token() returned %q; it must return %q", result.token, "token-1")
		}
	}

	if got := grants.Load(); got != 1 {
		t.Fatalf("token endpoint received %d grants; it must receive 1", got)
	}
}

func TestTokenRefreshesAfterThreshold(t *testing.T) {
	t.Parallel()

	var grants atomic.Int32
	server := newTokenServer(t, &grants)

	provider := NewTokenProvider("https://issuer.invalid", "client-id", "client-secret")
	provider.SetTokenEndpoint(server.URL)

	requestStarted := time.Now()
	first, err := provider.Token()
	requestFinished := time.Now()
	if err != nil {
		t.Fatalf("first Token() call failed: %v", err)
	}

	provider.mu.Lock()
	refreshAt := provider.expiry
	provider.expiry = time.Now().Add(-time.Second)
	provider.mu.Unlock()

	const refreshDelay = 4 * time.Minute
	if refreshAt.Before(requestStarted.Add(refreshDelay)) || refreshAt.After(requestFinished.Add(refreshDelay)) {
		t.Fatalf("refresh time = %v; want 80 percent of a 300-second lifetime", refreshAt)
	}

	second, err := provider.Token()
	if err != nil {
		t.Fatalf("second Token() call failed: %v", err)
	}
	if first != "token-1" || second != "token-2" {
		t.Fatalf("Token() returned %q and %q; want %q and %q", first, second, "token-1", "token-2")
	}
	if got := grants.Load(); got != 2 {
		t.Fatalf("token endpoint received %d grants; it must receive 2", got)
	}
}

func newTokenServer(t *testing.T, grants *atomic.Int32) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		grant := grants.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: fmt.Sprintf("token-%d", grant),
			ExpiresIn:   300,
			TokenType:   "Bearer",
		})
	}))
	t.Cleanup(server.Close)
	return server
}
