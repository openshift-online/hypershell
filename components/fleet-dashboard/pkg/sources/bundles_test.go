package sources

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
// API: it resolves each release's date from the GITOPS repo (authenticated with
// the installation token), orders oldest-first, and attaches the newer build's
// PRs by diffing the UPSTREAM product-source revisions (ManifestsRev) over the
// public upstream repo UNAUTHENTICATED (parsed from the "(#n)" trailer,
// newest-first, with author and a URL pointing at the upstream repo). Results
// are cached.
func TestGitHubBundlesEnrich(t *testing.T) {
	var calls int32
	var mu sync.Mutex
	authByPath := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		mu.Lock()
		authByPath[r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()
		switch r.URL.Path {
		case "/repos/acme/gitops/commits/oldsha0001":
			_, _ = w.Write([]byte(`{"commit":{"author":{"date":"2026-09-01T00:00:00Z"}}}`))
		case "/repos/acme/gitops/commits/newsha0002":
			_, _ = w.Write([]byte(`{"commit":{"author":{"date":"2026-09-10T00:00:00Z"}}}`))
		case "/repos/acme/hypershell/compare/oldrev0001...newrev0002":
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
		UpstreamRepo:    "acme/hypershell",
		GitHubAPIBase:   srv.URL,
		GitHubTokenFile: tokFile,
	})
	if g == nil {
		t.Fatal("NewGitHubBundles returned nil for a configured repo")
	}

	releases := map[string]*Release{
		"oldsha0001": {Version: "oldsha00", SHA: "oldsha0001", ManifestsRev: "oldrev0001"},
		"newsha0002": {Version: "newsha00", SHA: "newsha0002", ManifestsRev: "newrev0002"},
	}
	ctx := context.Background()
	g.Enrich(ctx, releases)

	// Dates come from the gitops repo, authenticated with the installation token.
	if got := authByPath["/repos/acme/gitops/commits/newsha0002"]; got != "Bearer inst-token-123" {
		t.Errorf("gitops date call should forward the installation token, got %q", got)
	}
	// The upstream compare is a PUBLIC repo read -- no token (the installation
	// token is scoped to the gitops repo and must not leak to, or 403 against, a
	// different repo).
	if got := authByPath["/repos/acme/hypershell/compare/oldrev0001...newrev0002"]; got != "" {
		t.Errorf("upstream compare should be unauthenticated, got auth %q", got)
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
	// PR links point at the UPSTREAM repo, not the gitops repo. The test API base
	// is the httptest server, so the derived HTML base is that server
	// (htmlBaseFrom's github.com path is covered separately).
	if want := srv.URL + "/acme/hypershell/pull/101"; prs[1].URL != want {
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
		if r.URL.Path == "/repos/acme/hypershell/compare/base...head" {
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

	g := NewGitHubBundles(&config.Config{GitHubRepo: "acme/gitops", UpstreamRepo: "acme/hypershell", GitHubAPIBase: srv.URL})
	prs := g.comparePRs(context.Background(), "acme/hypershell", "", "base", "head")

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

// fakeLockHistory is a static LockHistoryProvider for GitOps-group tests.
type fakeLockHistory struct{ commits []LockCommit }

func (f fakeLockHistory) LockHistory(context.Context, int) []LockCommit { return f.commits }

// TestComparePRsMergeCommitFormat confirms the extraction recognizes the non-squash
// "Merge pull request #N from ..." subject (FLT-BPR-09.04) alongside the squash
// "(#n)" trailer, taking the PR title from the merge commit's body.
func TestComparePRsMergeCommitFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/gitops/compare/base...head" {
			_, _ = w.Write([]byte(`{"total_commits":2,"commits":[
				{"commit":{"message":"Merge pull request #77 from acme/feature\n\nAdd the feature","author":{"date":"2026-09-02T00:00:00Z"}},"author":{"login":"dev"}},
				{"commit":{"message":"Squashed change (#78)","author":{"date":"2026-09-03T00:00:00Z"}},"author":{"login":"dev2"}}
			]}`))
			return
		}
		t.Errorf("unexpected call to %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := NewGitHubBundles(&config.Config{GitHubRepo: "acme/gitops", GitHubAPIBase: srv.URL})
	prs := g.comparePRs(context.Background(), "acme/gitops", "", "base", "head")
	if len(prs) != 2 {
		t.Fatalf("want 2 PRs, got %d: %+v", len(prs), prs)
	}
	byNum := map[int]PR{}
	for _, p := range prs {
		byNum[p.Number] = p
	}
	if byNum[77].Title != "Add the feature" {
		t.Errorf("merge-commit PR title should come from the body: %q", byNum[77].Title)
	}
	if byNum[78].Title != "Squashed change" {
		t.Errorf("squash PR title: %q", byNum[78].Title)
	}
	for _, n := range []int{77, 78} {
		if byNum[n].Repository != "acme/gitops" {
			t.Errorf("PR #%d repository = %q, want acme/gitops", n, byNum[n].Repository)
		}
	}
}

// TestComparePRsDedup confirms a PR appearing twice in a range (FLT-BPR-02.02) is
// emitted once for that repository.
func TestComparePRsDedup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/hypershell/compare/base...head" {
			_, _ = w.Write([]byte(`{"total_commits":2,"commits":[
				{"commit":{"message":"Change (#90)","author":{"date":"2026-09-02T00:00:00Z"}},"author":{"login":"a"}},
				{"commit":{"message":"Change again (#90)","author":{"date":"2026-09-03T00:00:00Z"}},"author":{"login":"a"}}
			]}`))
			return
		}
		t.Errorf("unexpected call to %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := NewGitHubBundles(&config.Config{GitHubRepo: "acme/gitops", UpstreamRepo: "acme/hypershell", GitHubAPIBase: srv.URL})
	prs := g.comparePRs(context.Background(), "acme/hypershell", "", "base", "head")
	if len(prs) != 1 || prs[0].Number != 90 {
		t.Fatalf("want a single #90, got %v", prNums(prs))
	}
}

// TestComparePRsPagination confirms a multi-page compare is followed to completion
// (FLT-BPR-09.03): a full first page (per_page=100) triggers a second request.
func TestComparePRsPagination(t *testing.T) {
	// Page 1: 100 PR commits (#1000..#1099), full page -> continue. Page 2: one PR.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/gitops/compare/base...head" {
			t.Errorf("unexpected call to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		page := r.URL.Query().Get("page")
		if page == "1" {
			var b strings.Builder
			b.WriteString(`{"total_commits":101,"commits":[`)
			for i := 0; i < 100; i++ {
				if i > 0 {
					b.WriteByte(',')
				}
				fmt.Fprintf(&b, `{"commit":{"message":"Change (#%d)","author":{"date":"2026-09-01T00:00:00Z"}},"author":{"login":"a"}}`, 1000+i)
			}
			b.WriteString(`]}`)
			_, _ = w.Write([]byte(b.String()))
			return
		}
		_, _ = w.Write([]byte(`{"total_commits":101,"commits":[
			{"commit":{"message":"Last (#2000)","author":{"date":"2026-09-02T00:00:00Z"}},"author":{"login":"b"}}
		]}`))
	}))
	defer srv.Close()

	g := NewGitHubBundles(&config.Config{GitHubRepo: "acme/gitops", GitHubAPIBase: srv.URL})
	prs := g.comparePRs(context.Background(), "acme/gitops", "", "base", "head")
	if len(prs) != 101 {
		t.Fatalf("want 101 PRs across both pages, got %d", len(prs))
	}
	var found bool
	for _, p := range prs {
		if p.Number == 2000 {
			found = true
		}
	}
	if !found {
		t.Error("the page-2 PR #2000 was lost")
	}
}

