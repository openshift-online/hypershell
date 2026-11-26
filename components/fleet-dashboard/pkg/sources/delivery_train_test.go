package sources

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// Component-level delivery, the bundle train, and the Pipeline now panel over
// pure inputs (FLT-DLV-02, -05, -06, -08, -10, -22, -23).

func tm(m int) string {
	return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC).Add(time.Duration(m) * time.Minute).Format(time.RFC3339)
}

func run(name, status, conclusion string, done int) CheckRun {
	r := CheckRun{Name: "Konflux / " + name, App: "konflux", Status: status, Conclusion: conclusion, StartedAt: tm(done - 5)}
	if status == "completed" {
		r.CompletedAt = tm(done)
	}
	return r
}

func passedBuild(comp string, done int) []CheckRun {
	return []CheckRun{
		run(comp+"-on-push", "completed", "success", done),
		run("app-enterprise-contract / "+comp, "completed", "success", done+1),
	}
}

func bundle(n int, rev string, comps map[string]string) *Release {
	tag := bundleTag(n, fmt.Sprintf("b%d", n))
	return &Release{Version: fmt.Sprintf("v%d", n), Tag: tag, Digest: fmt.Sprintf("sha256:d%d", n), Date: tm(n * 10), ManifestsRev: rev, Components: comps}
}

// compInput: commits c1..c5 (c5 newest). c4 rebuilt api, cli and agent; its
// agent build still runs. c2's api build failed and c4 rebuilt api. Bundles:
// b1 at c1, b2 carries api from c4, b3 carries api and cli from c4. Int is
// hub and spoke (roots); stage-a depends on hub only, stage-b on both.
func compInput() deliveryInput {
	commits := []UpstreamCommit{}
	for i := 5; i >= 1; i-- {
		commits = append(commits, UpstreamCommit{SHA: fmt.Sprintf("c%d", i), Message: fmt.Sprintf("change %d (#%d)", i, i), MergedAt: tm(i - 10)})
	}
	all := func(api, cli, agent string) map[string]string {
		return map[string]string{"api": api, "cli": cli, "agent": agent}
	}
	b1, b2, b3 := bundle(1, "c1", all("c1", "c1", "c1")), bundle(2, "c4", all("c4", "c1", "c1")), bundle(3, "c4", all("c4", "c4", "c1"))
	checks := map[string][]CheckRun{
		"c5": {},
		"c4": append(append(passedBuild("api", 20), passedBuild("cli", 22)...), run("agent-on-push", "in_progress", "", 0)),
		"c3": {},
		"c2": {run("api-on-push", "completed", "failure", 5)},
		"c1": {},
	}
	envs := map[string]Environment{
		"hub":     {Env: "int"},
		"spoke":   {Env: "int"},
		"stage-a": {Env: "stage", DependsOn: []string{"hub"}},
		"stage-b": {Env: "stage", DependsOn: []string{"hub", "spoke"}},
	}
	return deliveryInput{
		now:          time.Date(2026, 11, 1, 13, 0, 0, 0, time.UTC),
		window:       30,
		commits:      commits,
		bundles:      []*Release{b1, b2, b3},
		lockedSet:    map[string]bool{b1.Digest: true, b2.Digest: true},
		lockCommits:  []LockCommit{{SHA: "l2", Digest: b2.Digest, Date: tm(25), Message: "Update (#42)"}, {SHA: "l1", Digest: b1.Digest, Date: tm(12), Message: "Update (#41)"}},
		gitopsKnown:  true,
		pendingKnown: true,
		published:    map[string]time.Time{},
		promo:        &PromotionPayload{Order: []string{"hub", "spoke", "stage-a", "stage-b"}, Environments: envs},
		history:      map[string][]histPromotion{},
		checks:       checks,
		sources:      map[string]DeliverySourceStatus{},
	}
}

func setEnv(in *deliveryInput, key string, active, proposed *Release, phase string) {
	env := in.promo.Environments[key]
	env.Active, env.Proposed = active, proposed
	env.ActiveGates = []Gate{{Key: "hypershell-analysis", Phase: phase}}
	in.promo.Environments[key] = env
}

