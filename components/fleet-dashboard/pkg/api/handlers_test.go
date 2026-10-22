package api

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/cache"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/sources"
)

// fakeE2E is a minimal E2ERunPlane for handler tests.
type fakeE2E struct {
	detail *sources.E2ERunDetail
	err    error

	logs    *sources.E2ELogPage
	logsErr error
	// gotBefore and gotLimit record the last RunLogs arguments so a test can assert
	// the handler forwarded the before cursor and limit it parsed from the query.
	gotBefore string
	gotLimit  int
}

func (f *fakeE2E) RunDetail(context.Context, string) (*sources.E2ERunDetail, error) {
	return f.detail, f.err
}

func (f *fakeE2E) RunLogs(_ context.Context, _, before string, limit int) (*sources.E2ELogPage, error) {
	f.gotBefore = before
	f.gotLimit = limit
	return f.logs, f.logsErr
}

func (f *fakeE2E) Stream(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = w.Write([]byte("data: {\"kind\":\"status\"}\n\n"))
}

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
// Verifies: SDLC-VIS-01.02
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

// TestE2ERunsFeatureOff confirms the always-mounted recent-runs route reports
// the feature off (not an error) when the plane is unconfigured (E2E-LOG-01/02).
// Verifies: E2E-LOG-01.03
func TestE2ERunsFeatureOff(t *testing.T) {
	h := &Handlers{} // E2ERuns + E2E nil
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/e2e/runs", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (feature off, not error)", rr.Code)
	}
	var snap cache.Snapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := snap.Data.(map[string]any)
	if enabled, _ := data["enabled"].(bool); enabled {
		t.Error("disabled feature should report enabled=false")
	}
}

// TestE2ERunsHealthy confirms a configured recent-runs source serves 200.
func TestE2ERunsHealthy(t *testing.T) {
	src := oneShotSource(t, sources.E2ERunsPayload{Enabled: true, Runs: []sources.E2ERunSummary{{ID: "hub-00/ns/run-1"}}})
	h := &Handlers{E2ERuns: src}
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/e2e/runs", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

// TestE2ERunDetailNotFound confirms an absent run addressed by id returns 404.
func TestE2ERunDetailNotFound(t *testing.T) {
	h := &Handlers{E2E: &fakeE2E{err: sources.ErrRunNotFound}}
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/e2e/runs/hub-00/ns/missing", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

// TestE2ERunDetailDegradedDoesNotLeakError confirms a Loki failure degrades the
// detail pane (stale 200) without echoing the raw error, which could carry the
// internal Loki identity, to the browser (E2E-LOG-08).
func TestE2ERunDetailDegradedDoesNotLeakError(t *testing.T) {
	leaky := errors.New(`Get "http://e2e-loki.hypershell-observability.svc:3100/loki/api/v1/query_range?query={log_type=\"application\"}&X-Scope-OrgID=hub-00": refused`)
	h := &Handlers{E2E: &fakeE2E{err: leaky}}
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/e2e/runs/hub-00/ns/run-1", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (degraded, not blanked)", rr.Code)
	}
	body := rr.Body.String()
	for _, leak := range []string{"e2e-loki", "svc:3100", "query_range", "log_type", "X-Scope-OrgID", "hub-00"} {
		if strings.Contains(body, leak) {
			t.Errorf("response leaked internal Loki identity %q: %s", leak, body)
		}
	}
}

// TestE2ERunDetailHealthy confirms a found run returns 200 with the detail.
func TestE2ERunDetailHealthy(t *testing.T) {
	detail := &sources.E2ERunDetail{E2ERunSummary: sources.E2ERunSummary{ID: "hub-00/ns/run-1", Verdict: "passed"}}
	h := &Handlers{E2E: &fakeE2E{detail: detail}}
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/e2e/runs/hub-00/ns/run-1", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

// TestE2ERunLogsPage confirms the paginated logs route serves a page wrapped in
// the cache snapshot envelope and forwards the before cursor and parsed limit to
// the source (E2E-LOG-19).
func TestE2ERunLogsPage(t *testing.T) {
	fake := &fakeE2E{logs: &sources.E2ELogPage{
		Entries:    []sources.E2ELogLine{{Sequence: 1, Message: "hello"}},
		NextCursor: "42",
		HasMore:    true,
	}}
	h := &Handlers{E2E: fake}
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/e2e/logs?run=hub-00/ns/run-1&before=100&limit=150", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if fake.gotBefore != "100" {
		t.Errorf("before = %q, want 100", fake.gotBefore)
	}
	if fake.gotLimit != 150 {
		t.Errorf("limit = %d, want 150", fake.gotLimit)
	}
	var snap cache.Snapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := snap.Data.(map[string]any)
	if data["nextCursor"] != "42" {
		t.Errorf("nextCursor = %v, want 42", data["nextCursor"])
	}
	if hasMore, _ := data["hasMore"].(bool); !hasMore {
		t.Error("hasMore should be true")
	}
}

// TestE2ERunLogsNotFound confirms the logs route honors the tenant allowlist the
// same way the detail route does: an unaddressable tenant 404s (E2E-LOG-19.04).
func TestE2ERunLogsNotFound(t *testing.T) {
	h := &Handlers{E2E: &fakeE2E{logsErr: sources.ErrRunNotFound}}
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/e2e/logs?run=other/ns/run-1", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

// TestE2ERunLogsDegradedDoesNotLeakError confirms a Loki failure on the logs route
// degrades to a stale 200 without echoing the raw error (E2E-LOG-08/20).
func TestE2ERunLogsDegradedDoesNotLeakError(t *testing.T) {
	leaky := errors.New(`Get "http://e2e-loki.hypershell-observability.svc:3100/loki/api/v1/query_range": refused`)
	h := &Handlers{E2E: &fakeE2E{logsErr: leaky}}
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/e2e/logs?run=hub-00/ns/run-1", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (degraded, not blanked)", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "e2e-loki") || strings.Contains(rr.Body.String(), "query_range") {
		t.Errorf("response leaked internal Loki identity: %s", rr.Body.String())
	}
}

// TestE2ERunLogsFeatureOff confirms the logs route reports the feature off when
// the plane is unconfigured, rather than 404ing (E2E-LOG-01/02).
func TestE2ERunLogsFeatureOff(t *testing.T) {
	h := &Handlers{} // E2E nil
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/e2e/logs?run=hub-00/ns/run-1", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (feature off)", rr.Code)
	}
}

// TestE2EStreamContentType confirms the stream route produces an SSE response.
func TestE2EStreamContentType(t *testing.T) {
	h := &Handlers{E2E: &fakeE2E{}}
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/e2e/stream?run=hub-00/ns/run-1", nil))
	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if !contains(rr.Body.String(), "data:") {
		t.Error("expected at least one SSE data frame")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
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
