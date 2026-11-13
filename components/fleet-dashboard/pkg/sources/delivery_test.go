package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/cache"
	"github.com/openshift-online/hypershell/components/fleet-dashboard/pkg/config"
)

// --- Fakes -------------------------------------------------------------------

// linearRepo is a squash-merged application history: sha "c<n>", subject
// "change <n> (#<n>)", oldest first.
type linearRepo struct{ n int }

func (r linearRepo) sha(i int) string { return fmt.Sprintf("c%02d", i) }

// newestFirst returns the commits newest first, as the commits API does.
func (r linearRepo) newestFirst(base time.Time) []UpstreamCommit {
	out := make([]UpstreamCommit, 0, r.n)
	for i := r.n; i >= 1; i-- {
		out = append(out, UpstreamCommit{
			SHA:      r.sha(i),
			Message:  fmt.Sprintf("change %d (#%d)", i, i),
			MergedAt: base.Add(time.Duration(i) * time.Minute).UTC().Format(time.RFC3339),
		})
	}
	return out
}

// compareServer serves the GitHub compare API over a linearRepo and counts calls.
func compareServer(t *testing.T, repo linearRepo, calls *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, rng, ok := strings.Cut(r.URL.Path, "/compare/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(calls, 1)
		base, head, _ := strings.Cut(rng, "...")
		var from, to int
		_, _ = fmt.Sscanf(base, "c%d", &from)
		_, _ = fmt.Sscanf(head, "c%d", &to)
		var commits []map[string]any
		for i := from + 1; i <= to; i++ {
			commits = append(commits, map[string]any{"commit": map[string]any{
				"message": fmt.Sprintf("change %d (#%d)", i, i),
				"author":  map[string]any{"date": fmt.Sprintf("2026-11-01T00:%02d:00Z", i)},
			}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total_commits": len(commits), "commits": commits})
	}))
	t.Cleanup(srv.Close)
	return srv
}

type fakeDeliveryGitHub struct {
	commits    []UpstreamCommit
	commitsErr error
	checks     map[string][]CheckRun
	pr         *BundlePR
	checkCalls map[string]int
	calls      struct{ commits, prs, releases int }
	// envPromos are each environment branch's promotions, newest first.
	envPromos map[string][]EnvPromotion
	gateRuns  map[string][]CheckRun
	histMu    sync.Mutex // environments are read in parallel
	envCalls  map[string]int
	gateCalls map[string]int
}

func (f *fakeDeliveryGitHub) EnvPromotions(_ context.Context, branch string, _ time.Time) ([]EnvPromotion, error) {
	f.histMu.Lock()
	defer f.histMu.Unlock()
	if f.envCalls == nil {
		f.envCalls = map[string]int{}
	}
	f.envCalls[branch]++
	return f.envPromos[branch], nil
}

func (f *fakeDeliveryGitHub) GateRuns(_ context.Context, sha string) ([]CheckRun, error) {
	f.histMu.Lock()
	defer f.histMu.Unlock()
	if f.gateCalls == nil {
		f.gateCalls = map[string]int{}
	}
	f.gateCalls[sha]++
	return f.gateRuns[sha], nil
}

func (f *fakeDeliveryGitHub) Commits(context.Context, int) ([]UpstreamCommit, error) {
	f.calls.commits++
	return f.commits, f.commitsErr
}

func (f *fakeDeliveryGitHub) CheckRuns(_ context.Context, sha string) ([]CheckRun, error) {
	if f.checkCalls == nil {
		f.checkCalls = map[string]int{}
	}
	f.checkCalls[sha]++
	return f.checks[sha], nil
}

func (f *fakeDeliveryGitHub) OpenBundlePR(context.Context) (*BundlePR, error) {
	f.calls.prs++
	return f.pr, nil
}

func (f *fakeDeliveryGitHub) ReleasePublished(context.Context, string) (bool, string, error) {
	f.calls.releases++
	return false, "", nil
}

type fakeRegistry struct {
	tags     []RegistryBundle
	err      error
	revs     map[string]string
	revCalls int
	lists    int
}

func (f *fakeRegistry) ListBundles(context.Context) ([]RegistryBundle, error) {
	f.lists++
	return f.tags, f.err
}

func (f *fakeRegistry) ManifestsRevision(_ context.Context, digest string) (string, error) {
	f.revCalls++
	return f.revs[digest], nil
}

// fakeLock is the shared lock timeline: lock commits newest first, each resolved
// to a release.
type fakeLock struct {
	timeline []LockCommit
	releases map[string]*Release // lock commit sha -> release
	ok       bool
	hydrated map[string]*Release // environment-branch commit -> release
}

func (f *fakeLock) ResolveHydrated(_ context.Context, sha string) *Release {
	return copyRelease(f.hydrated[sha])
}

func (f *fakeLock) LastLockTimeline() ([]LockCommit, time.Time, bool) {
	return f.timeline, time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC), f.ok
}

func (f *fakeLock) ResolveCached(sha string) (*Release, bool) {
	r, ok := f.releases[sha]
	return copyRelease(r), ok
}

func (f *fakeLock) Resolve(_ context.Context, sha string) *Release {
	return copyRelease(f.releases[sha])
}

// bundleTag builds a release-bundle tag for minute m on 2026-11-01.
func bundleTag(m int, id string) string {
	return fmt.Sprintf("release-bundle-20261101T10%02d00000000Z-%s", m, id)
}

