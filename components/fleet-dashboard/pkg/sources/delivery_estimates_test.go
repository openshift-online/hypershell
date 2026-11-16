package sources

import (
	"testing"
	"time"
)

func at(hm string) time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-11-01T"+hm+":00Z")
	return t
}

func runsOf(ends []string, mins []int) []phaseRun {
	out := make([]phaseRun, len(mins))
	for i, m := range mins {
		out[i] = phaseRun{end: at(ends[i]), dur: time.Duration(m) * time.Minute}
	}
	return out
}

func TestEstimatesFromLastFiveRuns(t *testing.T) {
	// Verifies: FLT-DLV-09.01, FLT-DLV-09.02
	ends := []string{"01:00", "02:00", "03:00", "04:00", "05:00", "06:00", "07:00"}
	samples := map[string][]phaseRun{
		stageRelease: runsOf(ends, []int{30, 31, 32, 40, 41, 42, 90}),
		stagePolicy:  runsOf(ends[:2], []int{3, 4}),
	}
	got := estimates(samples, at("08:00"))
	e, ok := got[stageRelease]
	if !ok || e.MedianSeconds != 41*60 || e.SlowestSeconds != 90*60 || e.Samples != 5 {
		t.Errorf("release estimate = %+v, want median 41 min, slowest 90 min from the last 5 runs", e)
	}
	if _, ok := got[stagePolicy]; ok {
		t.Error("two runs produced an estimate")
	}
	// Runs that end after now are not history yet.
	if e := estimates(samples, at("05:30"))[stageRelease]; e.MedianSeconds != 32*60 || e.SlowestSeconds != 41*60 {
		t.Errorf("estimate at 05:30 = %+v, want median 32, slowest 41 from the 5 runs ended by then", e)
	}
}

func TestStageEstimateSlowestComponent(t *testing.T) {
	// Verifies: FLT-DLV-09.04
	est := map[string]PhaseEstimate{
		buildPhase("a"): {MedianSeconds: 8 * 60, SlowestSeconds: 20 * 60, Samples: 5},
		buildPhase("b"): {MedianSeconds: 14 * 60, SlowestSeconds: 16 * 60, Samples: 3},
	}
	e, ok := stageEstimate(est, stageBuild, []string{"a", "b"})
	if !ok || e.MedianSeconds != 14*60 || e.SlowestSeconds != 20*60 {
		t.Errorf("build estimate = %+v, want 14 min estimate and 20 min slowest", e)
	}
	if _, ok := stageEstimate(est, stageBuild, []string{"a", "c"}); ok {
		t.Error("a component without an estimate still produced one")
	}
	if _, ok := stageEstimate(est, stagePolicy, []string{"a"}); ok {
		t.Error("policy check estimate without policy runs")
	}
}

func TestGateTimesSkipRerun(t *testing.T) {
	// Verifies: FLT-DLV-09.05, FLT-DLV-08
	keys := []string{"analysis", "health"}
	once := []CheckRun{
		{Name: "analysis", Status: "completed", Conclusion: "success", StartedAt: "2026-11-01T10:00:00Z", CompletedAt: "2026-11-01T10:20:00Z"},
		{Name: "health", Status: "completed", Conclusion: "success", StartedAt: "2026-11-01T10:00:00Z", CompletedAt: "2026-11-01T10:05:00Z"},
	}
	if when, rerun := gateTimes(once, keys); when != "2026-11-01T10:20:00Z" || rerun {
		t.Errorf("gate times = %s, %v; want 10:20 without re-run", when, rerun)
	}
	twice := append(append([]CheckRun(nil), once...), CheckRun{Name: "analysis", Status: "completed", Conclusion: "success", StartedAt: "2026-11-01T11:00:00Z", CompletedAt: "2026-11-01T11:10:00Z"})
	when, rerun := gateTimes(twice, keys)
	if !rerun || when != "2026-11-01T11:10:00Z" {
		t.Errorf("gate times = %s, %v; want the latest run and a re-run flag", when, rerun)
	}
	if o, _ := gateOutcome(twice, keys); o != OutcomePassed {
		t.Errorf("outcome = %s, want passed from the latest runs", o)
	}

	// A re-run promotion is not sampled for its group.
	in := deliveryInput{history: map[string][]histPromotion{
		"e1": {{digest: "d", gatesAt: when, rerun: true}},
	}}
	if got := groupFinishFor(in, envGroup{name: "int", envs: []string{"e1"}}, "d"); !got.IsZero() {
		t.Errorf("group finish = %v, want none for a re-run gate", got)
	}
}

