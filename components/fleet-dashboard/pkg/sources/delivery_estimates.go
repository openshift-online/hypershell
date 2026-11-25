package sources

import (
	"sort"
	"time"
)

// Phase estimates (fleet-dashboard-delivery-pipeline.spec FLT-DLV-09). Each
// phase's estimate is the median duration of its last 5 completed runs, and its
// slowest value the longest of those runs. A run is one execution: one
// component's build or policy check, one bundle's release or GitOps update,
// one passed promotion in one environment. Many changes ride one
// bundle, so a bundle is sampled once, not once per change.

// estimateRuns is how many of the latest runs an estimate looks at.
const estimateRuns = 5

// overdueMargin is added to the slowest of the last runs before a phase in
// progress is Overdue (FLT-DLV-04).
const overdueMargin = 5 * time.Minute

// PhaseEstimate is one phase's estimate (FLT-DLV-09).
type PhaseEstimate struct {
	MedianSeconds  int64 `json:"medianSeconds"`
	SlowestSeconds int64 `json:"slowestSeconds"`
	Samples        int   `json:"samples"`
}

// phaseRun is one completed run of a phase.
type phaseRun struct {
	end time.Time
	dur time.Duration
}

// Phase keys of the estimate map. Build and policy check are per component.
func buildPhase(component string) string  { return stageBuild + ":" + component }
func policyPhase(component string) string { return stagePolicy + ":" + component }
func groupPhase(group string) string      { return "env:" + group }
func envPhase(env string) string          { return "environment:" + env }

func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	return t, err == nil
}

// phaseSamples collects the completed runs of every phase from the resolved
// inputs: the check runs of the listed changes, the known bundles, the GitOps
// lock timeline, and the environment histories.
func phaseSamples(in deliveryInput, pl placement, groups []envGroup) map[string][]phaseRun {
	out := map[string][]phaseRun{}
	add := func(key string, start, end time.Time) {
		if start.IsZero() || end.IsZero() || end.Before(start) {
			return
		}
		out[key] = append(out[key], phaseRun{end: end, dur: end.Sub(start)})
	}

	merged := map[string]time.Time{}
	for _, c := range in.commits {
		if t, ok := parseTime(c.MergedAt); ok {
			merged[c.SHA] = t
		}
	}

	// Build and policy check: one run per component check run. A component
	// built more than once on a commit (a re-run) is not sampled.
	for sha, runs := range in.checks {
		start, ok := merged[sha]
		if !ok {
			continue
		}
		builds, policies := classifyChecks(runs)
		count := map[string]int{}
		for _, b := range builds {
			if checkState(b.run) != StageNotNeeded {
				count[b.component]++
			}
		}
		buildEnd := map[string]time.Time{}
		for _, b := range builds {
			if count[b.component] != 1 || checkState(b.run) != StagePassed {
				continue
			}
			end, ok := parseTime(b.run.CompletedAt)
			if !ok {
				continue
			}
			buildEnd[b.component] = end
			add(buildPhase(b.component), start, end)
		}
		for _, p := range policies {
			bEnd, ok := buildEnd[p.component]
			if !ok || checkState(p.run) != StagePassed {
				continue
			}
			if end, ok := parseTime(p.run.CompletedAt); ok {
				add(policyPhase(p.component), bEnd, end)
			}
		}
	}

	// Release and GitOps update: one run per bundle. A group: one run per
	// bundle's promotion through it.
	for _, b := range in.bundles {
		published := bundlePublished(in, b)
		if releaseStart := policiesDone(in, b.ManifestsRev); !releaseStart.IsZero() {
			add(stageRelease, releaseStart, published)
		}
		var locked time.Time
		for i := len(in.lockCommits) - 1; i >= 0; i-- {
			if in.lockCommits[i].Digest == b.Digest {
				locked, _ = parseTime(in.lockCommits[i].Date)
				break
			}
		}
		add(stageGitOps, published, locked)
	}

	// Environment: one run per passed promotion, from its merge on the
	// environment branch to its gates' pass. Groups are not sampled: their
	// estimate follows the dependsOn graph (FLT-DLV-09).
	for key, promos := range in.history {
		for _, p := range promos {
			if p.gatesAt == "" || p.rerun {
				continue
			}
			start, ok1 := parseTime(p.MergedAt)
			end, ok2 := parseTime(p.gatesAt)
			if ok1 && ok2 {
				add(envPhase(key), start, end)
			}
		}
	}
	return out
}

