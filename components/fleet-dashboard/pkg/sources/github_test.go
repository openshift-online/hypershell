package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// TestMatchAnalysisURL covers the selection rule: exact name wins over a prefix
// match regardless of order, and absence yields "".
func TestMatchAnalysisURL(t *testing.T) {
	cr := checkRunsResponse{}
	cr.CheckRuns = []struct {
		Name    string `json:"name"`
		HTMLURL string `json:"html_url"`
	}{
		{Name: "hypershell-analysis-mirror", HTMLURL: "https://gh/prefix"},
		{Name: "other-check", HTMLURL: "https://gh/other"},
		{Name: "hypershell-analysis", HTMLURL: "https://gh/exact"},
	}
	if got := matchAnalysisURL(cr); got != "https://gh/exact" {
		t.Errorf("exact match should win: got %q", got)
	}

	// Prefix-only fallback.
	cr.CheckRuns = cr.CheckRuns[:1]
	if got := matchAnalysisURL(cr); got != "https://gh/prefix" {
		t.Errorf("prefix fallback: got %q", got)
	}

	// None present.
	cr.CheckRuns = cr.CheckRuns[1:1]
	if got := matchAnalysisURL(cr); got != "" {
		t.Errorf("no analysis run should yield empty, got %q", got)
	}
}

// TestGitHubAnalysisURL exercises the full fetch+cache path against a fake GitHub
// API: it resolves the exact check-run, sends the installation token, caches
// positive results (no second HTTP call), and never caches a miss.
func TestGitHubAnalysisURL(t *testing.T) {
	var calls int32
	tokenSeen := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		tokenSeen = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/repos/acme/gitops/commits/goodsha/check-runs":
			_, _ = w.Write([]byte(`{"check_runs":[{"name":"hypershell-analysis","html_url":"https://gh/run/1"}]}`))
		case "/repos/acme/gitops/commits/missing/check-runs":
			_, _ = w.Write([]byte(`{"check_runs":[{"name":"something-else","html_url":"https://gh/x"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tokFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokFile, []byte("inst-token-123\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	g := NewGitHubAnalysis(&config.Config{
		GitHubRepo:      "acme/gitops",
		GitHubAPIBase:   srv.URL,
		GitHubTokenFile: tokFile,
	})
	if g == nil {
		t.Fatal("NewGitHubAnalysis returned nil for a configured repo")
	}

	ctx := context.Background()

	// Empty SHA short-circuits with no HTTP call.
	if got := g.AnalysisURL(ctx, ""); got != "" {
		t.Errorf("empty sha: got %q", got)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Errorf("empty sha should not hit the API")
	}

	// Resolve + token header.
	if got := g.AnalysisURL(ctx, "goodsha"); got != "https://gh/run/1" {
		t.Fatalf("goodsha: got %q", got)
	}
	if tokenSeen != "Bearer inst-token-123" {
		t.Errorf("installation token not forwarded: %q", tokenSeen)
	}

	// Second call is served from cache (no new HTTP request).
	if got := g.AnalysisURL(ctx, "goodsha"); got != "https://gh/run/1" {
		t.Fatalf("goodsha cached: got %q", got)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("positive result not cached: %d API calls, want 1", n)
	}

	// A miss returns "" and is NOT cached, so it is retried (analysis may land later).
	if got := g.AnalysisURL(ctx, "missing"); got != "" {
		t.Errorf("missing analysis: got %q", got)
	}
	if got := g.AnalysisURL(ctx, "missing"); got != "" {
		t.Errorf("missing analysis (retry): got %q", got)
	}
	if n := atomic.LoadInt32(&calls); n != 3 {
		t.Errorf("miss should be retried (not cached): %d API calls, want 3", n)
	}
}

// TestNewGitHubAnalysisDisabled confirms the feature is opt-in: no repo => nil.
func TestNewGitHubAnalysisDisabled(t *testing.T) {
	if g := NewGitHubAnalysis(&config.Config{}); g != nil {
		t.Errorf("NewGitHubAnalysis should return nil when FD_GITHUB_REPO is unset")
	}
}