func TestPhaseSamplesOncePerBundle(t *testing.T) {
	// Verifies: FLT-DLV-09.03
	in := deliveryInput{
		bundles: []*Release{
			{Tag: bundleTag(1, "b1"), Digest: "sha256:b1", Date: "2026-11-01T10:01:00Z"},
			{Tag: bundleTag(2, "b2"), Digest: "sha256:b2", Date: "2026-11-01T10:02:00Z"},
		},
		lockCommits: []LockCommit{
			{Digest: "sha256:b2", Date: "2026-11-01T11:00:00Z"},
			{Digest: "sha256:b1", Date: "2026-11-01T10:30:00Z"},
		},
		// Ten changes ride these two bundles.
		commits: linearRepo{n: 10}.newestFirst(at("09:00")),
	}
	got := phaseSamples(in, placeBundles(in), nil)[stageGitOps]
	if len(got) != 2 {
		t.Fatalf("GitOps update runs = %d, want one per bundle (2), not one per change", len(got))
	}
}

func TestStageOverdue(t *testing.T) {
	// Verifies: FLT-DLV-04.01, FLT-DLV-04.02
	slowest := int64(14 * 60)
	s := DeliveryStage{Key: stageBuild, State: StageRunning, StartedAt: "2026-11-01T10:00:00Z", SlowestSeconds: &slowest}
	if !stageOverdue(s, at("10:20")) {
		t.Error("20 min against a 14 min slowest run is Overdue")
	}
	if stageOverdue(s, at("10:18")) {
		t.Error("18 min is within the slowest run plus 5 minutes")
	}
	s.SlowestSeconds = nil
	if stageOverdue(s, at("12:00")) {
		t.Error("a stage without an estimate is never Overdue")
	}
}

// withComponents gives the fixture's bundles component revisions: B1 web c02
// and api c01, B2 web c03 and api c05, B3 (pending) web c07 and api c08.
func withComponents(f *fixture) {
	f.lock.releases["lock1"].Components = map[string]string{"demo-web-main": f.repo.sha(2), "demo-api-main": f.repo.sha(1)}
	f.lock.releases["lock2"].Components = map[string]string{"demo-web-main": f.repo.sha(3), "demo-api-main": f.repo.sha(5)}
	f.reg.comps = map[string]map[string]string{"sha256:b3": {"demo-web-main": f.repo.sha(7), "demo-api-main": f.repo.sha(8)}}
}

func TestDeliveryContainmentByComponentRevision(t *testing.T) {
	// Verifies: FLT-DLV-06.05, FLT-DLV-06.02
	f := newFixture(t)
	withComponents(f)
	// c04 rebuilt web and the build failed; B2 (manifests c05) still carries
	// web from c03.
	f.gh.checks[f.repo.sha(4)] = []CheckRun{konflux("demo-web-main-on-push", "completed", "failure")}
	// c05 rebuilt api; B2 carries api from c05.
	f.gh.checks[f.repo.sha(5)] = []CheckRun{konflux("demo-api-main-on-push", "completed", "success")}
	p := f.refresh(t)
	if b := change(t, p, f.repo.sha(4)).Bundle; b == nil || b.Digest != "sha256:b3" {
		t.Errorf("c04 first bundle = %+v, want B3: B2's web revision is older than c04", b)
	}
	if b := change(t, p, f.repo.sha(5)).Bundle; b == nil || b.Digest != "sha256:b2" {
		t.Errorf("c05 first bundle = %+v, want B2", b)
	}
	// A change that rebuilt nothing keeps the manifest-revision rule.
	if b := change(t, p, f.repo.sha(3)).Bundle; b == nil || b.Digest != "sha256:b2" {
		t.Errorf("c03 first bundle = %+v, want B2 by manifest revision", b)
	}
}