func payloadOf(in deliveryInput) DeliveryPayload {
	listed, omitted := selectWindow(in)
	return buildDelivery(in, listed, omitted)
}

func TestSupersededBuild(t *testing.T) {
	// Verifies: FLT-DLV-05.04
	p := payloadOf(compInput())
	c := change(t, p, "c2")
	b := stage(t, c, stageBuild)
	if b.State != StagePassed || b.Reason != reasonRebuiltIn || b.RebuiltIn == nil || b.RebuiltIn.ShortSHA != "c4" || b.RebuiltIn.PR != 4 {
		t.Fatalf("c2 build = %+v, want Passed, rebuilt in c4 (#4)", b)
	}
	if s := stage(t, c, stagePolicy); s.State != StagePassed {
		t.Errorf("c2 policy = %+v, want c4's api policy check (Passed)", s)
	}
	if s := stage(t, c, stageRelease); s.State != StagePassed {
		t.Errorf("c2 release = %+v, want Passed in c4's api bundle", s)
	}
	if len(c.Builds) != 1 || c.Builds[0].State != StageFailed || c.Builds[0].SupersededBy != "c4" {
		t.Errorf("c2 builds = %+v, want its failed api build superseded by c4", c.Builds)
	}

	// Without the later build, the failed build stays Failed.
	in := compInput()
	in.checks["c4"] = nil
	delete(in.checks, "c4")
	if s := stage(t, change(t, payloadOf(in), "c2"), stageBuild); s.State != StageFailed || s.RebuiltIn != nil {
		t.Errorf("c2 build without a later build = %+v, want Failed", s)
	}
}

func TestPartlyShipped(t *testing.T) {
	// Verifies: FLT-DLV-06.07, FLT-DLV-08.05, FLT-DLV-22
	in := compInput()
	p := payloadOf(in)
	c := change(t, p, "c4")
	r := stage(t, c, stageRelease)
	if r.State != StageRunning || r.Reason != reasonPartlyReleased || r.Partly == nil || r.Partly.Done != 2 || r.Partly.Total != 3 {
		t.Fatalf("c4 release = %+v, want Running, 2 of 3 components released", r)
	}
	if len(r.Partly.Missing) != 1 || r.Partly.Missing[0] != (DeliveryMissing{Component: "agent", State: missingBuilding}) {
		t.Errorf("missing = %+v, want agent building", r.Partly.Missing)
	}

	// b3 locked and active in Int with passed gates: Int is partly, not Passed.
	b3 := in.bundles[2]
	in.lockedSet[b3.Digest] = true
	in.lockCommits = append([]LockCommit{{SHA: "l3", Digest: b3.Digest, Date: tm(35), Message: "Update (#43)"}}, in.lockCommits...)
	setEnv(&in, "hub", b3, b3, "success")
	setEnv(&in, "spoke", b3, b3, "success")
	c = change(t, payloadOf(in), "c4")
	if s := stage(t, c, stageGitOps); s.State != StageRunning || s.Reason != reasonPartlyShipped || s.Partly == nil || s.Partly.Done != 2 {
		t.Errorf("c4 gitops = %+v, want Running, partly 2 of 3", s)
	}
	if s := stage(t, c, "env:int"); s.State != StageRunning || s.Reason != reasonPartlyShipped || s.Partly == nil || s.Partly.Total != 3 {
		t.Errorf("c4 int = %+v, want Running, partly 2 of 3 (not Passed)", s)
	}
	if c.Delivered {
		t.Error("a partly shipped change reads delivered")
	}
}

func TestWaitsForOwnDependencies(t *testing.T) {
	// Verifies: FLT-DLV-08.04
	in := compInput()
	b1, b2 := in.bundles[0], in.bundles[1]
	setEnv(&in, "hub", b2, b2, "success")
	setEnv(&in, "spoke", b1, b2, "success")
	setEnv(&in, "stage-a", b1, b2, "success")
	setEnv(&in, "stage-b", b1, b2, "success")
	c := change(t, payloadOf(in), "c2") // first bundle b2
	if e := envOf(t, c, "stage-a"); len(e.WaitingFor) != 0 {
		t.Errorf("stage-a waits for %v, want nothing: its only dependency passed", e.WaitingFor)
	}
	if e := envOf(t, c, "stage-b"); strings.Join(e.WaitingFor, ",") != "spoke" {
		t.Errorf("stage-b waits for %v, want spoke", e.WaitingFor)
	}
	s := stage(t, c, "env:stage")
	if s.Reason != reasonWaitingEnvs || strings.Join(s.WaitingFor, ",") != "spoke" {
		t.Errorf("stage = %+v, want waiting for spoke only, not for the Int group", s)
	}
}

