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

// TestHTMLBaseFrom covers the API-base -> web-base derivation for github.com and
// GitHub Enterprise.
func TestHTMLBaseFrom(t *testing.T) {
	cases := map[string]string{
		"":                           "https://github.com",
		"https://api.github.com":     "https://github.com",
		"https://ghe.example/api/v3": "https://ghe.example",
	}
	for in, want := range cases {
		if got := htmlBaseFrom(in); got != want {
			t.Errorf("htmlBaseFrom(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNewGitHubBundlesDisabled confirms the feature is opt-in: no repo => nil.
func TestNewGitHubBundlesDisabled(t *testing.T) {
	if g := NewGitHubBundles(&config.Config{}); g != nil {
		t.Errorf("NewGitHubBundles should return nil when FD_GITHUB_REPO is unset")
	}
}

// TestGitHubBundlesEnrich exercises the full enrich path against a fake GitHub
// API: it resolves each release's date, orders oldest-first, attaches the PRs of
// the newer build (parsed from the "(#n)" trailer, newest-first, with author and
// constructed URL), forwards the installation token, and caches results.
func TestGitHubBundlesEnrich(t *testing.T) {
	var calls int32
	tokenSeen := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		tokenSeen = r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/repos/acme/gitops/commits/oldsha0001":
			_, _ = w.Write([]byte(`{"commit":{"author":{"date":"2026-09-01T00:00:00Z"}}}`))
		case "/repos/acme/gitops/commits/newsha0002":
			_, _ = w.Write([]byte(`{"commit":{"author":{"date":"2026-09-10T00:00:00Z"}}}`))
		case "/repos/acme/gitops/compare/oldsha0001...newsha0002":
			_, _ = w.Write([]byte(`{"commits":[
				{"commit":{"message":"First change (#101)","author":{"date":"2026-09-05T00:00:00Z"}},"author":{"login":"alice"}},
				{"commit":{"message":"Merge branch 'main'","author":{"date":"2026-09-06T00:00:00Z"}},"author":{"login":"bot"}},
				{"commit":{"message":"Second change (#102)\n\nbody","author":{"date":"2026-09-07T00:00:00Z"}},"author":null}
			]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tokFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokFile, []byte("inst-token-123\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	g := NewGitHubBundles(&config.Config{
		GitHubRepo:      "acme/gitops",
		GitHubAPIBase:   srv.URL,
		GitHubTokenFile: tokFile,
	})
	if g == nil {
		t.Fatal("NewGitHubBundles returned nil for a configured repo")
	}

	releases := map[string]*Release{
		"oldsha0001": {Version: "oldsha00", SHA: "oldsha0001"},
		"newsha0002": {Version: "newsha00", SHA: "newsha0002"},
	}
	ctx := context.Background()
	g.Enrich(ctx, releases)

	if tokenSeen != "Bearer inst-token-123" {
		t.Errorf("installation token not forwarded: %q", tokenSeen)
	}
	if got := releases["oldsha0001"].Date; got != "2026-09-01T00:00:00Z" {
		t.Errorf("old date: %q", got)
	}
	if got := releases["newsha0002"].Date; got != "2026-09-10T00:00:00Z" {
		t.Errorf("new date: %q", got)
	}
	// Oldest build has no prior build to diff against.
	if releases["oldsha0001"].PRs != nil {
		t.Errorf("oldest build should carry no PRs, got %+v", releases["oldsha0001"].PRs)
	}

	prs := releases["newsha0002"].PRs
	if len(prs) != 2 {
		t.Fatalf("want 2 PRs (merge commit skipped), got %d: %+v", len(prs), prs)
	}
	// Newest-first ordering: #102 before #101.
	if prs[0].Number != 102 || prs[1].Number != 101 {
		t.Errorf("PR order should be newest-first: got %d then %d", prs[0].Number, prs[1].Number)
	}
	if prs[1].Title != "First change" || prs[1].Author != "alice" {
		t.Errorf("PR #101 title/author: %q / %q", prs[1].Title, prs[1].Author)
	}
	// The test API base is the httptest server, so the derived HTML base is that
	// server (htmlBaseFrom's github.com path is covered separately).
	if want := srv.URL + "/acme/gitops/pull/101"; prs[1].URL != want {
		t.Errorf("PR #101 url: %q, want %q", prs[1].URL, want)
	}
	if prs[0].Title != "Second change" {
		t.Errorf("PR #102 title should strip trailer and body: %q", prs[0].Title)
	}
	if prs[0].Author != "" {
		t.Errorf("PR #102 author should be empty when login is null: %q", prs[0].Author)
	}

	// Second Enrich is fully served from cache (no new HTTP calls).
	before := atomic.LoadInt32(&calls)
	g.Enrich(ctx, releases)
	if after := atomic.LoadInt32(&calls); after != before {
		t.Errorf("results not cached: %d new API calls", after-before)
	}
}

// TestComparePRsSortedByMergeTime confirms the PR list is ordered by merge time
// (newest first) rather than by the compare API's raw commit order. The fake
// returns commits whose "(#n)" trailers are OUT of chronological order, so a plain
// reverse of the API order would mis-sort them; only an explicit MergedAt sort
// yields the expected newest-first result.
func TestComparePRsSortedByMergeTime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/gitops/compare/base...head" {
			// Deliberately scrambled: dates do not increase with position, and the
			// PR number order does not match the merge-time order either.
			_, _ = w.Write([]byte(`{"commits":[
				{"commit":{"message":"Middle (#200)","author":{"date":"2026-09-05T12:00:00Z"}},"author":{"login":"a"}},
				{"commit":{"message":"Newest (#150)","author":{"date":"2026-09-09T12:00:00Z"}},"author":{"login":"b"}},
				{"commit":{"message":"Oldest (#300)","author":{"date":"2026-09-01T12:00:00Z"}},"author":{"login":"c"}}
			]}`))
			return
		}
		t.Errorf("unexpected call to %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := NewGitHubBundles(&config.Config{GitHubRepo: "acme/gitops", GitHubAPIBase: srv.URL})
	prs := g.comparePRs(context.Background(), "base", "head")

	wantNums := []int{150, 200, 300} // newest merge time first
	if len(prs) != len(wantNums) {
		t.Fatalf("want %d PRs, got %d: %+v", len(wantNums), len(prs), prs)
	}
	for i, want := range wantNums {
		if prs[i].Number != want {
			t.Errorf("position %d: got #%d, want #%d (order: %v)", i, prs[i].Number, want, prNums(prs))
		}
	}
}

// prNums is a tiny helper for readable order assertions.
func prNums(prs []PR) []int {
	out := make([]int, len(prs))
	for i, p := range prs {
		out[i] = p.Number
	}
	return out
}

// TestGitHubBundlesSingleRelease confirms a lone release (no prior build) does
// no compare call and gets no PRs.
func TestGitHubBundlesSingleRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/gitops/commits/onlysha001" {
			_, _ = w.Write([]byte(`{"commit":{"author":{"date":"2026-09-01T00:00:00Z"}}}`))
			return
		}
		t.Errorf("unexpected call to %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := NewGitHubBundles(&config.Config{GitHubRepo: "acme/gitops", GitHubAPIBase: srv.URL})
	releases := map[string]*Release{"onlysha001": {SHA: "onlysha001"}}
	g.Enrich(context.Background(), releases)
	if releases["onlysha001"].PRs != nil {
		t.Errorf("single release should have no PRs")
	}
}
