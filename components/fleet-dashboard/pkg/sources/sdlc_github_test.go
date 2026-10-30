package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// TestNewGitHubSDLCDisabled confirms the adapter is off when no source repo is set.
func TestNewGitHubSDLCDisabled(t *testing.T) {
	if g := NewGitHubSDLC(&config.Config{}); g != nil {
		t.Errorf("NewGitHubSDLC should return nil when FD_UPSTREAM_REPO is unset")
	}
	if g := NewGitHubWorkItemPRs(&config.Config{}); g != nil {
		t.Errorf("NewGitHubWorkItemPRs should return nil when FD_UPSTREAM_REPO is unset")
	}
}

// TestGitHubSDLCRecords exercises the full adapter path against a fake GitHub API:
// the allowed work items are listed (pull requests skipped), each issue's label
// history is reconstructed from the timeline, and the current phase,
// time-in-phase start, and rework count are derived from it. This covers the
// SDLC-VIS-14 scenarios: time-in-phase from the latest phase label event, and a
// rework count that survives reject-label removal on retry.
// Verifies: SDLC-VIS-14.01, SDLC-VIS-14.02
func TestGitHubSDLCRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/hypershell/issues":
			if r.URL.Query().Get("page") != "1" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			// A code-phase work item (#10), a spec-phase record with a
			// removed-then-reapplied reject label (#20), and a pull request that the
			// issues endpoint also returns (#99) which must be skipped.
			_, _ = w.Write([]byte(`[
				{"number":10,"title":"Add SDLC tab","html_url":"https://github.com/acme/hypershell/issues/10","state":"open","labels":[{"name":"agent/allowed"},{"name":"agent/reviewable-code"}]},
				{"number":20,"title":"Spec rework","html_url":"https://github.com/acme/hypershell/issues/20","state":"open","labels":[{"name":"agent/allowed"},{"name":"agent/review-spec-rejected"}]},
				{"number":99,"title":"a pull request","state":"open","pull_request":{"url":"x"},"labels":[{"name":"agent/allowed"}]}
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
		t.Fatal("expected an error when the allowed work-item listing fails")
	}
}

// TestGitHubSDLCAllowedOnlyIntake confirms that an issue carrying agent/allowed
// with no agent/execution-record or further funnel label lands in Intake with
// PhaseSince from the agent/allowed labeled event.
// Verifies: SDLC-VIS-23.01, SDLC-VIS-23.02
func TestGitHubSDLCAllowedOnlyIntake(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/hypershell/issues":
			if r.URL.Query().Get("page") != "1" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			if r.URL.Query().Get("labels") != "agent/allowed" {
				t.Errorf("listing must filter by agent/allowed, got labels=%q", r.URL.Query().Get("labels"))
			}
			_, _ = w.Write([]byte(`[
				{"number":42,"title":"Allowed only","html_url":"https://github.com/acme/hypershell/issues/42","state":"open","labels":[{"name":"agent/allowed"},{"name":"agent/scope/test0"}]},
				{"number":43,"title":"With execution record","html_url":"https://github.com/acme/hypershell/issues/43","state":"open","labels":[{"name":"agent/allowed"},{"name":"agent/execution-record"},{"name":"agent/reviewable-spec"}]}
			]`))
		case "/repos/acme/hypershell/issues/42/timeline":
			if r.URL.Query().Get("page") != "1" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[
				{"event":"labeled","created_at":"2026-10-10T12:00:00Z","label":{"name":"agent/allowed"}}
			]`))
		case "/repos/acme/hypershell/issues/43/timeline":
			if r.URL.Query().Get("page") != "1" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[
				{"event":"labeled","created_at":"2026-10-08T00:00:00Z","label":{"name":"agent/allowed"}},
				{"event":"labeled","created_at":"2026-10-09T00:00:00Z","label":{"name":"agent/execution-record"}},
				{"event":"labeled","created_at":"2026-10-11T00:00:00Z","label":{"name":"agent/reviewable-spec"}}
			]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := NewGitHubSDLC(&config.Config{UpstreamRepo: "acme/hypershell", GitHubAPIBase: srv.URL})
	records, err := g.Records(context.Background())
	if err != nil {
		t.Fatalf("Records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("want 2 records, got %d", len(records))
	}

	byNum := map[int]SdlcRecord{}
	for _, r := range records {
		byNum[r.Number] = r
	}

	// SDLC-VIS-23.01: allowed-only issue lands in Intake
	rec42 := byNum[42]
	if rec42.Phase != PhaseIntake {
		t.Errorf("#42 phase = %q, want %q (allowed-only must land in Intake)", rec42.Phase, PhaseIntake)
	}
	// SDLC-VIS-23.02: PhaseSince from the agent/allowed labeled event
	if rec42.PhaseSince != "2026-10-10T12:00:00Z" {
		t.Errorf("#42 PhaseSince = %q, want 2026-10-10T12:00:00Z (from agent/allowed event)", rec42.PhaseSince)
	}
	if rec42.Scope != "agent/scope/test0" {
		t.Errorf("#42 Scope = %q, want agent/scope/test0", rec42.Scope)
	}

	// SDLC-VIS-23.03: issue with execution record + funnel label resolves to its phase
	rec43 := byNum[43]
	if rec43.Phase != PhaseSpec {
		t.Errorf("#43 phase = %q, want %q (existing record must resolve to its current phase)", rec43.Phase, PhaseSpec)
	}
	if rec43.PhaseSince != "2026-10-11T00:00:00Z" {
		t.Errorf("#43 PhaseSince = %q, want 2026-10-11T00:00:00Z", rec43.PhaseSince)
	}
}