func TestDeliveryTime(t *testing.T) {
	// Verifies: FLT-DLV-02.03
	in := compInput()
	b1 := in.bundles[0]
	for _, k := range in.promo.Order {
		setEnv(&in, k, b1, b1, "success")
		in.history[k] = []histPromotion{{EnvPromotion: EnvPromotion{SHA: "m-" + k, MergedAt: tm(13)}, digest: b1.Digest, outcome: OutcomePassed, gatesAt: tm(20)}}
	}
	in.history["stage-b"][0].gatesAt = tm(40)
	if c := change(t, payloadOf(in), "c1"); !c.Delivered || c.DeliveredAt != tm(40) {
		t.Errorf("c1 delivered at %q (delivered %v), want the last environment's finish %s", c.DeliveredAt, c.Delivered, tm(40))
	}
	// History that does not reach one environment's finish omits the time.
	delete(in.history, "stage-b")
	if c := change(t, payloadOf(in), "c1"); !c.Delivered || c.DeliveredAt != "" {
		t.Errorf("c1 delivered at %q, want omitted when the history does not reach it", c.DeliveredAt)
	}
}

func TestBundleTrain(t *testing.T) {
	// Verifies: FLT-DLV-22.01, FLT-DLV-22.02
	in := compInput()
	b1 := in.bundles[0]
	for _, k := range []string{"hub", "spoke"} {
		in.history[k] = []histPromotion{{EnvPromotion: EnvPromotion{SHA: "m-" + k, MergedAt: tm(13)}, digest: b1.Digest, outcome: OutcomePassed, gatesAt: tm(20)}}
	}
	// b4 repeats b3's components under a new release name; agent finished at c5.
	b3 := in.bundles[2]
	b4 := bundle(4, "c4", b3.Components)
	in.bundles = append(in.bundles, b4)
	in.checks["c5"] = passedBuild("agent", 50)
	p := payloadOf(in)
	if len(p.Train) != 4 {
		t.Fatalf("train = %+v, want b2, b3, b4 and the inferred agent bundle", p.Train)
	}
	if p.Train[0].Version != "v2" || len(p.Train[0].Changed) != 1 || p.Train[0].Changed[0].Component != "api" {
		t.Errorf("train[0] = %+v, want v2 changing api", p.Train[0])
	}
	if !p.Train[2].Same || len(p.Train[2].Changed) != 0 {
		t.Errorf("train[2] = %+v, want b4 with the same components", p.Train[2])
	}
	last := p.Train[3]
	if !last.Inferred || len(last.Changed) != 1 || last.Changed[0].Component != "agent" || last.PolicyAt != tm(51) {
		t.Errorf("train[3] = %+v, want the agent bundle releasing (inferred)", last)
	}
}