// TestGitHubBundlesGitOpsGroup exercises the GitOps deployment-change group: the
// range is bounded by the recorded lock timeline (baseline = the commit before the
// bundle's introduction), extended to the environment's current source commit so a
// config-only PR shows without a new digest, read from the gitops repo with the
// installation token, and tagged with the gitops repository (FLT-BPR-02/04/05/09).
func TestGitHubBundlesGitOpsGroup(t *testing.T) {
	var mu sync.Mutex
	authByPath := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authByPath[r.URL.Path] = r.Header.Get("Authorization")
		mu.Unlock()
		switch r.URL.Path {
		case "/repos/acme/gitops/compare/giti0001...giti0002":
			// The introduction range: just the lock-update PR.
			_, _ = w.Write([]byte(`{"total_commits":1,"commits":[
				{"commit":{"message":"Update release lock (#500)","author":{"date":"2026-09-05T00:00:00Z"}},"author":{"login":"promoter"}}
			]}`))
		case "/repos/acme/gitops/compare/giti0001...gitcfg04":
			// The prod environment extends to a config-only PR.
			_, _ = w.Write([]byte(`{"total_commits":2,"commits":[
				{"commit":{"message":"Update release lock (#500)","author":{"date":"2026-09-05T00:00:00Z"}},"author":{"login":"promoter"}},
				{"commit":{"message":"Tune replicas (#501)","author":{"date":"2026-09-08T00:00:00Z"}},"author":{"login":"ops"}}
			]}`))
		default:
			t.Errorf("unexpected call to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tokFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokFile, []byte("inst-token-abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	g := NewGitHubBundles(&config.Config{
		GitHubRepo:      "acme/gitops",
		UpstreamRepo:    "acme/hypershell",
		GitHubAPIBase:   srv.URL,
		GitHubTokenFile: tokFile,
	}).WithLockHistory(fakeLockHistory{commits: []LockCommit{
		{SHA: "giti0003", Digest: "dig-b"},
		{SHA: "giti0002", Digest: "dig-a"},
		{SHA: "giti0001", Digest: "dig-z"},
	}})

	rel := &Release{
		Version: "v1",
		Digest:  "dig-a",
		Date:    "2026-09-05T00:00:00Z",
		DeploymentRevisions: []DeploymentRevision{
			{Env: "prod", DrySHA: "gitcfg04"},
		},
	}
	releases := map[string]*Release{"dig-a": rel}
	g.Enrich(context.Background(), releases)

	if rel.GitOpsStatus != gitOpsAvailable {
		t.Fatalf("GitOpsStatus = %q, want %q", rel.GitOpsStatus, gitOpsAvailable)
	}
	got := prNums(rel.GitOpsPRs)
	if len(got) != 2 || got[0] != 501 || got[1] != 500 {
		t.Fatalf("want GitOps PRs [501 500] (lock-update + config, newest first), got %v", got)
	}
	for _, p := range rel.GitOpsPRs {
		if p.Repository != "acme/gitops" {
			t.Errorf("GitOps PR #%d repository = %q, want acme/gitops", p.Number, p.Repository)
		}
	}
	// The application group is untouched for a single bundle (no prior build).
	if rel.PRs != nil {
		t.Errorf("application PRs should stay nil for a single bundle, got %+v", rel.PRs)
	}
	// The gitops compare uses the installation token (unlike the public upstream).
	if got := authByPath["/repos/acme/gitops/compare/giti0001...gitcfg04"]; got != "Bearer inst-token-abc" {
		t.Errorf("gitops compare should forward the installation token, got %q", got)
	}
}

// TestGitHubBundlesGitOpsDivergentEnvironments proves FLT-BPR-06: when two
// environments on one bundle digest run different GitOps revisions, a PR present in
// only one environment's range is attributed to that environment alone and not to
// the one that lacks it.
func TestGitHubBundlesGitOpsDivergentEnvironments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/gitops/compare/base0001...intro002":
			// Introduction range: the shared lock-update PR.
			_, _ = w.Write([]byte(`{"total_commits":1,"commits":[
				{"commit":{"message":"Update release lock (#600)","author":{"date":"2026-09-05T00:00:00Z"}},"author":{"login":"promoter"}}
			]}`))
		case "/repos/acme/gitops/compare/base0001...stagehd":
			// Staging extends to a config PR prod has not received.
			_, _ = w.Write([]byte(`{"total_commits":2,"commits":[
				{"commit":{"message":"Update release lock (#600)","author":{"date":"2026-09-05T00:00:00Z"}},"author":{"login":"promoter"}},
				{"commit":{"message":"Staging-only tweak (#601)","author":{"date":"2026-09-07T00:00:00Z"}},"author":{"login":"ops"}}
			]}`))
		case "/repos/acme/gitops/compare/base0001...prodhd0":
			// Prod is still at the introduction commit's content.
			_, _ = w.Write([]byte(`{"total_commits":1,"commits":[
				{"commit":{"message":"Update release lock (#600)","author":{"date":"2026-09-05T00:00:00Z"}},"author":{"login":"promoter"}}
			]}`))
		default:
			t.Errorf("unexpected call to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := NewGitHubBundles(&config.Config{GitHubRepo: "acme/gitops", GitHubAPIBase: srv.URL}).
		WithLockHistory(fakeLockHistory{commits: []LockCommit{
			{SHA: "intro002", Digest: "dig-a"},
			{SHA: "base0001", Digest: "dig-z"},
		}})

	rel := &Release{
		Version: "v1",
		Digest:  "dig-a",
		Date:    "2026-09-05T00:00:00Z",
		DeploymentRevisions: []DeploymentRevision{
			{Env: "stage", DrySHA: "stagehd"},
			{Env: "prod", DrySHA: "prodhd0"},
		},
	}
	g.Enrich(context.Background(), map[string]*Release{"dig-a": rel})

	byNum := map[int]PR{}
	for _, p := range rel.GitOpsPRs {
		byNum[p.Number] = p
	}
	// The shared lock-update PR reached both environments.
	if got := byNum[600].Environments; len(got) != 2 || got[0] != "prod" || got[1] != "stage" {
		t.Errorf("PR #600 environments = %v, want [prod stage]", got)
	}
	// The staging-only PR is attributed to staging alone; prod never claims it.
	if got := byNum[601].Environments; len(got) != 1 || got[0] != "stage" {
		t.Errorf("PR #601 environments = %v, want [stage] only", got)
	}
}

// TestGitHubBundlesGitOpsUnavailableNoBaseline confirms that when a bundle is the
// oldest on the recorded timeline (no older baseline commit), the GitOps range is
// reported unavailable rather than invented (FLT-BPR-08.02), and no compare is made.
func TestGitHubBundlesGitOpsUnavailableNoBaseline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no HTTP call expected when no baseline exists, got %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := NewGitHubBundles(&config.Config{GitHubRepo: "acme/gitops", GitHubAPIBase: srv.URL}).
		WithLockHistory(fakeLockHistory{commits: []LockCommit{{SHA: "giti0001", Digest: "dig-a"}}})

	rel := &Release{Digest: "dig-a", Date: "2026-09-05T00:00:00Z"}
	releases := map[string]*Release{"dig-a": rel}
	g.Enrich(context.Background(), releases)

	if rel.GitOpsStatus != gitOpsUnavailable {
		t.Errorf("GitOpsStatus = %q, want %q", rel.GitOpsStatus, gitOpsUnavailable)
	}
	if rel.GitOpsPRs != nil {
		t.Errorf("no GitOps PRs expected when the range is unavailable, got %+v", rel.GitOpsPRs)
	}
}

// TestGitHubBundlesGitOpsRollback proves FLT-BPR-08.03: when a digest is introduced,
// replaced by another digest, and later rolled back to (reintroduced), the GitOps
// range anchors to the MOST RECENT lock commit that selected the digest (the
// reintroduction), not the first introduction. The range must therefore be bounded
// to the reintroduction window and must NOT span the whole excursion -- it must not
// surface the intervening digest's lock-update or configuration pull requests, and
// must not compare from the pre-first-introduction baseline. The fake errors on any
// compare anchored at the first-introduction baseline, so the old (oldest-occurrence)
// behavior would fail this test.
func TestGitHubBundlesGitOpsRollback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/gitops/compare/other004...reintro5":
			// The reintroduction (rollback) lock-update PR shared by the current deployment.
			_, _ = w.Write([]byte(`{"total_commits":1,"commits":[
				{"commit":{"message":"Roll back release lock (#700)","author":{"date":"2026-09-10T00:00:00Z"}},"author":{"login":"promoter"}}
			]}`))
		case "/repos/acme/gitops/compare/other004...cfgcur6":
			// Prod extends to a config-only PR landed after the reintroduction.
			_, _ = w.Write([]byte(`{"total_commits":2,"commits":[
				{"commit":{"message":"Roll back release lock (#700)","author":{"date":"2026-09-10T00:00:00Z"}},"author":{"login":"promoter"}},
				{"commit":{"message":"Tune after rollback (#701)","author":{"date":"2026-09-12T00:00:00Z"}},"author":{"login":"ops"}}
			]}`))
		default:
			// Any compare anchored at the first-introduction baseline (base0002...) or
			// spanning the intervening digest is the bug this test guards against.
			t.Errorf("unexpected compare %s (range must anchor to the reintroduction, not the first introduction)", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := NewGitHubBundles(&config.Config{GitHubRepo: "acme/gitops", GitHubAPIBase: srv.URL}).
		WithLockHistory(fakeLockHistory{commits: []LockCommit{
			{SHA: "reintro5", Digest: "dig-a"}, // most recent: rolled back to dig-a
			{SHA: "other004", Digest: "dig-b"}, // intervening digest
			{SHA: "intro003", Digest: "dig-a"}, // first introduction of dig-a
			{SHA: "base0002", Digest: "dig-z"},
		}})

	rel := &Release{
		Version: "v1",
		Digest:  "dig-a",
		Date:    "2026-09-12T00:00:00Z",
		DeploymentRevisions: []DeploymentRevision{
			{Env: "prod", DrySHA: "cfgcur6"},
		},
	}
	g.Enrich(context.Background(), map[string]*Release{"dig-a": rel})

	if rel.GitOpsStatus != gitOpsAvailable {
		t.Fatalf("GitOpsStatus = %q, want %q", rel.GitOpsStatus, gitOpsAvailable)
	}
	got := prNums(rel.GitOpsPRs)
	if len(got) != 2 || got[0] != 701 || got[1] != 700 {
		t.Fatalf("want bounded rollback-window PRs [701 700], got %v (an excursion-spanning range leaks intervening PRs)", got)
	}
}

// TestGitHubBundlesGitOpsPartialFailureUnavailable confirms a GitOps group is marked
// unavailable, not available, when one environment's compare fails even though the
// introduction compare succeeds (FLT-BPR-10). A partially-built group would omit the
// failed environment's configuration PRs and attribution, so reporting "available"
// would overstate completeness.
func TestGitHubBundlesGitOpsPartialFailureUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/gitops/compare/giti0001...giti0002":
			// The introduction compare succeeds.
			_, _ = w.Write([]byte(`{"total_commits":1,"commits":[
				{"commit":{"message":"Update release lock (#800)","author":{"date":"2026-09-05T00:00:00Z"}},"author":{"login":"promoter"}}
			]}`))
		case "/repos/acme/gitops/compare/giti0001...stagehd":
			_, _ = w.Write([]byte(`{"total_commits":1,"commits":[
				{"commit":{"message":"Update release lock (#800)","author":{"date":"2026-09-05T00:00:00Z"}},"author":{"login":"promoter"}}
			]}`))
		case "/repos/acme/gitops/compare/giti0001...prodhd0":
			// One environment's extension fails.
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Errorf("unexpected call to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := NewGitHubBundles(&config.Config{GitHubRepo: "acme/gitops", GitHubAPIBase: srv.URL}).
		WithLockHistory(fakeLockHistory{commits: []LockCommit{
			{SHA: "giti0002", Digest: "dig-a"},
			{SHA: "giti0001", Digest: "dig-z"},
		}})

	rel := &Release{
		Version: "v1",
		Digest:  "dig-a",
		Date:    "2026-09-05T00:00:00Z",
		DeploymentRevisions: []DeploymentRevision{
			{Env: "stage", DrySHA: "stagehd"},
			{Env: "prod", DrySHA: "prodhd0"},
		},
	}
	g.Enrich(context.Background(), map[string]*Release{"dig-a": rel})

	if rel.GitOpsStatus != gitOpsUnavailable {
		t.Errorf("GitOpsStatus = %q, want %q (a partial failure must not report available)", rel.GitOpsStatus, gitOpsUnavailable)
	}
	if rel.GitOpsPRs != nil {
		t.Errorf("no GitOps PRs expected when the group is unavailable, got %+v", rel.GitOpsPRs)
	}
}

// TestGitHubBundlesGitOpsFailureIndependent confirms a failed GitOps compare marks
// only the GitOps group unavailable and never clears the application PRs
// (FLT-BPR-10.01).
func TestGitHubBundlesGitOpsFailureIndependent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/hypershell/compare/oldrev0001...newrev0002":
			_, _ = w.Write([]byte(`{"total_commits":1,"commits":[
				{"commit":{"message":"App change (#101)","author":{"date":"2026-09-05T00:00:00Z"}},"author":{"login":"alice"}}
			]}`))
		case "/repos/acme/gitops/compare/giti0001...giti0002":
			// GitOps compare fails.
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := NewGitHubBundles(&config.Config{
		GitHubRepo:    "acme/gitops",
		UpstreamRepo:  "acme/hypershell",
		GitHubAPIBase: srv.URL,
	}).WithLockHistory(fakeLockHistory{commits: []LockCommit{
		{SHA: "giti0002", Digest: "dig-a"},
		{SHA: "giti0001", Digest: "dig-z"},
	}})

	older := &Release{Version: "v0", Digest: "dig-z", Date: "2026-09-01T00:00:00Z", ManifestsRev: "oldrev0001"}
	newer := &Release{Version: "v1", Digest: "dig-a", Date: "2026-09-10T00:00:00Z", ManifestsRev: "newrev0002"}
	releases := map[string]*Release{"dig-z": older, "dig-a": newer}
	g.Enrich(context.Background(), releases)

	if got := prNums(newer.PRs); len(got) != 1 || got[0] != 101 {
		t.Errorf("application PRs should survive a GitOps failure, got %v", got)
	}
	if newer.GitOpsStatus != gitOpsUnavailable {
		t.Errorf("GitOpsStatus = %q, want %q", newer.GitOpsStatus, gitOpsUnavailable)
	}
}

// TestGitHubBundlesNoUpstreamRev confirms a bundle without upstream provenance
// (ManifestsRev empty -- a short-SHA fallback or a lock predating bundle
// provenance) is skipped: it is never diffed, so no compare call is made and it
// carries no PRs. Both releases are dated, so only the missing revision prevents
// the diff.
func TestGitHubBundlesNoUpstreamRev(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no HTTP call expected (dates preset, one release lacks ManifestsRev), got %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := NewGitHubBundles(&config.Config{GitHubRepo: "acme/gitops", UpstreamRepo: "acme/hypershell", GitHubAPIBase: srv.URL})
	releases := map[string]*Release{
		"a": {SHA: "a", Date: "2026-09-01T00:00:00Z", ManifestsRev: "rev-a"},
		"b": {SHA: "b", Date: "2026-09-02T00:00:00Z"}, // no ManifestsRev
	}
	g.Enrich(context.Background(), releases)
	if releases["a"].PRs != nil || releases["b"].PRs != nil {
		t.Errorf("no PRs expected when a bundle lacks upstream provenance: a=%+v b=%+v", releases["a"].PRs, releases["b"].PRs)
	}
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