func TestDeliveryOutsideBundleComponent(t *testing.T) {
	// Verifies: FLT-DLV-06.06, FLT-DLV-05.01
	f := newFixture(t)
	withComponents(f)
	f.gh.checks[f.repo.sha(10)] = []CheckRun{
		konflux("demo-board-main-on-push", "completed", "success"),
		konflux("demo-main-enterprise-contract / demo-board-main", "completed", "neutral"),
	}
	p := f.refresh(t)
	c := change(t, p, f.repo.sha(10))
	if s := stage(t, c, stageRelease); s.State != StageNotNeeded || s.Reason != reasonOutsideBundle {
		t.Errorf("release = %+v, want Not needed, outside-bundle", s)
	}
	if len(c.Builds) != 1 || !c.Builds[0].OutsideBundle {
		t.Errorf("builds = %+v, want the build marked outside the bundle", c.Builds)
	}
}

func TestDeliveryReleaseDiagnostics(t *testing.T) {
	// Verifies: FLT-DLV-20.01, FLT-DLV-12
	f := newFixture(t)
	f.d.releasePaths = []string{"pipelines/release-bundle/", "scripts/release_bundle.py"}
	f.d.releaseLogURL = "https://konflux.example/releases"
	f.gh.touching = map[string][]UpstreamCommit{
		"scripts/release_bundle.py": {{SHA: "abcdef1234", Message: "fix(release): read the snapshot (#77)", MergedAt: "2026-11-01T11:00:00Z"}},
	}
	f.gh.latestTag = bundleTag(2, "b2")

	// No Overdue release: no diagnostics and no extra calls.
	p := f.refresh(t)
	if p.Release != nil || f.gh.calls.touching != 0 || f.gh.calls.latest != 0 {
		t.Fatalf("diagnostics read without an Overdue release: %+v", p.Release)
	}

	diag := f.d.releaseDiagnostics(t.Context(), deliveryInput{
		bundles:      []*Release{{Tag: bundleTag(3, "b3"), Digest: "sha256:b3", Date: "2026-11-01T10:03:00Z"}},
		htmlBase:     "https://github.example",
		upstreamRepo: "app/repo",
		published:    map[string]time.Time{},
		lockedSet:    map[string]bool{},
	}, f.reg.tags)
	if diag.LastGood == nil || diag.LastGood.Digest != "sha256:b3" || diag.LogURL == "" {
		t.Errorf("last good / log = %+v", diag)
	}
	if len(diag.Commits) != 1 || diag.Commits[0].PR != 77 || diag.Commits[0].ShortSHA != "abcdef1" {
		t.Errorf("publisher commits = %+v, want the one release_bundle.py commit", diag.Commits)
	}
	if diag.RegistryTag != bundleTag(3, "b3") || diag.GitHubReleaseTag != bundleTag(2, "b2") {
		t.Errorf("tags = %s vs %s", diag.RegistryTag, diag.GitHubReleaseTag)
	}
	if f.gh.calls.touching != 2 || f.gh.calls.latest != 1 {
		t.Errorf("calls = %d path reads, %d release reads; want 2 and 1", f.gh.calls.touching, f.gh.calls.latest)
	}
}

func TestDeliveryReleaseDiagnosticsWhenOverdue(t *testing.T) {
	// Verifies: FLT-DLV-20.01, FLT-DLV-04.01
	slowest := int64(40 * 60)
	p := DeliveryPayload{Changes: []DeliveryChange{{Stages: []DeliveryStage{
		{Key: stageRelease, State: StageRunning, StartedAt: "2026-11-01T11:58:00Z", SlowestSeconds: &slowest},
	}}}}
	if releaseOverdue(p, at("12:40")) {
		t.Error("42 min against a 40 min slowest release is not Overdue yet")
	}
	if !releaseOverdue(p, at("12:44")) {
		t.Error("46 min against a 40 min slowest release is Overdue")
	}
}