func lockedRelease(tag, digest, rev string) *Release {
	return &Release{Version: versionFromTag(tag), Tag: tag, Digest: digest, Date: dateFromTag(tag), ManifestsRev: rev}
}

// fixture is a delivery source over fakes with three bundles: B1 (rev c02) and
// B2 (rev c05) locked, B3 (rev c08) pending in the registry. The application
// history has ten changes.
type fixture struct {
	repo     linearRepo
	gh       *fakeDeliveryGitHub
	reg      *fakeRegistry
	lock     *fakeLock
	bundles  *GitHubBundles
	compares *int32
	promo    *PromotionPayload
	d        *Delivery
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	repo := linearRepo{n: 10}
	var compares int32
	srv := compareServer(t, repo, &compares)
	cfg := &config.Config{GitHubRepo: "ops/gitops", GitHubAPIBase: srv.URL, UpstreamRepo: "app/repo", DeliveryWindow: 30, DeliveryCheckApp: "konflux"}
	t1, t2, t3 := bundleTag(1, "b1"), bundleTag(2, "b2"), bundleTag(3, "b3")
	lock := &fakeLock{
		ok: true,
		timeline: []LockCommit{
			{SHA: "lock2", Digest: "sha256:b2", Date: "2026-11-01T11:00:00Z", Message: "Update bundle (#42)"},
			{SHA: "lock1", Digest: "sha256:b1", Date: "2026-11-01T10:30:00Z", Message: "Update bundle (#41)"},
		},
		releases: map[string]*Release{
			"lock1": lockedRelease(t1, "sha256:b1", repo.sha(2)),
			"lock2": lockedRelease(t2, "sha256:b2", repo.sha(5)),
		},
	}
	reg := &fakeRegistry{
		tags: []RegistryBundle{
			{Tag: t3, Digest: "sha256:b3", PublishedAt: time.Date(2026, 11, 1, 10, 45, 0, 0, time.UTC)},
			{Tag: t2, Digest: "sha256:b2"},
			{Tag: t1, Digest: "sha256:b1"},
		},
		revs: map[string]string{"sha256:b3": repo.sha(8)},
	}
	base := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	gh := &fakeDeliveryGitHub{commits: repo.newestFirst(base), checks: map[string][]CheckRun{}}
	promo := &PromotionPayload{
		Order: []string{"int-a", "stage-a", "prod-a", "prod-b"},
		Environments: map[string]Environment{
			"int-a":   {Env: "int", Active: &Release{Digest: "sha256:b2"}, Proposed: &Release{Digest: "sha256:b2"}},
			"stage-a": {Env: "stage", Active: &Release{Digest: "sha256:b1"}, Proposed: &Release{Digest: "sha256:b2"}},
			"prod-a":  {Env: "prod", Active: &Release{Digest: "sha256:b1"}, Proposed: &Release{Digest: "sha256:b2"}},
			"prod-b":  {Env: "prod", Active: &Release{Digest: "sha256:b1"}, Proposed: &Release{Digest: "sha256:b2"}},
		},
	}
	f := &fixture{repo: repo, gh: gh, reg: reg, lock: lock, compares: &compares, promo: promo}
	f.bundles = NewGitHubBundles(cfg)
	f.d = newDelivery(cfg, gh, reg, lock, f.bundles, f)
	f.d.now = func() time.Time { return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) }
	return f
}

