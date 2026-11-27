package sources

import (
	"slices"
	"sort"
	"time"
)

// The bundle train and the Pipeline now panel (fleet-dashboard-delivery-
// pipeline.spec FLT-DLV-22, FLT-DLV-23). Both come from data the payload
// already reads: no extra source call.

// trainLimit bounds the bundles listed in the train.
const trainLimit = 10

// Next event kinds (FLT-DLV-23). The UI owns the wording.
const (
	nextExpecting = "expecting-bundle"
	nextRenovate  = "renovate"
	nextRestart   = "restart"
	nextPromote   = "promote"
)

// Renovate states of a published bundle that is not locked yet (FLT-DLV-07,
// FLT-DLV-23).
const (
	renovateWaiting = "waiting"
	renovateRunning = "running"
	renovatePassed  = "passed"
	renovateFailed  = "failed"
)

// DeliveryTrainBundle is one bundle of the train (FLT-DLV-22).
type DeliveryTrainBundle struct {
	Tag         string `json:"tag,omitempty"`
	Version     string `json:"version,omitempty"`
	Digest      string `json:"digest,omitempty"`
	PublishedAt string `json:"publishedAt,omitempty"`
	Locked      bool   `json:"locked"`
	LockedAt    string `json:"lockedAt,omitempty"`
	// Changed lists the components whose revision differs from the previous
	// bundle; Same is true when none does (only the release name changed).
	Changed []DeliveryComponentRev `json:"changed"`
	Same    bool                   `json:"same"`
	// Inferred marks a bundle not published yet, inferred from finished
	// component builds (Components) whose bundle is releasing.
	Inferred   bool   `json:"inferred,omitempty"`
	BuiltAt    string `json:"builtAt,omitempty"`
	PolicyAt   string `json:"policyAt,omitempty"`
	ExpectedAt string `json:"expectedAt,omitempty"`
	// Active, Proposed, and Replaced name the environments where the bundle
	// is active, proposed, or where a promotion of it was replaced.
	Active   []string `json:"active"`
	Proposed []string `json:"proposed"`
	Replaced []string `json:"replaced"`
	// Restarts names the environments whose running gate this bundle is
	// expected to restart.
	Restarts []string `json:"restarts"`
}

// DeliveryComponentRev is one component at one application commit.
type DeliveryComponentRev struct {
	Component string `json:"component"`
	Revision  string `json:"revision,omitempty"`
}

// DeliveryNow is the situation at this moment (FLT-DLV-23).
type DeliveryNow struct {
	AppHead    *DeliveryAppHead    `json:"appHead,omitempty"`
	GitOpsHead *DeliveryGitOpsHead `json:"gitopsHead,omitempty"`
	// Checks are the component builds and policy checks running on main.
	Checks       []DeliveryNowCheck  `json:"checks"`
	BundlePR     *DeliveryNowPR      `json:"bundlePr,omitempty"`
	Environments []DeliveryEnvNow    `json:"environments"`
	Next         []DeliveryNextEvent `json:"next"`
}

// DeliveryAppHead is the application repository's main head.
type DeliveryAppHead struct {
	SHA      string `json:"sha"`
	ShortSHA string `json:"shortSha"`
	PR       int    `json:"pr,omitempty"`
	Title    string `json:"title"`
	MergedAt string `json:"mergedAt"`
}

// DeliveryGitOpsHead is the bundle GitOps main locks.
type DeliveryGitOpsHead struct {
	Digest      string `json:"digest"`
	Tag         string `json:"tag"`
	Version     string `json:"version"`
	PublishedAt string `json:"publishedAt,omitempty"`
	LockedAt    string `json:"lockedAt,omitempty"`
	PR          int    `json:"pr,omitempty"`
	// SameAs is the digest of the bundle published just before this one when
	// this one is a re-cut with the same component revisions (FLT-DLV-23.06).
	SameAs string `json:"sameAs,omitempty"`
}

