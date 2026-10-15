package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// TestNewGitHubSDLCDisabled confirms the adapter is off when no source repo is set.
func TestNewGitHubSDLCDisabled(t *testing.T) {
	if g := NewGitHubSDLC(&config.Config{}); g != nil {
		t.Errorf("NewGitHubSDLC should return nil when FD_UPSTREAM_REPO is unset")
	}
	if g := NewGitHubIssueBundles(&config.Config{}); g != nil {
		t.Errorf("NewGitHubIssueBundles should return nil when FD_UPSTREAM_REPO is unset")
	}
}

// TestGitHubSDLCRecords exercises the full adapter path against a fake GitHub API:
// the execution-record issues are listed (pull requests skipped), each issue's
// label history is reconstructed from the timeline, and the current phase,
// time-in-phase start, and rework count are derived from it. This covers the
// SDLC-VIS-14 scenarios: time-in-phase from the latest phase label event, and a
// rework count that survives reject-label removal on retry.
func TestGitHubSDLCRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/hypershell/issues":
			if r.URL.Query().Get("page") != "1" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			// A code-phase execution record (#10), a spec-phase record with a
			// removed-then-reapplied reject label (#20), and a pull request that the
			// issues endpoint also returns (#99) which must be skipped.
			_, _ = w.Write([]byte(`[
				{"number":10,"title":"Add SDLC tab","html_url":"https://github.com/acme/hypershell/issues/10","state":"open","labels":[{"name":"agent/execution-record"},{"name":"agent/reviewable-code"}]},
				{"number":20,"title":"Spec rework","html_url":"https://github.com/acme/hypershell/issues/20","state":"open","labels":[{"name":"agent/execution-record"},{"name":"agent/review-spec-rejected"}]},
				{"number":99,"title":"a pull request","state":"open","pull_request":{"url":"x"},"labels":[{"name":"agent/execution-record"}]}
			]`))
		case "/repos/acme/hypershell/issues/10/timeline":
			if r.URL.Query().Get("page") != "1" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[
				{"event":"labeled","created_at":"2026-10-01T00:00:00Z","label":{"name":"agent/workable"}},
				{"event":"commented","created_at":"2026-10-02T00:00:00Z"},
				{"event":"labeled","created_at":"2026-10-03T00:00:00Z","label":{"name":"agent/reviewable-spec"}},
				{"event":"labeled","created_at":"2026-10-05T00:00:00Z","label":{"name":"agent/reviewable-code"}}
			]`))
		case "/repos/acme/hypershell/issues/20/timeline":
			if r.URL.Query().Get("page") != "1" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[
				{"event":"labeled","created_at":"2026-10-01T00:00:00Z","label":{"name":"agent/reviewable-spec"}},
				{"event":"labeled","created_at":"2026-10-02T00:00:00Z","label":{"name":"agent/review-spec-rejected"}},
				{"event":"unlabeled","created_at":"2026-10-03T00:00:00Z","label":{"name":"agent/review-spec-rejected"}},
				{"event":"labeled","created_at":"2026-10-04T00:00:00Z","label":{"name":"agent/review-spec-rejected"}}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := NewGitHubSDLC(&config.Config{UpstreamRepo: "acme/hypershell", GitHubAPIBase: srv.URL})
	if g == nil {
		t.Fatal("NewGitHubSDLC returned nil for a configured repo")
	}
	records, err := g.Records(context.Background())
	if err != nil {
		t.Fatalf("Records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 records (pull request skipped), got %d", len(records))
	}
	byNum := map[int]SdlcRecord{}
	for _, r := range records {
		byNum[r.Number] = r
	}

	ten := byNum[10]
	if ten.Phase != PhaseCode {
		t.Errorf("#10 phase = %q, want %q", ten.Phase, PhaseCode)
	}
	// Time-in-phase is measured from the latest labeled event for the current phase
	// (reviewable-code at 2026-10-05), not an earlier phase's event.
	if ten.PhaseSince != "2026-10-05T00:00:00Z" {
		t.Errorf("#10 phaseSince = %q, want 2026-10-05T00:00:00Z", ten.PhaseSince)
	}
	if ten.ReworkCount != 0 {
		t.Errorf("#10 rework = %d, want 0", ten.ReworkCount)
	}

	twenty := byNum[20]
	if twenty.Phase != PhaseSpec {
		t.Errorf("#20 phase = %q, want %q", twenty.Phase, PhaseSpec)
	}
	// The reject label was applied, removed on retry, then applied again: the count
	// is the number of `labeled` events (2), which the live label set cannot retain.
	if twenty.ReworkCount != 2 {
		t.Errorf("#20 rework = %d, want 2 (derived from two labeled events)", twenty.ReworkCount)
	}
}

// TestGitHubSDLCRecordsListFailure confirms a hard failure to list the in-scope set
// returns an error (so the plane can go stale / 503), not a silent empty success.
func TestGitHubSDLCRecordsListFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	g := NewGitHubSDLC(&config.Config{UpstreamRepo: "acme/hypershell", GitHubAPIBase: srv.URL})
	if _, err := g.Records(context.Background()); err == nil {
		t.Fatal("expected an error when the execution-record listing fails")
	}
}

// TestGitHubIssueBundlesClosingIssue confirms the PR->closing-issue hop parses the
// closing reference and caches successful lookups (no second HTTP call).
func TestGitHubIssueBundlesClosingIssue(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/repos/acme/hypershell/pulls/568":
			_, _ = w.Write([]byte(`{"body":"This adds the tab.\n\nCloses #565\n"}`))
		case "/repos/acme/hypershell/pulls/600":
			_, _ = w.Write([]byte(`{"body":"No closing reference here."}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := NewGitHubIssueBundles(&config.Config{UpstreamRepo: "acme/hypershell", GitHubAPIBase: srv.URL})
	ctx := context.Background()
	if got := g.ClosingIssue(ctx, 568); got != 565 {
		t.Errorf("ClosingIssue(568) = %d, want 565", got)
	}
	if got := g.ClosingIssue(ctx, 600); got != 0 {
		t.Errorf("ClosingIssue(600) = %d, want 0 (no closing reference)", got)
	}
	before := calls
	if got := g.ClosingIssue(ctx, 568); got != 565 {
		t.Errorf("cached ClosingIssue(568) = %d, want 565", got)
	}
	if calls != before {
		t.Errorf("successful lookup not cached: %d extra calls", calls-before)
	}
}

// TestClosesRefVariants confirms the closing-reference regex matches the common
// GitHub closing keywords and ignores bare mentions.
func TestClosesRefVariants(t *testing.T) {
	cases := map[string]int{
		"Closes #565":             565,
		"closes: #42":             42,
		"Fixes #7":                7,
		"resolved #9":             9,
		"see #100 for context":    0,
		"relates to HYPERSHELL-1": 0,
	}
	for body, want := range cases {
		got := 0
		if m := closesRef.FindStringSubmatch(body); m != nil {
			got = mustAtoi(m[1])
		}
		if got != want {
			t.Errorf("closesRef(%q) = %d, want %d", body, got, want)
		}
	}
}

func mustAtoi(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		n = n*10 + int(r-'0')
	}
	return n
}