// groupEstimates derives each environment group's estimate from the
// environment estimates along the dependsOn graph (FLT-DLV-09, FLT-DLV-10).
// An environment finishes its estimate after the latest of its dependencies;
// a group ends when its last environment does. A group's estimate is the time
// it adds after the groups before it, so the UI's sum of later phases reaches
// it. A group with an environment without an estimate has none.
func groupEstimates(est map[string]PhaseEstimate, in deliveryInput, groups []envGroup) {
	if in.promo == nil {
		return
	}
	median := envOffsets(est, in, func(e PhaseEstimate) int64 { return e.MedianSeconds })
	slowest := envOffsets(est, in, func(e PhaseEstimate) int64 { return e.SlowestSeconds })
	var prevM, prevS int64
	for _, g := range groups {
		var endM, endS int64
		samples, ok := 0, true
		for _, key := range g.envs {
			m, okm := median[key]
			sl, oks := slowest[key]
			if !okm || !oks {
				ok = false
				break
			}
			endM, endS = max(endM, m), max(endS, sl)
			if n := est[envPhase(key)].Samples; samples == 0 || n < samples {
				samples = n
			}
		}
		if !ok {
			// Later groups cannot be reached by a known time either.
			return
		}
		est[groupPhase(g.name)] = PhaseEstimate{MedianSeconds: max(0, endM-prevM), SlowestSeconds: max(0, endS-prevS), Samples: samples}
		prevM, prevS = max(prevM, endM), max(prevS, endS)
	}
}

// envOffsets is, per environment, the time from the GitOps lock to its gates'
// pass along the dependsOn graph: the latest offset of its dependencies plus
// its own estimate. An environment without an estimate, or that depends on
// one, is absent.
func envOffsets(est map[string]PhaseEstimate, in deliveryInput, pick func(PhaseEstimate) int64) map[string]int64 {
	out := map[string]int64{}
	state := map[string]int{} // 1 visiting, 2 done
	var visit func(key string) (int64, bool)
	visit = func(key string) (int64, bool) {
		switch state[key] {
		case 1:
			return 0, false // a cycle: no estimate
		case 2:
			v, ok := out[key]
			return v, ok
		}
		state[key] = 1
		defer func() { state[key] = 2 }()
		e, ok := est[envPhase(key)]
		if !ok {
			return 0, false
		}
		var start int64
		for _, d := range in.promo.Environments[key].DependsOn {
			v, ok := visit(d)
			if !ok {
				return 0, false
			}
			start = max(start, v)
		}
		out[key] = start + pick(e)
		return out[key], true
	}
	for _, key := range in.promo.Order {
		visit(key)
	}
	return out
}

// bundlePublished is a bundle's publication time: the registry's, or the time
// in its tag.
func bundlePublished(in deliveryInput, b *Release) time.Time {
	if t := in.published[b.Tag]; !t.IsZero() {
		return t
	}
	t, _ := parseTime(b.Date)
	return t
}

// policiesDone is when the release of a bundle could start: the latest passed
// policy check on the bundle's manifests revision commit, or zero when unknown.
func policiesDone(in deliveryInput, rev string) time.Time {
	var latest time.Time
	_, policies := classifyChecks(in.checks[rev])
	for _, p := range policies {
		if checkState(p.run) != StagePassed {
			continue
		}
		if t, ok := parseTime(p.run.CompletedAt); ok && t.After(latest) {
			latest = t
		}
	}
	return latest
}