func TestPipelineNow(t *testing.T) {
	// Verifies: FLT-DLV-23.01, FLT-DLV-10.07
	in := compInput()
	b1, b2, b3 := in.bundles[0], in.bundles[1], in.bundles[2]
	in.lockedSet[b3.Digest] = true
	in.lockCommits = append([]LockCommit{{SHA: "l3", Digest: b3.Digest, Date: tm(35), Message: "Update (#43)"}}, in.lockCommits...)
	in.checks["c5"] = passedBuild("agent", 50)
	setEnv(&in, "hub", b3, b3, "pending")
	setEnv(&in, "spoke", b3, b3, "success")
	setEnv(&in, "stage-a", b1, b3, "success")
	setEnv(&in, "stage-b", b1, b3, "success")
	env := in.promo.Environments["hub"]
	env.ActiveHydrated = "mhub"
	in.promo.Environments["hub"] = env
	in.history["hub"] = []histPromotion{{EnvPromotion: EnvPromotion{SHA: "mhub000", MergedAt: tm(43)}, digest: b3.Digest, attempt: 1, attemptOf: 3}}
	_ = b2

	pl := placeBundles(in)
	groups := groupsOf(in.promo)
	listed, _ := selectWindow(in)
	p := buildDelivery(in, listed, 0)
	est := map[string]PhaseEstimate{
		stageRelease:        {MedianSeconds: 120, SlowestSeconds: 300, Samples: 5},
		stageGitOps:         {MedianSeconds: 360, SlowestSeconds: 600, Samples: 5},
		envPhase("hub"):     {MedianSeconds: 1500, SlowestSeconds: 1800, Samples: 5},
		envPhase("spoke"):   {MedianSeconds: 300, SlowestSeconds: 400, Samples: 5},
		envPhase("stage-a"): {MedianSeconds: 600, SlowestSeconds: 900, Samples: 5},
		envPhase("stage-b"): {MedianSeconds: 600, SlowestSeconds: 900, Samples: 5},
	}
	train := buildTrain(in, pl, groups, est)
	now := buildNow(in, pl, groups, est, train, p.Changes)

	if now.AppHead == nil || now.AppHead.ShortSHA != "c5" || now.AppHead.PR != 5 {
		t.Errorf("app head = %+v, want c5 (#5)", now.AppHead)
	}
	if now.GitOpsHead == nil || now.GitOpsHead.Version != "v3" || now.GitOpsHead.LockedAt != tm(35) || now.GitOpsHead.PR != 43 {
		t.Errorf("gitops head = %+v, want v3 locked at %s by #43", now.GitOpsHead, tm(35))
	}
	var hub, stageB DeliveryEnvNow
	for _, e := range now.Environments {
		switch e.Env {
		case "hub":
			hub = e
		case "stage-b":
			stageB = e
		}
	}
	if len(now.Environments) != 4 || hub.State != StageRunning || hub.Attempt != 1 || hub.AttemptOf != 3 || hub.Since != tm(43) || hub.Version != "v3" {
		t.Errorf("hub now = %+v, want running v3 since %s, attempt 1 of 3", hub, tm(43))
	}
	if stageB.State != StageWaiting || strings.Join(stageB.WaitingFor, ",") != "hub" || stageB.ProposedVersion != "v3" {
		t.Errorf("stage-b now = %+v, want waiting for hub with v3 proposed", stageB)
	}
	if len(now.Checks) != 1 || now.Checks[0].Component != "agent" || now.Checks[0].Kind != stageBuild {
		t.Errorf("checks = %+v, want c4's agent build running", now.Checks)
	}
	// agent's policy passed at 12:51: published about 12:53, locked about
	// 12:59 (now 13:00), and lands in hub before its analysis ends at 13:08.
	// c4 is partly shipped, so only c5 expects a bundle.
	kinds := []string{}
	for _, e := range now.Next {
		kinds = append(kinds, e.Kind)
	}
	if strings.Join(kinds, ",") != "expecting-bundle,restart,promote" {
		t.Fatalf("next = %v, want expecting-bundle, restart, promote", kinds)
	}
	if e := now.Next[0]; len(e.Changes) != 1 || e.Changes[0].ShortSHA != "c5" || strings.Join(e.Components, ",") != "agent" {
		t.Errorf("expecting = %+v, want c5's agent bundle", e)
	}
	if r := now.Next[1]; strings.Join(r.Envs, ",") != "hub" || strings.Join(r.Components, ",") != "agent" {
		t.Errorf("restart = %+v, want hub restarted by the agent bundle", r)
	}
	if pr := now.Next[2]; strings.Join(pr.Envs, ",") != "stage-a,stage-b" || strings.Join(pr.After, ",") != "hub" || pr.At != tm(68) {
		t.Errorf("promote = %+v, want Stage after hub at %s", pr, tm(68))
	}
	if !slicesEqual(train[len(train)-1].Restarts, []string{"hub"}) {
		t.Errorf("inferred bundle restarts = %v, want hub", train[len(train)-1].Restarts)
	}
	s := stage(t, change(t, p, "c4"), "env:int")
	if s.Restart == nil || strings.Join(s.Restart.Envs, ",") != "hub" || strings.Join(s.Restart.Components, ",") != "agent" {
		t.Errorf("c4 int = %+v, want a restart by the agent bundle", s)
	}
}

