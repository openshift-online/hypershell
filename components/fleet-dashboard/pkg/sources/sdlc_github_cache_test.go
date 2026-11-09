package sources

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// fakeGitHub serves fixed JSON bodies by path and behaves like GitHub for
// conditional requests: every 200 carries an ETag, and a request whose
// If-None-Match matches gets 304. It counts full (200) and 304 answers per path.
type fakeGitHub struct {
	mu     sync.Mutex
	bodies map[string]string
	full   map[string]int
	notMod map[string]int
}

func newFakeGitHub(bodies map[string]string) *fakeGitHub {
	return &fakeGitHub{bodies: bodies, full: map[string]int{}, notMod: map[string]int{}}
}

func (f *fakeGitHub) set(path, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies[path] = body
}

func (f *fakeGitHub) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.full = map[string]int{}
	f.notMod = map[string]int{}
}

// counts returns the number of full answers per path and the total 304 answers.
func (f *fakeGitHub) counts() (map[string]int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	full := map[string]int{}
	for k, v := range f.full {
		full[k] = v
	}
	n := 0
	for _, v := range f.notMod {
		n += v
	}
	return full, n
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, ok := f.bodies[r.URL.Path]
	if !ok || r.URL.Query().Get("page") != "1" {
		body = `[]`
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256([]byte(body)))
	if r.Header.Get("If-None-Match") == etag {
		f.notMod[r.URL.Path]++
		w.WriteHeader(http.StatusNotModified)
		return
	}
	f.full[r.URL.Path]++
	w.Header().Set("ETag", etag)
	_, _ = w.Write([]byte(body))
}

const (
	cacheListPath = "/repos/acme/hypershell/issues"
	cacheTL10     = "/repos/acme/hypershell/issues/10/timeline"
	cacheTL20     = "/repos/acme/hypershell/issues/20/timeline"
)

func cacheListBody(updated10, updated20 string) string {
	return fmt.Sprintf(`[
		{"number":10,"title":"a","state":"open","updated_at":%q,"labels":[{"name":"agent/allowed"},{"name":"agent/reviewable-code"}]},
		{"number":20,"title":"b","state":"closed","closed_at":"2026-10-09T00:00:00Z","updated_at":%q,"labels":[{"name":"agent/allowed"}]}
	]`, updated10, updated20)
}

// TestGitHubSDLCSecondRefreshListOnly confirms the refresh cost does not grow with
// the number of agent/allowed issues: a second refresh with unchanged issues makes
// only the issue-list call, and that call is a conditional request answered 304
// (which GitHub does not count against the rate limit). No timeline is re-read.
// Verifies: SDLC-VIS-04.03
func TestGitHubSDLCSecondRefreshListOnly(t *testing.T) {
	fake := newFakeGitHub(map[string]string{
		cacheListPath: cacheListBody("2026-10-05T00:00:00Z", "2026-10-09T00:00:00Z"),
		cacheTL10:     `[{"event":"labeled","created_at":"2026-10-05T00:00:00Z","label":{"name":"agent/reviewable-code"}}]`,
		cacheTL20:     `[{"event":"labeled","created_at":"2026-10-01T00:00:00Z","label":{"name":"agent/allowed"}}]`,
	})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	g := NewGitHubSDLC(&config.Config{UpstreamRepo: "acme/hypershell", GitHubAPIBase: srv.URL})
	ctx := context.Background()

	first, err := g.Records(ctx)
	if err != nil {
		t.Fatalf("first Records: %v", err)
	}
	full, _ := fake.counts()
	if full[cacheTL10] != 1 || full[cacheTL20] != 1 {
		t.Fatalf("first refresh should read each timeline once, got %v", full)
	}

	fake.reset()
	second, err := g.Records(ctx)
	if err != nil {
		t.Fatalf("second Records: %v", err)
	}
	full, notMod := fake.counts()
	if len(full) != 0 {
		t.Errorf("second refresh with unchanged issues made counted calls: %v", full)
	}
	if notMod != 1 {
		t.Errorf("second refresh should make exactly one conditional list call, got %d 304 answers", notMod)
	}
	if len(second) != len(first) {
		t.Fatalf("second refresh returned %d records, want %d", len(second), len(first))
	}
	for i := range first {
		if first[i].PhaseSince != second[i].PhaseSince || len(first[i].History) != len(second[i].History) {
			t.Errorf("record #%d changed across a refresh with no GitHub change", first[i].Number)
		}
	}
}

