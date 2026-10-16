package api

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/cache"
)

// oneShotSource returns a cache.Source whose single refresh yields data, so its
// snapshot is populated (GeneratedAt set) without a running refresh loop.
func oneShotSource(t *testing.T, data any) *cache.Source {
	t.Helper()
	src := cache.NewSource("test", time.Hour, func(context.Context) (any, error) {
		return data, nil
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Run refreshes once immediately, then returns on the cancelled ctx.
	src.Run(ctx)
	return src
}

// TestServeEncodeErrorIsVisible is the regression guard for the blank-panel bug:
// a payload that encoding/json cannot marshal (a NaN float) must produce a 500
// with a JSON error body, never a silent empty 200 that blanks the UI panel.
func TestServeEncodeErrorIsVisible(t *testing.T) {
	src := oneShotSource(t, map[string]float64{"p95Ms": math.NaN()})
	h := &Handlers{Fleet: src}

	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/fleet", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Fatal("body is empty; the failure must be visible, not a silent empty response")
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// TestServeSDLCColdIs503 confirms the SDLC route returns 503 before its first
// successful refresh (never-refreshed), distinguishing cold-start from stale
// (SDLC-VIS-01).
func TestServeSDLCColdIs503(t *testing.T) {
	cold := cache.NewSource("sdlc", time.Hour, func(context.Context) (any, error) {
		return nil, context.DeadlineExceeded
	}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cold.Run(ctx) // one failed refresh: GeneratedAt stays zero
	h := &Handlers{SDLC: cold}

	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/sdlc", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("cold SDLC status = %d, want 503", rr.Code)
	}
}

// TestSDLCRouteNotMountedWhenNil confirms the optional SDLC plane leaves no route
// behind when it is disabled (no source repo), so a request 404s rather than
// panicking on a nil source.
func TestSDLCRouteNotMountedWhenNil(t *testing.T) {
	h := &Handlers{} // SDLC nil
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/sdlc", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("disabled SDLC route status = %d, want 404", rr.Code)
	}
}

// TestServeHealthy confirms a marshalable payload is returned 200 with its body.
func TestServeHealthy(t *testing.T) {
	src := oneShotSource(t, map[string]any{"instances": []string{"alpha", "beta"}})
	h := &Handlers{Instances: src}

	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/instances", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var snap cache.Snapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if snap.Stale {
		t.Errorf("snapshot should not be stale after a successful refresh")
	}
	if snap.Data == nil {
		t.Errorf("snapshot data missing")
	}
}