func TestPipelineNowQuiet(t *testing.T) {
	// Verifies: FLT-DLV-23.02
	in := compInput()
	b2 := in.bundles[1]
	in.bundles = in.bundles[:2]
	in.checks = map[string][]CheckRun{}
	for _, k := range in.promo.Order {
		setEnv(&in, k, b2, b2, "success")
	}
	p := payloadOf(in)
	if len(p.Now.Checks) != 0 || len(p.Now.Next) != 0 || len(p.Now.Environments) != 4 {
		t.Errorf("now = %+v, want nothing in flight and every environment listed", p.Now)
	}
	for _, e := range p.Now.Environments {
		if e.State != StagePassed || e.Version != "v2" {
			t.Errorf("%s now = %+v, want v2 passed", e.Env, e)
		}
	}
}

func slicesEqual(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }

func TestPipelineNowInFlightChecks(t *testing.T) {
	// Verifies: FLT-DLV-23.03
	in := compInput()
	// c4 builds agent on push. Its merge queue also ran policy checks for web
	// (no push build) and a group check: neither is a stage of c4.
	in.checks["c4"] = append(in.checks["c4"],
		run("app-enterprise-contract / web", "in_progress", "", 0),
		run("app-enterprise-contract / pr group", "in_progress", "", 0),
	)
	// c1 is delivered everywhere; a policy check of its own api build stays open.
	in.checks["c1"] = []CheckRun{
		run("api-on-push", "completed", "success", -8),
		run("app-enterprise-contract / api", "in_progress", "", 0),
	}
	b1 := in.bundles[0]
	for _, k := range in.promo.Order {
		setEnv(&in, k, b1, b1, "success")
	}
	p := payloadOf(in)
	var got []string
	for _, c := range p.Now.Checks {
		got = append(got, c.Kind+":"+c.Component+"@"+c.ShortSHA)
	}
	if !slicesEqual(got, []string{"build:agent@c4"}) {
		t.Errorf("in flight = %v, want only c4's agent build", got)
	}
}

func estP(m int64) *int64 { v := m * 60; return &v }

func TestExpectingBundle(t *testing.T) {
	// Verifies: FLT-DLV-23.04
	at := func(h, m int) string { return time.Date(2026, 11, 25, h, m, 0, 0, time.UTC).Format(time.RFC3339) }
	in := deliveryInput{now: time.Date(2026, 11, 25, 18, 50, 0, 0, time.UTC)}
	pending := func(sha string, pr int, rebuiltIn *DeliveryCommitRef) DeliveryChange {
		return DeliveryChange{SHA: sha, ShortSHA: sha, PR: pr, Builds: []DeliveryCheck{{Component: "api-server"}}, Stages: []DeliveryStage{
			{Key: stageMerged, State: StagePassed, FinishedAt: at(18, 42)},
			{Key: stageBuild, State: StageRunning, StartedAt: at(18, 42), EstimateSeconds: estP(14), Components: []string{"api-server"}, RebuiltIn: rebuiltIn},
			{Key: stagePolicy, State: StageNotStarted, EstimateSeconds: estP(5)},
			{Key: stageRelease, State: StageNotStarted, EstimateSeconds: estP(15)},
		}}
	}
	// Newest first, as in the payload. c1190's failed build was superseded
	// by c1192's, so both ride one bundle.
	shipped := pending("c1189", 1189, nil)
	shipped.Bundle = &DeliveryBundle{Digest: "sha256:x"}
	changes := []DeliveryChange{
		pending("c1192", 1192, nil),
		pending("c1190", 1190, &DeliveryCommitRef{SHA: "c1192"}),
		shipped,
	}
	got := expectingBundles(in, changes)
	if len(got) != 1 {
		t.Fatalf("expecting = %+v, want one line for the shared bundle", got)
	}
	e := got[0]
	// 6 min of the build left, then 5 and 15: 26 min from 18:50.
	if e.Kind != nextExpecting || e.At != at(19, 16) || len(e.Changes) != 2 || e.Changes[0].PR != 1190 || e.Changes[1].PR != 1192 {
		t.Errorf("expecting = %+v, want #1190 and #1192 about 19:16", e)
	}
	// An unknown estimate omits the time.
	changes[0].Stages[3].EstimateSeconds = nil
	if got := expectingBundles(in, changes[:1]); len(got) != 1 || got[0].At != "" {
		t.Errorf("expecting = %+v, want no time without the Release estimate", got)
	}
}