// TestGitHubSDLCInertRecordExcluded confirms that an issue carrying
// agent/execution-record but NOT agent/allowed is excluded from the listing,
// because the listing filters by agent/allowed (SDLC-VIS-23.05).
func TestGitHubSDLCInertRecordExcluded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/hypershell/issues":
			if r.URL.Query().Get("labels") != "agent/allowed" {
				t.Errorf("listing must filter by agent/allowed, got labels=%q", r.URL.Query().Get("labels"))
			}
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := NewGitHubSDLC(&config.Config{UpstreamRepo: "acme/hypershell", GitHubAPIBase: srv.URL})
	records, err := g.Records(context.Background())
	if err != nil {
		t.Fatalf("Records: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("want 0 records (inert records excluded by agent/allowed filter), got %d", len(records))
	}
}

// TestGitHubWorkItemPRsTimeline confirms the unauthenticated work-item -> PRs hop
// reads the REST issue timeline, collects only cross-referenced events whose source
// is a pull request, dedupes, caches a non-empty result (no second call), and does
// NOT cache an empty result (so an as-yet-unlinked record is retried).
// Verifies: SDLC-VIS-03.05
func TestGitHubWorkItemPRsTimeline(t *testing.T) {
	calls := map[int]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/hypershell/issues/565/timeline":
			calls[565]++
			// Two PR cross-refs (one duplicated), a plain-issue cross-ref (not a PR),
			// and an unrelated labeled event -- only the PRs 101 and 102 should survive.
			_, _ = w.Write([]byte(`[
				{"event":"cross-referenced","source":{"issue":{"number":101,"pull_request":{"url":"u"}}}},
				{"event":"cross-referenced","source":{"issue":{"number":55}}},
				{"event":"cross-referenced","source":{"issue":{"number":102,"pull_request":{"url":"u"}}}},
				{"event":"cross-referenced","source":{"issue":{"number":101,"pull_request":{"url":"u"}}}},
				{"event":"labeled","label":{"name":"agent/workable"}}
			]`))
		case "/repos/acme/hypershell/issues/600/timeline":
			calls[600]++
			_, _ = w.Write([]byte(`[{"event":"labeled","label":{"name":"agent/workable"}}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := NewGitHubWorkItemPRs(&config.Config{UpstreamRepo: "acme/hypershell", GitHubAPIBase: srv.URL})
	ctx := context.Background()

	got := g.AssociatedPRs(ctx, SdlcRecord{Number: 565})
	if len(got) != 2 || got[0] != 101 || got[1] != 102 {
		t.Errorf("AssociatedPRs(565) = %v, want [101 102]", got)
	}
	// Non-empty result is cached: a second call must not re-hit the timeline.
	_ = g.AssociatedPRs(ctx, SdlcRecord{Number: 565})
	if calls[565] != 1 {
		t.Errorf("non-empty lookup not cached: %d timeline calls for #565", calls[565])
	}

	// An issue with no PR cross-refs returns empty and is NOT cached (retried).
	if got := g.AssociatedPRs(ctx, SdlcRecord{Number: 600}); len(got) != 0 {
		t.Errorf("AssociatedPRs(600) = %v, want empty", got)
	}
	_ = g.AssociatedPRs(ctx, SdlcRecord{Number: 600})
	if calls[600] != 2 {
		t.Errorf("empty lookup should not be cached: %d timeline calls for #600, want 2", calls[600])
	}
}

// TestGitHubWorkItemPRsGraphQL confirms that when an upstream token is configured the
// resolver uses the authoritative GraphQL closedByPullRequestsReferences set and does
// not fall back to the REST timeline.
func TestGitHubWorkItemPRsGraphQL(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	graphqlCalls := 0
	graphql := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		graphqlCalls++
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("graphql auth header = %q, want bearer token", got)
		}
		_, _ = w.Write([]byte(`{"data":{"repository":{"issue":{"closedByPullRequestsReferences":{"nodes":[{"number":321},{"number":322}]}}}}}`))
	}))
	defer graphql.Close()

	restCalls := 0
	rest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		restCalls++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer rest.Close()

	g := NewGitHubWorkItemPRs(&config.Config{
		UpstreamRepo:      "acme/hypershell",
		GitHubAPIBase:     rest.URL,
		GitHubGraphQL:     graphql.URL,
		UpstreamTokenFile: tokenFile,
	})
	got := g.AssociatedPRs(context.Background(), SdlcRecord{Number: 565})
	if len(got) != 2 || got[0] != 321 || got[1] != 322 {
		t.Errorf("AssociatedPRs(565) via GraphQL = %v, want [321 322]", got)
	}
	if graphqlCalls != 1 {
		t.Errorf("want exactly one GraphQL call, got %d", graphqlCalls)
	}
	if restCalls != 0 {
		t.Errorf("GraphQL success must not fall back to the REST timeline, got %d REST calls", restCalls)
	}
}