// estimates turns samples into phase estimates from the last 5 runs that ended
// by now. A phase with fewer than 3 runs has none (FLT-DLV-09).
func estimates(samples map[string][]phaseRun, now time.Time) map[string]PhaseEstimate {
	out := map[string]PhaseEstimate{}
	for key, runs := range samples {
		done := make([]phaseRun, 0, len(runs))
		for _, r := range runs {
			if !r.end.After(now) {
				done = append(done, r)
			}
		}
		sort.Slice(done, func(i, j int) bool { return done[i].end.Before(done[j].end) })
		if len(done) > estimateRuns {
			done = done[len(done)-estimateRuns:]
		}
		if len(done) < typicalMinSamples {
			continue
		}
		secs := make([]int64, len(done))
		var slowest int64
		for i, r := range done {
			secs[i] = int64(r.dur.Seconds())
			if secs[i] > slowest {
				slowest = secs[i]
			}
		}
		out[key] = PhaseEstimate{MedianSeconds: median(secs), SlowestSeconds: slowest, Samples: len(done)}
	}
	return out
}

// estimateComponents are the components whose runs decide a change's Build and
// Policy check estimate: the rebuilt bundled components, or every built
// component when it rebuilt none (FLT-DLV-09).
func estimateComponents(runs []CheckRun, bundled map[string]bool) []string {
	if rebuilt := rebuiltComponents(runs, bundled); len(rebuilt) > 0 {
		return rebuilt
	}
	builds, _ := classifyChecks(runs)
	var out []string
	for _, b := range builds {
		if checkState(b.run) != StageNotNeeded {
			out = append(out, b.component)
		}
	}
	return out
}

// stageEstimate resolves one stage's estimate for a change: per component for
// Build and Policy check (the slowest component decides), per phase otherwise.
func stageEstimate(est map[string]PhaseEstimate, key string, components []string) (PhaseEstimate, bool) {
	if key != stageBuild && key != stagePolicy {
		e, ok := est[key]
		return e, ok
	}
	if len(components) == 0 {
		return PhaseEstimate{}, false
	}
	var out PhaseEstimate
	for _, c := range components {
		k := buildPhase(c)
		if key == stagePolicy {
			k = policyPhase(c)
		}
		e, ok := est[k]
		if !ok {
			return PhaseEstimate{}, false
		}
		out.MedianSeconds = max(out.MedianSeconds, e.MedianSeconds)
		out.SlowestSeconds = max(out.SlowestSeconds, e.SlowestSeconds)
		if out.Samples == 0 || e.Samples < out.Samples {
			out.Samples = e.Samples
		}
	}
	return out, true
}

// attachEstimates puts each stage's estimate on every listed change's stages.
func attachEstimates(in deliveryInput, pl placement, changes []DeliveryChange, est map[string]PhaseEstimate) {
	for i := range changes {
		comps := estimateComponents(in.checks[changes[i].SHA], pl.bundled)
		for j := range changes[i].Stages {
			s := &changes[i].Stages[j]
			if s.Key == stageMerged {
				continue
			}
			if e, ok := stageEstimate(est, s.Key, comps); ok {
				m, sl := e.MedianSeconds, e.SlowestSeconds
				s.EstimateSeconds, s.SlowestSeconds = &m, &sl
			}
		}
	}
}

// stageOverdue reports whether a Running or Waiting stage has run longer than
// its slowest of the last 5 runs plus the margin (FLT-DLV-04).
func stageOverdue(s DeliveryStage, now time.Time) bool {
	if (s.State != StageRunning && s.State != StageWaiting) || s.SlowestSeconds == nil {
		return false
	}
	start, ok := parseTime(s.StartedAt)
	if !ok {
		return false
	}
	return now.Sub(start) > time.Duration(*s.SlowestSeconds)*time.Second+overdueMargin
}