func TestRenovateEvents(t *testing.T) {
	// Verifies: FLT-DLV-23.05
	in := deliveryInput{now: time.Date(2026, 11, 25, 19, 12, 0, 0, time.UTC)}
	at := func(m int) string { return time.Date(2026, 11, 25, 19, m, 0, 0, time.UTC).Format(time.RFC3339) }
	train := []DeliveryTrainBundle{
		{Digest: "sha256:old", Version: "v1", Locked: true},
		{Digest: "sha256:b", Version: "v2", PublishedAt: at(9), Changed: []DeliveryComponentRev{{Component: "api-server"}}},
		{Inferred: true},
	}
	est := map[string]PhaseEstimate{stageGitOps: {MedianSeconds: 360}}
	got := renovateEvents(in, train, est)
	if len(got) != 1 || got[0].State != renovateWaiting || got[0].Digest != "sha256:b" || got[0].At != at(15) {
		t.Fatalf("renovate = %+v, want B waiting, locked about 19:15", got)
	}
	in.bundlePR = &BundlePR{Number: 2951, Checks: "running", CreatedAt: at(10)}
	in.bundlePRDig = "sha256:b"
	got = renovateEvents(in, train, est)
	if len(got) != 1 || got[0].State != renovateRunning || got[0].PR != 2951 || got[0].At != at(16) {
		t.Errorf("renovate = %+v, want PR #2951 open, checks running, locked about 19:16", got)
	}
	in.bundlePR.Checks = "failed"
	if got = renovateEvents(in, train, est); got[0].State != renovateFailed || got[0].At != "" {
		t.Errorf("renovate = %+v, want failed with no expected time", got)
	}
}

func TestRenovateStage(t *testing.T) {
	// Verifies: FLT-DLV-07.01, FLT-DLV-07.02
	in := compInput()
	// Only b1 is locked: c2's first bundle b2 waits for Renovate.
	in.lockedSet = map[string]bool{in.bundles[0].Digest: true}
	in.lockCommits = in.lockCommits[1:]
	in.published[in.bundles[1].Tag] = mustTime(tm(30))
	if s := stage(t, change(t, payloadOf(in), "c2"), stageGitOps); s.State != StageWaiting || s.Reason != reasonWaitingBundlePR || s.StartedAt != tm(30) {
		t.Errorf("c2 renovate = %+v, want Waiting for Renovate since publication at %s", s, tm(30))
	}
	// The open pull request proposes b3, which contains c2.
	in.bundlePR = &BundlePR{Number: 2951, Checks: "running", CreatedAt: tm(33)}
	in.bundlePRDig = in.bundles[2].Digest
	if s := stage(t, change(t, payloadOf(in), "c2"), stageGitOps); s.State != StageRunning || s.PR != 2951 || s.Checks != "running" || s.StartedAt != tm(33) {
		t.Errorf("c2 renovate = %+v, want Running, PR #2951 opened at %s", s, tm(33))
	}
	in.bundlePR.Checks = "failed"
	if s := stage(t, change(t, payloadOf(in), "c2"), stageGitOps); s.State != StageFailed || s.PR != 2951 {
		t.Errorf("c2 renovate = %+v, want Failed, PR #2951 checks failed", s)
	}
}

func TestChangesNewestFirst(t *testing.T) {
	// Verifies: FLT-DLV-03.03
	in := compInput()
	listed, _ := selectWindow(in)
	slices.Reverse(listed)
	p := buildDelivery(in, listed, 0)
	var got []string
	for _, c := range p.Changes {
		got = append(got, c.SHA)
	}
	if !slicesEqual(got, []string{"c5", "c4", "c3", "c2", "c1"}) {
		t.Errorf("changes = %v, want newest first", got)
	}
}