// TestGitHubSDLCChangedIssueRefetchesOnlyItsTimeline confirms that when one issue's
// updated_at moves (for example a label change), only that issue's timeline is
// re-read and its new history is used.
// Verifies: SDLC-VIS-04.03
func TestGitHubSDLCChangedIssueRefetchesOnlyItsTimeline(t *testing.T) {
	fake := newFakeGitHub(map[string]string{
		cacheListPath: cacheListBody("2026-10-05T00:00:00Z", "2026-10-09T00:00:00Z"),
		cacheTL10:     `[{"event":"labeled","created_at":"2026-10-05T00:00:00Z","label":{"name":"agent/reviewable-code"}}]`,
		cacheTL20:     `[{"event":"labeled","created_at":"2026-10-01T00:00:00Z","label":{"name":"agent/allowed"}}]`,
	})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	g := NewGitHubSDLC(&config.Config{UpstreamRepo: "acme/hypershell", GitHubAPIBase: srv.URL})
	ctx := context.Background()
	if _, err := g.Records(ctx); err != nil {
		t.Fatalf("first Records: %v", err)
	}

	// #10 is rejected and re-applied: its updated_at and timeline change.
	fake.set(cacheListPath, cacheListBody("2026-10-07T00:00:00Z", "2026-10-09T00:00:00Z"))
	fake.set(cacheTL10, `[
		{"event":"labeled","created_at":"2026-10-05T00:00:00Z","label":{"name":"agent/reviewable-code"}},
		{"event":"labeled","created_at":"2026-10-07T00:00:00Z","label":{"name":"agent/review-code-rejected"}}
	]`)
	fake.reset()

	records, err := g.Records(ctx)
	if err != nil {
		t.Fatalf("second Records: %v", err)
	}
	full, _ := fake.counts()
	if full[cacheListPath] != 1 || full[cacheTL10] != 1 || full[cacheTL20] != 0 {
		t.Errorf("want one list call and only #10's timeline re-read, got %v", full)
	}
	for _, r := range records {
		if r.Number == 10 && r.ReworkCount != 1 {
			t.Errorf("#10 rework = %d, want 1 from the re-read timeline", r.ReworkCount)
		}
	}
}

// TestGitHubSDLCFailedTimelineNotCached confirms a failed timeline read is retried
// on the next refresh instead of pinning an empty history.
func TestGitHubSDLCFailedTimelineNotCached(t *testing.T) {
	fake := newFakeGitHub(map[string]string{
		cacheListPath: cacheListBody("2026-10-05T00:00:00Z", "2026-10-09T00:00:00Z"),
		cacheTL20:     `[]`,
		// cacheTL10 is absent: the fake answers 404.
	})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	g := NewGitHubSDLC(&config.Config{UpstreamRepo: "acme/hypershell", GitHubAPIBase: srv.URL})
	ctx := context.Background()
	if _, err := g.Records(ctx); err != nil {
		t.Fatalf("first Records: %v", err)
	}
	fake.set(cacheTL10, `[{"event":"labeled","created_at":"2026-10-05T00:00:00Z","label":{"name":"agent/reviewable-code"}}]`)
	fake.reset()
	records, err := g.Records(ctx)
	if err != nil {
		t.Fatalf("second Records: %v", err)
	}
	full, _ := fake.counts()
	if full[cacheTL10] != 1 || full[cacheTL20] != 0 {
		t.Errorf("want only #10's failed timeline retried, got %v", full)
	}
	for _, r := range records {
		if r.Number == 10 && r.PhaseSince != "2026-10-05T00:00:00Z" {
			t.Errorf("#10 phaseSince = %q after retry, want 2026-10-05T00:00:00Z", r.PhaseSince)
		}
	}
}

// TestGitHubWorkItemPRsConditionalRetry confirms the retry of a work item with no
// linked PRs is a conditional request: an unchanged timeline answers 304 (not
// counted against the rate limit) and still yields the same empty result.
// Verifies: SDLC-VIS-04.03
func TestGitHubWorkItemPRsConditionalRetry(t *testing.T) {
	const path = "/repos/acme/hypershell/issues/600/timeline"
	fake := newFakeGitHub(map[string]string{
		path: `[{"event":"labeled","label":{"name":"agent/workable"}}]`,
	})
	srv := httptest.NewServer(fake)
	defer srv.Close()

	g := NewGitHubWorkItemPRs(&config.Config{UpstreamRepo: "acme/hypershell", GitHubAPIBase: srv.URL})
	ctx := context.Background()
	_ = g.AssociatedPRs(ctx, SdlcRecord{Number: 600})
	fake.reset()
	if got := g.AssociatedPRs(ctx, SdlcRecord{Number: 600}); len(got) != 0 {
		t.Errorf("AssociatedPRs(600) = %v, want empty", got)
	}
	full, notMod := fake.counts()
	if len(full) != 0 || notMod != 1 {
		t.Errorf("retry should be one 304 conditional call, got full=%v 304s=%d", full, notMod)
	}

	// Once a PR cross-references the issue, the timeline changes and is re-read.
	fake.set(path, `[{"event":"cross-referenced","source":{"issue":{"number":101,"pull_request":{"url":"u"}}}}]`)
	if got := g.AssociatedPRs(ctx, SdlcRecord{Number: 600}); len(got) != 1 || got[0] != 101 {
		t.Errorf("AssociatedPRs(600) after link = %v, want [101]", got)
	}
}

// TestGitHubETagCacheBounded confirms the ETag cache never grows past its bound.
func TestGitHubETagCacheBounded(t *testing.T) {
	c := newGitHubETagCache(2)
	for i := 0; i < 5; i++ {
		c.put(fmt.Sprintf("u%d", i), etagEntry{etag: "e"})
	}
	if n := len(c.entries); n != 2 {
		t.Errorf("cache holds %d entries, want bound 2", n)
	}
}