// Get makes the fixture its own promotion snapshot reader, so tests can edit
// f.promo between refreshes.
func (f *fixture) Get() cache.Snapshot {
	if f.promo == nil {
		return cache.Snapshot{Stale: true}
	}
	return cache.Snapshot{Data: *f.promo, GeneratedAt: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
}

func (f *fixture) refresh(t *testing.T) DeliveryPayload {
	t.Helper()
	got, err := f.d.Delivery(context.Background())
	if err != nil {
		t.Fatalf("Delivery: %v", err)
	}
	return got.(DeliveryPayload)
}

func change(t *testing.T, p DeliveryPayload, sha string) DeliveryChange {
	t.Helper()
	for _, c := range p.Changes {
		if c.SHA == sha {
			return c
		}
	}
	t.Fatalf("change %s not listed", sha)
	return DeliveryChange{}
}

func stage(t *testing.T, c DeliveryChange, key string) DeliveryStage {
	t.Helper()
	for _, s := range c.Stages {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("change %s has no stage %s", c.SHA, key)
	return DeliveryStage{}
}

func konflux(name, status, conclusion string) CheckRun {
	return CheckRun{Name: "Konflux / " + name, App: "konflux", Status: status, Conclusion: conclusion,
		StartedAt: "2026-11-01T11:50:00Z", CompletedAt: map[bool]string{true: "2026-11-01T11:58:00Z"}[status == "completed"]}
}

// --- Tests -------------------------------------------------------------------

func TestDeliveryCheckRunClassification(t *testing.T) {
	// Verifies: FLT-DLV-05.01, FLT-DLV-05.02, FLT-DLV-05.03
	f := newFixture(t)
	f.gh.checks[f.repo.sha(10)] = []CheckRun{
		konflux("demo-control-plane-main-on-push", "in_progress", ""),
		konflux("demo-api-server-main-on-merge-queue", "completed", "success"),  // not a push build
		{Name: "other / x-on-push", App: "someone-else", Status: "in_progress"}, // other app
	}
	f.gh.checks[f.repo.sha(9)] = []CheckRun{
		konflux("demo-web-console-main-on-push", "completed", "failure"),
	}
	// sha 8 is in pending bundle B3; give it a passed build and policy.
	f.gh.checks[f.repo.sha(8)] = []CheckRun{
		konflux("demo-cli-main-on-push", "completed", "success"),
		konflux("demo-main-enterprise-contract / demo-cli-main", "completed", "neutral"),
		konflux("demo-main-enterprise-contract / pr group", "queued", ""), // not a built component
	}
	p := f.refresh(t)

	building := change(t, p, f.repo.sha(10))
	if s := stage(t, building, stageBuild); s.State != StageRunning || strings.Join(s.Components, ",") != "control-plane" {
		t.Errorf("building change: build = %+v, want running [control-plane]", s)
	}

	failed := change(t, p, f.repo.sha(9))
	if s := stage(t, failed, stageBuild); s.State != StageFailed || strings.Join(s.Components, ",") != "web-console" {
		t.Errorf("failed change: build = %+v, want failed [web-console]", s)
	}
	for _, key := range []string{stagePolicy, stageRelease} {
		if s := stage(t, failed, key); s.State != StageNotStarted {
			t.Errorf("failed change: %s = %s, want not-started", key, s.State)
		}
	}

	built := change(t, p, f.repo.sha(8))
	if s := stage(t, built, stagePolicy); s.State != StagePassed || strings.Join(s.Components, ",") != "cli" {
		t.Errorf("neutral policy check: policy = %+v, want passed [cli]", s)
	}

	docs := change(t, p, f.repo.sha(7)) // no check runs at all
	for _, key := range []string{stageBuild, stagePolicy} {
		if s := stage(t, docs, key); s.State != StageNotNeeded || s.Reason != reasonNoComponentChanged {
			t.Errorf("docs change: %s = %+v, want not-needed/no-component-changed", key, s)
		}
	}
	// A docs-only change rides the bundle whose window holds it (B3).
	if docs.Bundle == nil || docs.Bundle.Digest != "sha256:b3" {
		t.Errorf("docs change bundle = %+v, want B3", docs.Bundle)
	}
}

func TestDeliveryCancelledBuildIsReplacedNotFailed(t *testing.T) {
	f := newFixture(t)
	f.gh.checks[f.repo.sha(10)] = []CheckRun{konflux("demo-cli-main-on-push", "completed", "cancelled")}
	p := f.refresh(t)
	if s := stage(t, change(t, p, f.repo.sha(10)), stageBuild); s.State != StageNotNeeded || s.Reason != reasonBuildReplaced {
		t.Errorf("cancelled build = %+v, want not-needed/build-replaced", s)
	}
}

func TestDeliveryFirstBundleAndRidingChanges(t *testing.T) {
	// Verifies: FLT-DLV-06.02
	f := newFixture(t)
	p := f.refresh(t)
	cases := map[int]string{3: "sha256:b2", 5: "sha256:b2", 6: "sha256:b3", 8: "sha256:b3"}
	for i, want := range cases {
		c := change(t, p, f.repo.sha(i))
		if c.Bundle == nil || c.Bundle.Digest != want {
			t.Errorf("change %d first bundle = %+v, want %s", i, c.Bundle, want)
		}
	}
	if c := change(t, p, f.repo.sha(3)); !c.Bundle.Locked {
		t.Errorf("B2 should be reported locked")
	}
	if c := change(t, p, f.repo.sha(6)); c.Bundle.Locked {
		t.Errorf("B3 should be reported pending")
	}
	// Changes after the newest bundle are not placed.
	if c := change(t, p, f.repo.sha(9)); c.Bundle != nil {
		t.Errorf("change 9 has bundle %+v, want none", c.Bundle)
	}
}

func TestDeliveryPendingPlacementMatchesSharedSourceOnceLocked(t *testing.T) {
	// Verifies: FLT-DLV-06.03, FLT-DLV-06.04
	f := newFixture(t)
	before := f.refresh(t)
	comparesBefore := atomic.LoadInt32(f.compares)

	// B3 enters the release lock.
	f.lock.timeline = append([]LockCommit{{SHA: "lock3", Digest: "sha256:b3", Date: "2026-11-01T11:30:00Z", Message: "Update bundle (#43)"}}, f.lock.timeline...)
	f.lock.releases["lock3"] = lockedRelease(bundleTag(3, "b3"), "sha256:b3", f.repo.sha(8))
	after := f.refresh(t)

	for _, i := range []int{6, 7, 8} {
		b, a := change(t, before, f.repo.sha(i)), change(t, after, f.repo.sha(i))
		if b.Bundle == nil || a.Bundle == nil || b.Bundle.Digest != a.Bundle.Digest {
			t.Errorf("change %d: pending bundle %+v, locked bundle %+v; want the same bundle", i, b.Bundle, a.Bundle)
		}
		if !a.Bundle.Locked {
			t.Errorf("change %d: bundle not reported locked after locking", i)
		}
	}
	// The same consecutive pairs come from the shared compare cache: no new call.
	if got := atomic.LoadInt32(f.compares); got != comparesBefore {
		t.Errorf("compare calls after locking = %d, want %d (shared cache)", got, comparesBefore)
	}
	// The same membership the promotion plane computes for these bundles.
	shared := map[string][]int{}
	for _, w := range f.bundles.BundleWindows(context.Background(), []*Release{
		lockedRelease(bundleTag(1, "b1"), "sha256:b1", f.repo.sha(2)),
		lockedRelease(bundleTag(2, "b2"), "sha256:b2", f.repo.sha(5)),
		lockedRelease(bundleTag(3, "b3"), "sha256:b3", f.repo.sha(8)),
	}) {
		for _, pr := range w.PRs {
			shared[w.Release.Digest] = append(shared[w.Release.Digest], pr.Number)
		}
	}
	for digest, prs := range shared {
		for _, n := range prs {
			if c := change(t, after, f.repo.sha(n)); c.Bundle == nil || c.Bundle.Digest != digest {
				t.Errorf("PR #%d: delivery bundle %+v, shared source bundle %s", n, c.Bundle, digest)
			}
		}
	}
}

func TestDeliveryReleaseInferredAfterPolicy(t *testing.T) {
	// Verifies: FLT-DLV-06.01
	f := newFixture(t)
	f.gh.checks[f.repo.sha(10)] = []CheckRun{
		konflux("demo-cli-main-on-push", "completed", "success"),
		konflux("demo-main-enterprise-contract / demo-cli-main", "completed", "neutral"),
	}
	p := f.refresh(t)
	s := stage(t, change(t, p, f.repo.sha(10)), stageRelease)
	if s.State != StageRunning || s.Reason != reasonInferredRelease || s.StartedAt != "2026-11-01T11:58:00Z" {
		t.Errorf("release = %+v, want running/inferred-release from the policy check end", s)
	}
}

func TestDeliveryGitOpsStage(t *testing.T) {
	// Verifies: FLT-DLV-07.01
	f := newFixture(t)
	p := f.refresh(t)
	if s := stage(t, change(t, p, f.repo.sha(4)), stageGitOps); s.State != StagePassed || s.PR != 42 {
		t.Errorf("locked change gitops = %+v, want passed with PR 42", s)
	}
	if s := stage(t, change(t, p, f.repo.sha(7)), stageGitOps); s.State != StageWaiting || s.Reason != reasonWaitingBundlePR {
		t.Errorf("pending change gitops = %+v, want waiting for the bundle PR", s)
	}

	f.gh.pr = &BundlePR{Number: 77, URL: "https://example.test/pr/77", HeadSHA: "prhead", Checks: "running"}
	f.lock.releases["prhead"] = lockedRelease(bundleTag(3, "b3"), "sha256:b3", f.repo.sha(8))
	p = f.refresh(t)
	s := stage(t, change(t, p, f.repo.sha(7)), stageGitOps)
	if s.State != StageRunning || s.PR != 77 || s.Checks != "running" {
		t.Errorf("gitops with open bundle PR = %+v, want running PR 77 checks running", s)
	}
}

func TestDeliveryEnvironmentGroups(t *testing.T) {
	// Verifies: FLT-DLV-03.01, FLT-DLV-08.01, FLT-DLV-08.02
	f := newFixture(t)
	f.promo.Environments["int-a"] = Environment{Env: "int", Active: &Release{Digest: "sha256:b2"}, Proposed: &Release{Digest: "sha256:b2"},
		ActiveGates: []Gate{{Key: "hypershell-analysis", Phase: "pending"}}}
	f.promo.Environments["prod-a"] = Environment{Env: "prod", Active: &Release{Digest: "sha256:b2"}, Proposed: &Release{Digest: "sha256:b2"}}
	p := f.refresh(t)

	var groups []string
	for _, s := range p.Stages {
		if s.Kind == "group" {
			groups = append(groups, s.Group)
		}
	}
	if strings.Join(groups, ",") != "int,stage,prod" {
		t.Errorf("groups = %v, want int,stage,prod (promotion order)", groups)
	}

	c := change(t, p, f.repo.sha(4)) // in B2
	if s := stage(t, c, "env:int"); s.State != StageRunning || s.Reason != reasonGateRunning || s.Gate != "hypershell-analysis" {
		t.Errorf("int = %+v, want running with the analysis gate", s)
	}
	if s := stage(t, c, "env:stage"); s.State != StageWaiting || s.Reason != reasonWaitingGroups || strings.Join(s.Groups, ",") != "int" {
		t.Errorf("stage = %+v, want waiting for int", s)
	}
	if s := stage(t, c, "env:prod"); s.State != StageRunning || s.Reason != reasonPartial || s.Done != 1 || s.Total != 2 {
		t.Errorf("prod = %+v, want running 1 of 2", s)
	}

	// A change on GitOps main that no proposed release holds waits for hydration.
	f.promo.Environments["stage-a"] = Environment{Env: "stage", Active: &Release{Digest: "sha256:b1"}, Proposed: &Release{Digest: "sha256:b1"}}
	f.lock.timeline = append([]LockCommit{{SHA: "lock3", Digest: "sha256:b3", Date: "2026-11-01T11:30:00Z"}}, f.lock.timeline...)
	f.lock.releases["lock3"] = lockedRelease(bundleTag(3, "b3"), "sha256:b3", f.repo.sha(8))
	p = f.refresh(t)
	if s := stage(t, change(t, p, f.repo.sha(7)), "env:stage"); s.State != StageWaiting || s.Reason != reasonWaitingHydration {
		t.Errorf("unhydrated stage = %+v, want waiting for hydration", s)
	}
}

func TestDeliveryFailedGateFailsGroup(t *testing.T) {
	f := newFixture(t)
	f.promo.Environments["int-a"] = Environment{Env: "int", Active: &Release{Digest: "sha256:b2"}, Proposed: &Release{Digest: "sha256:b2"},
		ActiveGates: []Gate{{Key: "argocd-health", Phase: "failure"}}}
	p := f.refresh(t)
	if s := stage(t, change(t, p, f.repo.sha(4)), "env:int"); s.State != StageFailed || s.Gate != "argocd-health" {
		t.Errorf("int = %+v, want failed naming argocd-health", s)
	}
}

func TestDeliveryWindow(t *testing.T) {
	// Verifies: FLT-DLV-02.01, FLT-DLV-02.02
	f := newFixture(t)
	// Every environment on B2: changes 1..5 are delivered, 6..10 are not.
	for k, e := range f.promo.Environments {
		e.Active = &Release{Digest: "sha256:b2"}
		f.promo.Environments[k] = e
	}
	f.d.window = 7
	p := f.refresh(t)
	var got []string
	for _, c := range p.Changes {
		got = append(got, c.SHA)
	}
	want := "c10,c09,c08,c07,c06,c05,c04"
	if strings.Join(got, ",") != want || p.Omitted != 0 {
		t.Errorf("window = %v omitted %d, want %s omitted 0", got, p.Omitted, want)
	}
	if !change(t, p, "c05").Delivered || change(t, p, "c06").Delivered {
		t.Errorf("delivered flags wrong")
	}

	f.d.window = 3
	p = f.refresh(t)
	got = got[:0]
	for _, c := range p.Changes {
		got = append(got, c.SHA)
	}
	if strings.Join(got, ",") != "c10,c09,c08" || p.Omitted != 2 {
		t.Errorf("overflow window = %v omitted %d, want c10,c09,c08 omitted 2", got, p.Omitted)
	}
}

func TestDeliveryTypicalDurations(t *testing.T) {
	// Verifies: FLT-DLV-09.01
	mk := func(start, end string) DeliveryChange {
		return DeliveryChange{Stages: []DeliveryStage{
			{Key: stageBuild, State: StagePassed, StartedAt: start, FinishedAt: end},
			{Key: stagePolicy, State: StagePassed, StartedAt: end, FinishedAt: end[:15] + "9:00Z"},
		}}
	}
	changes := []DeliveryChange{
		mk("2026-11-01T10:00:00Z", "2026-11-01T10:10:00Z"),
		mk("2026-11-01T11:00:00Z", "2026-11-01T11:20:00Z"),
		mk("2026-11-01T12:00:00Z", "2026-11-01T12:12:00Z"),
	}
	got := typicalDurations(changes)
	if got[stageBuild] != 12*60 {
		t.Errorf("typical build = %d, want 720 (median of 10, 20, 12 min)", got[stageBuild])
	}
	if _, ok := typicalDurations(changes[:2])[stageBuild]; ok {
		t.Errorf("two samples produced a typical duration")
	}
}

func TestDeliveryCallBudget(t *testing.T) {
	// Verifies: FLT-DLV-12.01
	f := newFixture(t)
	f.gh.pr = &BundlePR{Number: 77, HeadSHA: "prhead"}
	f.refresh(t)
	f.refresh(t)
	if f.gh.calls.commits != 2 || f.gh.calls.prs != 2 || f.reg.lists != 2 {
		t.Errorf("per-refresh calls: commits %d prs %d tag lists %d, want 2 each over two refreshes",
			f.gh.calls.commits, f.gh.calls.prs, f.reg.lists)
	}
	if f.reg.revCalls != 1 {
		t.Errorf("bundle.json reads = %d, want 1 (one new pending bundle, then cached)", f.reg.revCalls)
	}
	// Delivered changes whose runs were read once are not read again.
	for k, e := range f.promo.Environments {
		e.Active = &Release{Digest: "sha256:b2"}
		f.promo.Environments[k] = e
	}
	f.refresh(t)
	before := f.gh.checkCalls["c03"]
	f.refresh(t)
	if f.gh.checkCalls["c03"] != before {
		t.Errorf("delivered change c03 check runs re-read")
	}
}

func TestDeliveryStopsReadingSettledCheckRuns(t *testing.T) {
	// Verifies: FLT-DLV-12.01
	f := newFixture(t)
	sha := f.repo.sha(10)
	f.gh.checks[sha] = []CheckRun{
		konflux("demo-cli-main-on-push", "completed", "success"),
		konflux("demo-main-enterprise-contract / demo-cli-main", "completed", "neutral"),
		// Runs the stages ignore must not keep the commit unsettled: a policy
		// check for a component that was not built, a group check that never
		// finishes, and another app's failed job.
		konflux("demo-main-enterprise-contract / demo-api-main", "queued", ""),
		konflux("demo-main-enterprise-contract / pr group", "queued", ""),
		{Name: "E2E / Kind", App: "github-actions", Status: "completed", Conclusion: "failure"},
	}
	f.refresh(t)
	before := f.gh.checkCalls[sha]
	f.refresh(t)
	if f.gh.checkCalls[sha] != before {
		t.Errorf("settled check runs were read again")
	}
}

func TestDeliveryRereadsFailedCheckRuns(t *testing.T) {
	// Verifies: FLT-DLV-12.02
	// A Konflux check run can report a failure and later finish as a success
	// under the same id. A failure is never treated as final.
	f := newFixture(t)
	sha, old := f.repo.sha(10), f.repo.sha(3)
	f.gh.checks[sha] = []CheckRun{konflux("demo-cli-main-on-push", "completed", "failure")}
	f.gh.checks[old] = []CheckRun{konflux("demo-cli-main-on-push", "completed", "failure")}
	p := f.refresh(t)
	if s := stage(t, change(t, p, sha), stageBuild); s.State != StageFailed {
		t.Fatalf("build = %s, want failed", s.State)
	}
	f.gh.checks[sha] = []CheckRun{
		konflux("demo-cli-main-on-push", "completed", "success"),
		konflux("demo-main-enterprise-contract / demo-cli-main", "completed", "neutral"),
	}
	p = f.refresh(t)
	if s := stage(t, change(t, p, sha), stageBuild); s.State != StagePassed {
		t.Errorf("build after the run finished = %s, want passed", s.State)
	}

	// The same holds for a delivered change, which is otherwise read once.
	for k, e := range f.promo.Environments {
		e.Active = &Release{Digest: "sha256:b2"}
		f.promo.Environments[k] = e
	}
	p = f.refresh(t)
	if !change(t, p, old).Delivered {
		t.Fatalf("change %s should be delivered", old)
	}
	f.gh.checks[old] = []CheckRun{konflux("demo-cli-main-on-push", "completed", "success")}
	p = f.refresh(t)
	if s := stage(t, change(t, p, old), stageBuild); s.State != StagePassed {
		t.Errorf("delivered change kept a failure Konflux later replaced: build = %s", s.State)
	}
}

func TestDeliveryWaitsForBuildsOnNewChange(t *testing.T) {
	// A change with no build yet keeps being read until a build would have
	// started; after that it is a change with no component build.
	f := newFixture(t)
	sha := f.repo.sha(10)
	merged := time.Date(2026, 11, 1, 9, 10, 0, 0, time.UTC)
	f.d.now = func() time.Time { return merged.Add(10 * time.Minute) }
	f.refresh(t)
	before := f.gh.checkCalls[sha]
	f.refresh(t)
	if f.gh.checkCalls[sha] == before {
		t.Errorf("a minutes-old change with no build was not read again")
	}
	f.d.now = func() time.Time { return merged.Add(2 * time.Hour) }
	f.refresh(t)
	before = f.gh.checkCalls[sha]
	f.refresh(t)
	if f.gh.checkCalls[sha] != before {
		t.Errorf("an hours-old change with no build was read again")
	}
}

func TestDeliveryDegradation(t *testing.T) {
	// Verifies: FLT-DLV-13.01, FLT-DLV-13.02
	f := newFixture(t)
	f.reg.err = errors.New("registry down")
	f.gh.checks[f.repo.sha(10)] = []CheckRun{
		konflux("demo-cli-main-on-push", "completed", "success"),
		konflux("demo-main-enterprise-contract / demo-cli-main", "completed", "neutral"),
	}
	p := f.refresh(t)
	if st := p.Sources[srcRegistry]; !st.Never || !st.Stale {
		t.Errorf("registry status = %+v, want never/stale", st)
	}
	// Locked placement still works; a change only a pending bundle could hold reads
	// Unknown once its policy check passed.
	if c := change(t, p, f.repo.sha(4)); c.Bundle == nil || stage(t, c, stageRelease).State != StagePassed {
		t.Errorf("locked change lost its release without the registry")
	}
	if s := stage(t, change(t, p, f.repo.sha(10)), stageRelease); s.State != StageUnknown {
		t.Errorf("release without registry = %s, want unknown", s.State)
	}
	if s := stage(t, change(t, p, f.repo.sha(10)), stageBuild); s.State != StagePassed {
		t.Errorf("build should still render from its own source, got %s", s.State)
	}

	// Application repository fails after a success: the last list is served stale.
	f.gh.commitsErr = errors.New("rate limited")
	p = f.refresh(t)
	if st := p.Sources[srcUpstream]; !st.Stale || st.Never || st.LastSuccess == "" {
		t.Errorf("upstream status = %+v, want stale with a last success", st)
	}
	if len(p.Changes) == 0 {
		t.Errorf("stale upstream dropped the change list")
	}

	// Never succeeded: no changes, flagged never.
	g := newFixture(t)
	g.gh.commitsErr = errors.New("down")
	g.gh.commits = nil
	p = g.refresh(t)
	if st := p.Sources[srcUpstream]; !st.Never || len(p.Changes) != 0 {
		t.Errorf("upstream never: status %+v changes %d, want never and none", st, len(p.Changes))
	}

	// GitOps never read: no change can be placed in a bundle, so every
	// environment reads Unknown, never Waiting.
	k := newFixture(t)
	k.lock.ok = false
	k.lock.timeline = nil
	p = k.refresh(t)
	for _, c := range p.Changes {
		for _, e := range c.Envs {
			if e.State != StageUnknown {
				t.Errorf("change %s env %s = %s without GitOps, want unknown", c.ShortSHA, e.Env, e.State)
			}
		}
	}

	// Promotion never succeeded: group stages are Unknown.
	h := newFixture(t)
	h.promo = nil
	p = h.refresh(t)
	if !p.Sources[srcPromotion].Never {
		t.Errorf("promotion status = %+v, want never", p.Sources[srcPromotion])
	}
	// Without promotion data the groups cannot be derived, and nothing is known
	// delivered.
	for _, s := range p.Stages {
		if s.Kind == "group" {
			t.Errorf("group column %s derived without promotion data", s.Key)
		}
	}
	for _, c := range p.Changes {
		if c.Delivered {
			t.Errorf("change %s delivered without promotion data", c.SHA)
		}
	}
}

func TestShortComponent(t *testing.T) {
	if got := shortComponent("demo-control-plane-main", "demo-main"); got != "control-plane" {
		t.Errorf("shortComponent = %q", got)
	}
	if got := shortComponent("other-thing", "demo-main"); got != "other-thing" {
		t.Errorf("unmatched component should be unchanged, got %q", got)
	}
	if got := shortComponent("demo-x-main", ""); got != "demo-x-main" {
		t.Errorf("unknown application should leave the name, got %q", got)
	}
}

// withIntHistory gives the int-a environment a branch with the given
// promotions (newest first), the last of which is active.
func withIntHistory(f *fixture, promos []EnvPromotion, digests map[string]string, gates []Gate) {
	f.lock.hydrated = map[string]*Release{}
	for sha, dg := range digests {
		f.lock.hydrated[sha] = &Release{Digest: dg}
	}
	f.gh.envPromos = map[string][]EnvPromotion{"env/int-a": promos}
	env := f.promo.Environments["int-a"]
	env.Branch, env.ActiveHydrated, env.ActiveGates = "env/int-a", promos[0].SHA, gates
	env.Active = &Release{Digest: digests[promos[0].SHA]}
	f.promo.Environments["int-a"] = env
}

func envOf(t *testing.T, c DeliveryChange, key string) DeliveryEnvState {
	t.Helper()
	for _, e := range c.Envs {
		if e.Env == key {
			return e
		}
	}
	t.Fatalf("change %s has no environment %s", c.ShortSHA, key)
	return DeliveryEnvState{}
}

func outcomes(h []DeliveryPromotion) []string {
	out := make([]string, 0, len(h))
	for _, a := range h {
		out = append(out, a.Version+":"+a.Outcome)
	}
	return out
}

func TestPromotionHistoryReplacedWhileRunning(t *testing.T) {
	// Verifies: FLT-DLV-18.01
	f := newFixture(t)
	analysis := "hypershell-analysis"
	withIntHistory(f, []EnvPromotion{
		{SHA: "m2", MergedAt: "2026-11-01T10:50:00Z", PR: 2},
		{SHA: "m1", MergedAt: "2026-11-01T10:40:00Z", PR: 1},
	}, map[string]string{"m1": "sha256:b1", "m2": "sha256:b2"}, []Gate{{Key: analysis, Phase: "success"}})
	f.gh.gateRuns = map[string][]CheckRun{"m1": {{Name: analysis, Status: "in_progress"}}}

	p := f.refresh(t)
	c := change(t, p, f.repo.sha(2)) // first bundle b1
	e := envOf(t, c, "int-a")
	if got := outcomes(e.History); len(got) != 2 || got[0] != versionFromTag(bundleTag(1, "b1"))+":replaced" || got[1] != versionFromTag(bundleTag(2, "b2"))+":passed" {
		t.Fatalf("int-a history = %v, want b1 replaced then b2 passed", got)
	}
	if e.History[0].Gate != analysis || e.History[0].PromotedAt != "2026-11-01T10:40:00Z" || e.History[0].GitOpsPR != 41 {
		t.Errorf("replaced attempt = %+v", e.History[0])
	}
	if s := stage(t, c, "env:int"); s.State != StagePassed || s.FinishedAt != "2026-11-01T10:40:00Z" || s.FailedAttempts != 0 {
		t.Errorf("int stage = %+v, want Passed at 10:40 with no failed attempt", s)
	}
	// A change first in b2 has one attempt there.
	if got := outcomes(envOf(t, change(t, p, f.repo.sha(4)), "int-a").History); len(got) != 1 {
		t.Errorf("c04 int-a history = %v, want only b2", got)
	}
}

func TestPromotionHistoryFailedAttempt(t *testing.T) {
	// Verifies: FLT-DLV-18.02, FLT-DLV-18.03
	f := newFixture(t)
	analysis := "hypershell-analysis"
	// m1 (b1) failed, m2 (b2) passed, m3 (b2 again, re-rendered) is active.
	// Each outcome must come from the promotion's own commit.
	withIntHistory(f, []EnvPromotion{
		{SHA: "m3", MergedAt: "2026-11-01T11:30:00Z", PR: 3},
		{SHA: "m2", MergedAt: "2026-11-01T10:50:00Z", PR: 2},
		{SHA: "m1", MergedAt: "2026-11-01T10:40:00Z", PR: 1},
	}, map[string]string{"m1": "sha256:b1", "m2": "sha256:b2", "m3": "sha256:b2"}, []Gate{{Key: analysis, Phase: "success"}})
	f.gh.gateRuns = map[string][]CheckRun{
		"m1": {{Name: analysis, Status: "completed", Conclusion: "failure"}},
		"m2": {{Name: analysis, Status: "completed", Conclusion: "success"}},
	}
	p := f.refresh(t)
	c := change(t, p, f.repo.sha(2))
	h := envOf(t, c, "int-a").History
	if len(h) != 2 || h[0].Outcome != OutcomeFailed || h[0].Gate != analysis || h[1].Outcome != OutcomePassed {
		t.Fatalf("int-a history = %v, want b1 failed then b2 passed (stops at the first pass)", outcomes(h))
	}
	if s := stage(t, c, "env:int"); s.State != StagePassed || s.FailedAttempts != 1 {
		t.Errorf("int stage = %+v, want Passed with 1 failed attempt", s)
	}

	// Settled outcomes and an unchanged branch are not read again.
	f.refresh(t)
	if f.gh.envCalls["env/int-a"] != 1 || f.gh.gateCalls["m1"] != 1 || f.gh.gateCalls["m2"] != 1 {
		t.Errorf("reads after a second refresh: branch %d, m1 %d, m2 %d; want 1 each",
			f.gh.envCalls["env/int-a"], f.gh.gateCalls["m1"], f.gh.gateCalls["m2"])
	}
	if f.gh.gateCalls["m3"] != 0 {
		t.Errorf("the active promotion's gates were read from GitHub")
	}
	if st := p.Sources[srcHistory]; st.Never || st.Stale {
		t.Errorf("history status = %+v, want current", st)
	}
}

func TestPassedGroupIgnoresNewerBundleChecks(t *testing.T) {
	// Verifies: FLT-DLV-08.03
	f := newFixture(t)
	analysis := "hypershell-analysis"
	m1, m2 := strings.Repeat("1", 40), strings.Repeat("2", 40)
	// b1 passed in int-a; the newer b2 (which also contains b1's changes) is
	// active and its release analysis is still running.
	withIntHistory(f, []EnvPromotion{
		{SHA: m2, MergedAt: "2026-11-01T11:30:00Z", PR: 2},
		{SHA: m1, MergedAt: "2026-11-01T10:40:00Z", PR: 1},
	}, map[string]string{m1: "sha256:b1", m2: "sha256:b2"}, []Gate{{Key: analysis, Phase: "pending"}})
	// The promotion payload carries a shortened commit ID.
	env := f.promo.Environments["int-a"]
	env.ActiveHydrated = m2[:10]
	f.promo.Environments["int-a"] = env
	f.gh.gateRuns = map[string][]CheckRun{m1: {{Name: analysis, Status: "completed", Conclusion: "success"}}}

	p := f.refresh(t)
	old := change(t, p, f.repo.sha(2)) // first bundle b1
	if s := stage(t, old, "env:int"); s.State != StagePassed {
		t.Errorf("int stage of a change that passed in b1 = %+v, want Passed", s)
	}
	if e := envOf(t, old, "int-a"); e.State != StagePassed || len(e.History) != 1 || e.History[0].Outcome != OutcomePassed {
		t.Errorf("int-a for the b1 change = %s %v, want Passed with b1 passed", e.State, outcomes(e.History))
	}

	cur := change(t, p, f.repo.sha(5)) // first bundle b2, the active one
	if s := stage(t, cur, "env:int"); s.State != StageRunning || s.Gate != analysis {
		t.Errorf("int stage of the b2 change = %+v, want Running on %s", s, analysis)
	}
	h := envOf(t, cur, "int-a").History
	if len(h) != 1 || h[0].Outcome != OutcomeRunning || h[0].Gate != analysis {
		t.Errorf("int-a history for the b2 change = %v, want the active b2 running", outcomes(h))
	}
	if f.gh.gateCalls[m2] != 0 {
		t.Errorf("the active promotion's gates were read from GitHub")
	}
}

func TestGateOutcome(t *testing.T) {
	keys := []string{"argocd-health", "hypershell-analysis"}
	ok := CheckRun{Name: "argocd-health", Status: "completed", Conclusion: "success"}
	cases := []struct {
		runs []CheckRun
		want string
	}{
		{[]CheckRun{ok, {Name: "hypershell-analysis", Status: "completed", Conclusion: "success"}}, OutcomePassed},
		{[]CheckRun{ok, {Name: "hypershell-analysis", Status: "completed", Conclusion: "failure"}}, OutcomeFailed},
		{[]CheckRun{ok, {Name: "hypershell-analysis", Status: "in_progress"}}, OutcomeReplaced},
		{[]CheckRun{ok}, OutcomeReplaced},
		// A run named after another branch's gate does not count.
		{[]CheckRun{ok, {Name: "hypershell-analysis/env/x", Status: "completed", Conclusion: "failure"}}, OutcomeReplaced},
	}
	for i, c := range cases {
		if got, _ := gateOutcome(c.runs, keys); got != c.want {
			t.Errorf("case %d: outcome %s, want %s", i, got, c.want)
		}
	}
	if got, _ := gateOutcome(nil, nil); got != OutcomePassed {
		t.Errorf("no gates: outcome %s, want passed", got)
	}
}