// DeliveryNowCheck is one running build or policy check.
type DeliveryNowCheck struct {
	Kind      string `json:"kind"` // "build" or "policy"
	Component string `json:"component"`
	StartedAt string `json:"startedAt,omitempty"`
	ShortSHA  string `json:"shortSha"`
	PR        int    `json:"pr,omitempty"`
}

// DeliveryNowPR is the open Renovate pull request (FLT-DLV-07).
type DeliveryNowPR struct {
	Number    int    `json:"number"`
	URL       string `json:"url,omitempty"`
	Checks    string `json:"checks,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	// Digest and Version name the bundle the pull request proposes, read from
	// the release lock at its head.
	Digest  string `json:"digest,omitempty"`
	Version string `json:"version,omitempty"`
}

// DeliveryEnvNow is one environment now.
type DeliveryEnvNow struct {
	Env     string `json:"env"`
	Group   string `json:"group"`
	Digest  string `json:"digest,omitempty"`
	Tag     string `json:"tag,omitempty"`
	Version string `json:"version,omitempty"`
	// State is passed (gates passed), running (a gate runs), failed, waiting
	// (a newer bundle is proposed), or unknown.
	State string `json:"state"`
	Gate  string `json:"gate,omitempty"`
	// Since is when the active promotion merged; PassedAt when its gates passed.
	Since     string `json:"since,omitempty"`
	PassedAt  string `json:"passedAt,omitempty"`
	Attempt   int    `json:"attempt,omitempty"`
	AttemptOf int    `json:"attemptOf,omitempty"`
	// ProposedVersion is the newer bundle proposed while waiting.
	ProposedVersion string `json:"proposedVersion,omitempty"`
	ProposedDigest  string `json:"proposedDigest,omitempty"`
	// WaitingFor names the dependencies that have not passed the proposed bundle.
	WaitingFor []string `json:"waitingFor,omitempty"`
}

// DeliveryNextEvent is one expected event, in order (FLT-DLV-23).
type DeliveryNextEvent struct {
	Kind       string   `json:"kind"`
	Components []string `json:"components,omitempty"`
	Version    string   `json:"version,omitempty"`
	Digest     string   `json:"digest,omitempty"`
	Envs       []string `json:"envs,omitempty"`
	// After names the environments whose gates must pass first (promote).
	After []string `json:"after,omitempty"`
	// Changes are the changes that will ride the expected bundle
	// (expecting-bundle).
	Changes []DeliveryCommitRef `json:"changes,omitempty"`
	// State is the Renovate state of the bundle, and PR its pull request when
	// one proposes it (renovate).
	State string `json:"state,omitempty"`
	PR    int    `json:"pr,omitempty"`
	// At is the expected time; empty when estimates do not allow one.
	At string `json:"at,omitempty"`
}

// DeliveryRestart says that a newer bundle is expected to restart the gate a
// change's group stage waits on (FLT-DLV-10, FLT-DLV-22).
type DeliveryRestart struct {
	Components []string `json:"components,omitempty"`
	Version    string   `json:"version,omitempty"`
	Digest     string   `json:"digest,omitempty"`
	Envs       []string `json:"envs"`
	// LandsAt is when the newer bundle is expected in the group.
	LandsAt string `json:"landsAt,omitempty"`
}

// landing is a newer bundle expected to reach environments.
type landing struct {
	version    string
	components []string
	digest     string
	lockAt     time.Time // zero when unknown
	known      bool      // lockAt is known (an expected or real time)
	train      int       // index in the train
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func secs(n int64) time.Duration { return time.Duration(n) * time.Second }

// trainBase is the newest bundle whose gates passed in every environment of
// the first group, or -1 (FLT-DLV-22).
func trainBase(in deliveryInput, pl placement, groups []envGroup) int {
	if in.promo == nil || len(groups) == 0 {
		return -1
	}
	base := -1
	for bi, b := range in.bundles {
		all := true
		for _, key := range groups[0].envs {
			if !passedBundle(in, key, b.Digest) {
				all = false
				break
			}
		}
		if all {
			base = bi
		}
	}
	return base
}

// passedBundle reports whether a promotion of the bundle passed its gates in
// the environment.
func passedBundle(in deliveryInput, key, digest string) bool {
	env := in.promo.Environments[key]
	if env.Active != nil && env.Active.Digest == digest {
		if o, _ := activeOutcome(env); o == OutcomePassed {
			return true
		}
	}
	for _, p := range in.history[key] {
		if p.digest == digest && p.outcome == OutcomePassed {
			return true
		}
	}
	return false
}

// buildTrain lists the bundles after the train base, then the inferred
// bundles of finished builds not in any bundle yet (FLT-DLV-22).
func buildTrain(in deliveryInput, pl placement, groups []envGroup, est map[string]PhaseEstimate) []DeliveryTrainBundle {
	base := trainBase(in, pl, groups)
	start := max(base+1, len(in.bundles)-trainLimit)
	out := []DeliveryTrainBundle{}
	for bi := start; bi < len(in.bundles); bi++ {
		b := in.bundles[bi]
		tb := DeliveryTrainBundle{
			Tag: b.Tag, Version: b.Version, Digest: b.Digest,
			PublishedAt: fmtTime(bundlePublished(in, b)), Locked: in.lockedSet[b.Digest],
			Changed: []DeliveryComponentRev{}, Active: []string{}, Proposed: []string{}, Replaced: []string{}, Restarts: []string{},
		}
		if lc, ok := lockCommitOf(in, b.Digest); ok {
			tb.LockedAt = lc.Date
		}
		if bi > 0 {
			prev := in.bundles[bi-1]
			for _, k := range sortedKeys(b.Components) {
				if prev.Components[k] != b.Components[k] {
					tb.Changed = append(tb.Changed, DeliveryComponentRev{Component: shortComponent(k, in.appName), Revision: shortSHA7(b.Components[k])})
				}
			}
			tb.Same = len(tb.Changed) == 0 && len(prev.Components) > 0
		}
		if in.promo != nil {
			for _, key := range in.promo.Order {
				env := in.promo.Environments[key]
				switch {
				case env.Active != nil && env.Active.Digest == b.Digest:
					tb.Active = append(tb.Active, key)
				case env.Proposed != nil && env.Proposed.Digest == b.Digest:
					tb.Proposed = append(tb.Proposed, key)
				}
				for _, p := range in.history[key] {
					if p.digest == b.Digest && p.outcome == OutcomeReplaced && !slices.Contains(tb.Replaced, key) {
						tb.Replaced = append(tb.Replaced, key)
					}
				}
			}
		}
		out = append(out, tb)
	}
	return append(out, inferredBundles(in, pl, est)...)
}

// inferredBundles are the finished component builds on main newer than every
// known bundle's revision of the component: their bundle is releasing. One
// entry per commit (FLT-DLV-22).
func inferredBundles(in deliveryInput, pl placement, est map[string]PhaseEstimate) []DeliveryTrainBundle {
	// newest bundle revision index per component (lower is newer)
	newestRev := map[string]int{}
	for _, b := range in.bundles {
		for k, rev := range b.Components {
			j, ok := pl.commitIdx[rev]
			if !ok {
				j = len(in.commits)
			}
			if cur, seen := newestRev[k]; !seen || j < cur {
				newestRev[k] = j
			}
		}
	}
	type pending struct {
		comps          []string
		built, policed string
		policyDone     bool
	}
	byCommit := map[int]*pending{}
	seen := map[string]bool{}
	for j := range in.commits {
		runs, ok := in.checks[in.commits[j].SHA]
		if !ok {
			continue
		}
		builds, policies := classifyChecks(runs)
		for _, b := range builds {
			if !pl.bundled[b.component] || seen[b.component] || checkState(b.run) != StagePassed {
				continue
			}
			seen[b.component] = true // the newest passed build of the component
			if r, ok := newestRev[b.component]; ok && r <= j {
				continue // a bundle already carries this build or a newer one
			}
			p := byCommit[j]
			if p == nil {
				p = &pending{policyDone: true}
				byCommit[j] = p
			}
			p.comps = append(p.comps, shortComponent(b.component, in.appName))
			p.built = max(p.built, b.run.CompletedAt)
			pst, _, _, pfin := aggregate(policiesOf(policies, b.component), "")
			if pst != StagePassed && pst != StageNotNeeded {
				p.policyDone = false
			} else {
				p.policed = max(p.policed, pfin)
			}
		}
	}
	idx := make([]int, 0, len(byCommit))
	for j := range byCommit {
		idx = append(idx, j)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(idx))) // oldest commit first
	out := []DeliveryTrainBundle{}
	for _, j := range idx {
		p := byCommit[j]
		sort.Strings(p.comps)
		tb := DeliveryTrainBundle{
			Inferred: true, BuiltAt: p.built, Changed: []DeliveryComponentRev{},
			Active: []string{}, Proposed: []string{}, Replaced: []string{}, Restarts: []string{},
		}
		for _, c := range p.comps {
			tb.Changed = append(tb.Changed, DeliveryComponentRev{Component: c, Revision: shortSHA7(in.commits[j].SHA)})
		}
		if p.policyDone {
			tb.PolicyAt = p.policed
			if e, ok := est[stageRelease]; ok {
				if t, ok := parseTime(p.policed); ok {
					tb.ExpectedAt = fmtTime(t.Add(secs(e.MedianSeconds)))
				}
			}
		}
		out = append(out, tb)
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// lockCommitOf is the oldest lock commit of a bundle on GitOps main.
func lockCommitOf(in deliveryInput, digest string) (LockCommit, bool) {
	for i := len(in.lockCommits) - 1; i >= 0; i-- {
		if in.lockCommits[i].Digest == digest {
			return in.lockCommits[i], true
		}
	}
	return LockCommit{}, false
}

// landings are the train bundles newer than what the first environments run,
// with their expected (or real) GitOps lock time.
func landings(in deliveryInput, train []DeliveryTrainBundle, est map[string]PhaseEstimate) []landing {
	var out []landing
	gitops, okG := est[stageGitOps]
	for i, tb := range train {
		l := landing{version: tb.Version, digest: tb.Digest, train: i}
		for _, c := range tb.Changed {
			l.components = append(l.components, c.Component)
		}
		switch {
		case tb.LockedAt != "":
			l.lockAt, l.known = mustTime(tb.LockedAt), true
		case tb.PublishedAt != "" && okG:
			l.lockAt, l.known = mustTime(tb.PublishedAt).Add(secs(gitops.MedianSeconds)), true
		case tb.ExpectedAt != "" && okG:
			l.lockAt, l.known = mustTime(tb.ExpectedAt).Add(secs(gitops.MedianSeconds)), true
		}
		out = append(out, l)
	}
	return out
}

func mustTime(s string) time.Time {
	t, _ := parseTime(s)
	return t
}

// gateEnd is when an environment's running gate is expected to pass: its
// promotion time plus its estimate. ok is false without either.
func gateEnd(in deliveryInput, est map[string]PhaseEstimate, key string) (time.Time, bool) {
	since := activeSince(in, key)
	e, ok := est[envPhase(key)]
	if since.IsZero() || !ok {
		return time.Time{}, false
	}
	return since.Add(secs(e.MedianSeconds)), true
}

// activeSince is when the environment's active promotion merged.
func activeSince(in deliveryInput, key string) time.Time {
	env := in.promo.Environments[key]
	for _, p := range in.history[key] {
		if isActive(p.SHA, env) {
			return mustTime(p.MergedAt)
		}
	}
	return time.Time{}
}

// activeAttempt is the running gate's attempt on the active promotion.
func activeAttempt(in deliveryInput, key string) (int, int) {
	env := in.promo.Environments[key]
	for _, p := range in.history[key] {
		if isActive(p.SHA, env) {
			return p.attempt, p.attemptOf
		}
	}
	return 0, 0
}

// arriveOffset is how long after a GitOps lock a new bundle reaches the
// environment: the latest offset of its dependencies' gates along the graph.
// ok is false when an estimate is missing.
func arriveOffset(in deliveryInput, est map[string]PhaseEstimate, key string) (time.Duration, bool) {
	offsets := envOffsets(est, in, func(e PhaseEstimate) int64 { return e.MedianSeconds })
	var start int64
	for _, d := range in.promo.Environments[key].DependsOn {
		v, ok := offsets[d]
		if !ok {
			return 0, false
		}
		start = max(start, v)
	}
	return secs(start), true
}

// restarts finds, per environment with a running gate, the first newer
// bundle expected to land there before the gate ends (FLT-DLV-10.07).
func restarts(in deliveryInput, train []DeliveryTrainBundle, est map[string]PhaseEstimate) map[string]landing {
	out := map[string]landing{}
	if in.promo == nil {
		return out
	}
	lands := landings(in, train, est)
	for _, key := range in.promo.Order {
		env := in.promo.Environments[key]
		if o, _ := activeOutcome(env); o != OutcomeRunning || env.Active == nil {
			continue
		}
		end, ok := gateEnd(in, est, key)
		if !ok {
			continue
		}
		off, ok := arriveOffset(in, est, key)
		if !ok {
			continue
		}
		activeIdx := -1
		for i, tb := range train {
			if tb.Digest != "" && tb.Digest == env.Active.Digest {
				activeIdx = i
			}
		}
		for _, l := range lands {
			if l.train <= activeIdx || !l.known {
				continue
			}
			at := l.lockAt.Add(off)
			if at.Before(in.now) {
				at = in.now
			}
			if at.Before(end) {
				l.lockAt = at // reuse as the landing time here
				out[key] = l
				break
			}
		}
	}
	return out
}

type runningCheck struct {
	componentCheck
	kind string
}

// runningChecks are a commit's open checks that its stages use: component
// builds, and the policy checks of components built on that commit. Policy
// checks of merge-queue builds and group checks (such as "pr group") are not a
// stage of the change, so they are not in flight for it (FLT-DLV-05,
// FLT-DLV-23).
func runningChecks(runs []CheckRun) []runningCheck {
	builds, policies := classifyChecks(runs)
	built := map[string]bool{}
	var out []runningCheck
	for _, b := range builds {
		built[b.component] = true
		if checkState(b.run) == StageRunning {
			out = append(out, runningCheck{b, stageBuild})
		}
	}
	for _, p := range policies {
		if built[p.component] && checkState(p.run) == StageRunning {
			out = append(out, runningCheck{p, stagePolicy})
		}
	}
	return out
}

// buildNow assembles the Pipeline now panel's data (FLT-DLV-23) and marks the
// train bundles and changes a restart affects.
func buildNow(in deliveryInput, pl placement, groups []envGroup, est map[string]PhaseEstimate, train []DeliveryTrainBundle, changes []DeliveryChange) DeliveryNow {
	now := DeliveryNow{Checks: []DeliveryNowCheck{}, Environments: []DeliveryEnvNow{}, Next: []DeliveryNextEvent{}}
	if len(in.commits) > 0 {
		c := in.commits[0]
		pr, title := splitSubject(c.Message)
		now.AppHead = &DeliveryAppHead{SHA: c.SHA, ShortSHA: shortSHA7(c.SHA), PR: pr, Title: title, MergedAt: c.MergedAt}
	}
	if in.gitopsKnown && len(in.lockCommits) > 0 {
		lc := in.lockCommits[0]
		if bi, ok := pl.digests[lc.Digest]; ok {
			b := in.bundles[bi]
			h := &DeliveryGitOpsHead{Digest: b.Digest, Tag: b.Tag, Version: b.Version, PublishedAt: fmtTime(bundlePublished(in, b))}
			if bi > 0 && sameComponents(in.bundles[bi-1].Components, b.Components) {
				h.SameAs = in.bundles[bi-1].Digest
			}
			if first, ok := lockCommitOf(in, lc.Digest); ok {
				h.LockedAt = first.Date
				if n, _, ok := extractPR(first.Message); ok {
					h.PR = n
				}
			}
			now.GitOpsHead = h
		}
	}
	if in.bundlePR != nil {
		now.BundlePR = &DeliveryNowPR{Number: in.bundlePR.Number, URL: in.bundlePR.URL, Checks: in.bundlePR.Checks, CreatedAt: in.bundlePR.CreatedAt, Digest: in.bundlePRDig}
		if bi, ok := pl.digests[in.bundlePRDig]; ok {
			now.BundlePR.Version = in.bundles[bi].Version
		}
	}
	for j, c := range in.commits {
		runs, ok := in.checks[c.SHA]
		if !ok {
			continue
		}
		pr, _ := splitSubject(c.Message)
		if isDelivered(in, pl, groups, changeFirstBundle(in, pl, j, pr)) {
			// A delivered change has nothing in flight: a check still open on
			// it is stuck or from a cached read, and changes nothing shipped.
			continue
		}
		for _, cc := range runningChecks(runs) {
			now.Checks = append(now.Checks, DeliveryNowCheck{Kind: cc.kind, Component: shortComponent(cc.component, in.appName), StartedAt: cc.run.StartedAt, ShortSHA: shortSHA7(c.SHA), PR: pr})
		}
	}
	sort.Slice(now.Checks, func(a, b int) bool {
		x, y := now.Checks[a], now.Checks[b]
		if x.Kind != y.Kind {
			return x.Kind < y.Kind
		}
		return x.Component < y.Component
	})

	rs := restarts(in, train, est)
	if in.promo != nil {
		group := map[string]string{}
		for _, g := range groups {
			for _, key := range g.envs {
				group[key] = g.name
			}
		}
		for _, key := range in.promo.Order {
			env := in.promo.Environments[key]
			en := DeliveryEnvNow{Env: key, Group: group[key], State: StageUnknown}
			if env.Active != nil {
				en.Digest, en.Tag, en.Version = env.Active.Digest, env.Active.Tag, env.Active.Version
				if bi, ok := pl.digests[env.Active.Digest]; ok {
					en.Tag, en.Version = in.bundles[bi].Tag, in.bundles[bi].Version
				}
				en.Since = fmtTime(activeSince(in, key))
				switch o, gate := activeOutcome(env); o {
				case OutcomePassed:
					en.State = StagePassed
					for _, p := range in.history[key] {
						if isActive(p.SHA, env) {
							en.PassedAt = p.gatesAt
						}
					}
				case OutcomeFailed:
					en.State, en.Gate = StageFailed, gate
				case OutcomeRunning:
					en.State, en.Gate = StageRunning, gate
					en.Attempt, en.AttemptOf = activeAttempt(in, key)
				}
			}
			if env.Proposed != nil && (env.Active == nil || env.Proposed.Digest != env.Active.Digest) && en.State != StageRunning && en.State != StageFailed {
				en.State, en.ProposedVersion, en.ProposedDigest = StageWaiting, env.Proposed.Version, env.Proposed.Digest
				if bi, ok := pl.digests[env.Proposed.Digest]; ok {
					en.ProposedVersion = in.bundles[bi].Version
					for _, d := range env.DependsOn {
						if !depPassed(in, pl, d, bi) {
							en.WaitingFor = append(en.WaitingFor, d)
						}
					}
				}
			}
			now.Environments = append(now.Environments, en)
		}
	}

	// Next events, in order (FLT-DLV-23).
	now.Next = append(now.Next, expectingBundles(in, changes)...)
	now.Next = append(now.Next, renovateEvents(in, train, est)...)
	for _, key := range in.promo.OrderOrNil() {
		if l, ok := rs[key]; ok {
			now.Next = append(now.Next, DeliveryNextEvent{Kind: nextRestart, Components: l.components, Version: l.version, Digest: l.digest, Envs: []string{key}, At: fmtTime(l.lockAt)})
			train[l.train].Restarts = append(train[l.train].Restarts, key)
		}
	}
	now.Next = append(now.Next, promoteEvents(in, est, groups, now.Environments)...)
	sort.SliceStable(now.Next, func(a, b int) bool {
		x, y := now.Next[a].At, now.Next[b].At
		return x != "" && y != "" && x < y
	})

	markRestarts(in, groups, rs, changes)
	return now
}

// OrderOrNil returns the promotion order, or nil without a payload.
func (p *PromotionPayload) OrderOrNil() []string {
	if p == nil {
		return nil
	}
	return p.Order
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// depPassed reports whether the dependency passed a bundle at index bi or newer.
func depPassed(in deliveryInput, pl placement, key string, bi int) bool {
	env := in.promo.Environments[key]
	if env.Active == nil {
		return false
	}
	ai, ok := pl.digests[env.Active.Digest]
	if !ok || ai < bi {
		return false
	}
	o, _ := activeOutcome(env)
	return o == OutcomePassed
}

// promoteEvents are the waiting environments' expected promotions, one per
// group: after the dependencies' gates pass.
func promoteEvents(in deliveryInput, est map[string]PhaseEstimate, groups []envGroup, envs []DeliveryEnvNow) []DeliveryNextEvent {
	byEnv := map[string]DeliveryEnvNow{}
	for _, e := range envs {
		byEnv[e.Env] = e
	}
	var out []DeliveryNextEvent
	for _, g := range groups {
		ev := DeliveryNextEvent{Kind: nextPromote}
		var at time.Time
		known := true
		for _, key := range g.envs {
			e := byEnv[key]
			if e.State != StageWaiting || len(e.WaitingFor) == 0 {
				continue
			}
			ev.Envs = append(ev.Envs, key)
			for _, d := range e.WaitingFor {
				if !slices.Contains(ev.After, d) {
					ev.After = append(ev.After, d)
				}
				end, ok := gateEnd(in, est, d)
				if !ok || byEnv[d].State != StageRunning {
					known = false
					continue
				}
				if end.After(at) {
					at = end
				}
			}
		}
		if len(ev.Envs) == 0 {
			continue
		}
		ev.After = orderEnvs(in, ev.After)
		if known && !at.IsZero() {
			if at.Before(in.now) {
				at = in.now
			}
			ev.At = fmtTime(at)
		}
		out = append(out, ev)
	}
	return out
}

// markRestarts sets Restart on each change's group stage whose running gate a
// newer bundle is expected to restart (FLT-DLV-10.07).
func markRestarts(in deliveryInput, groups []envGroup, rs map[string]landing, changes []DeliveryChange) {
	if len(rs) == 0 {
		return
	}
	for ci := range changes {
		ch := &changes[ci]
		if ch.Delivered {
			continue
		}
		for si := range ch.Stages {
			s := &ch.Stages[si]
			if s.State != StageRunning {
				continue
			}
			for _, g := range groups {
				if s.Key != groupPhase(g.name) {
					continue
				}
				var r *DeliveryRestart
				for _, key := range g.envs {
					l, ok := rs[key]
					if !ok || !envRunningFor(ch, key) {
						continue
					}
					if r == nil {
						r = &DeliveryRestart{Components: l.components, Version: l.version, Digest: l.digest, LandsAt: fmtTime(l.lockAt)}
					}
					r.Envs = append(r.Envs, key)
				}
				s.Restart = r
			}
		}
	}
}

// envRunningFor reports whether the change's state in the environment is a
// running gate.
func envRunningFor(ch *DeliveryChange, key string) bool {
	for _, e := range ch.Envs {
		if e.Env == key {
			return e.State == StageRunning
		}
	}
	return false
}

// expectingBundles are the expected bundles of the listed changes whose rebuilt
// components no published bundle contains yet (FLT-DLV-23). Changes whose
// failed builds a later build superseded ride that build's bundle, so they
// share its line. The time is now plus the remaining Build, Policy check, and
// Release estimates; it is omitted when one is unknown.
func expectingBundles(in deliveryInput, changes []DeliveryChange) []DeliveryNextEvent {
	var out []DeliveryNextEvent
	index := map[string]int{}
	// Oldest change first: its bundle comes first.
	for ci := len(changes) - 1; ci >= 0; ci-- {
		ch := changes[ci]
		if ch.Delivered || ch.Bundle != nil || len(ch.Builds) == 0 || len(ch.Stages) < 4 {
			continue
		}
		build, release := ch.Stages[1], ch.Stages[3]
		switch {
		case build.State == StageFailed || build.State == StageNotNeeded || build.State == StageUnknown:
			continue
		case release.State == StageNotNeeded || release.State == StageUnknown || release.Reason == reasonPartlyReleased:
			continue
		}
		key := ch.SHA
		if build.RebuiltIn != nil {
			key = build.RebuiltIn.SHA
		}
		at, known := remainingToBundle(ch.Stages[1:4], in.now)
		ref := DeliveryCommitRef{SHA: ch.SHA, ShortSHA: ch.ShortSHA, PR: ch.PR}
		if k, ok := index[key]; ok {
			ev := &out[k]
			ev.Changes = append(ev.Changes, ref)
			for _, c := range build.Components {
				if !slices.Contains(ev.Components, c) {
					ev.Components = append(ev.Components, c)
				}
			}
			if !known {
				ev.At = ""
			} else if ev.At != "" && fmtTime(at) > ev.At {
				ev.At = fmtTime(at)
			}
			continue
		}
		ev := DeliveryNextEvent{Kind: nextExpecting, Changes: []DeliveryCommitRef{ref}, Components: slices.Clone(build.Components)}
		if known {
			ev.At = fmtTime(at)
		}
		index[key] = len(out)
		out = append(out, ev)
	}
	return out
}

// remainingToBundle is when the stages up to Release are expected to finish:
// now plus the rest of the current stage and the estimates of the later ones.
func remainingToBundle(stages []DeliveryStage, now time.Time) (time.Time, bool) {
	var rest time.Duration
	for _, s := range stages {
		switch s.State {
		case StagePassed, StageNotNeeded:
			continue
		}
		if s.EstimateSeconds == nil {
			return time.Time{}, false
		}
		est := secs(*s.EstimateSeconds)
		if s.State == StageRunning || s.State == StageWaiting {
			start, ok := parseTime(s.StartedAt)
			if !ok {
				return time.Time{}, false
			}
			rest += max(0, start.Add(est).Sub(now))
			continue
		}
		rest += est
	}
	return now.Add(rest), true
}

// renovateEvents are the published bundles newer than the newest locked one,
// each with its Renovate state and expected lock time: the publication time,
// or the opening time of the pull request that proposes it, plus the Renovate
// estimate (FLT-DLV-07, FLT-DLV-23).
func renovateEvents(in deliveryInput, train []DeliveryTrainBundle, est map[string]PhaseEstimate) []DeliveryNextEvent {
	newestLocked := -1
	for i, tb := range train {
		if tb.Locked {
			newestLocked = i
		}
	}
	e, okE := est[stageGitOps]
	var out []DeliveryNextEvent
	for i := newestLocked + 1; i < len(train); i++ {
		tb := train[i]
		if tb.Inferred || tb.Locked || tb.Digest == "" {
			continue
		}
		ev := DeliveryNextEvent{Kind: nextRenovate, Version: tb.Version, Digest: tb.Digest, State: renovateWaiting}
		for _, c := range tb.Changed {
			ev.Components = append(ev.Components, c.Component)
		}
		base := tb.PublishedAt
		if pr := in.bundlePR; pr != nil && in.bundlePRDig == tb.Digest {
			ev.PR = pr.Number
			switch pr.Checks {
			case "failed":
				ev.State = renovateFailed
			case "passed":
				ev.State = renovatePassed
			default:
				ev.State = renovateRunning
			}
			if pr.CreatedAt != "" {
				base = pr.CreatedAt
			}
		}
		if t, ok := parseTime(base); ok && okE && ev.State != renovateFailed {
			ev.At = fmtTime(maxTime(t.Add(secs(e.MedianSeconds)), in.now))
		}
		out = append(out, ev)
	}
	return out
}
