package sources

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Promotion history per environment (fleet-dashboard-delivery-pipeline.spec
// FLT-DLV-18): what happened to each change in each environment at the time,
// read from the GitOps environment branches. Each merge commit on an
// environment branch is one promotion. Its bundle comes from the commit's
// hydration metadata (the shared lock resolver), and the outcome of a replaced
// promotion from the gate check runs on that same commit. The promoter's own
// history records are not used: each records the gate results of the commit it
// replaced, not its own.

// Promotion outcomes (FLT-DLV-18).
const (
	OutcomePassed   = "passed"
	OutcomeFailed   = "failed"
	OutcomeReplaced = "replaced"
	OutcomeRunning  = "running"
	OutcomeUnknown  = "unknown"
)

const srcHistory = "history"

// historySettleAfter is how long after its replacement a promotion whose gate
// never finished is kept as Replaced without reading its check runs again.
const historySettleAfter = time.Hour

// DeliveryPromotion is one promotion of a bundle that contains the change, in
// one environment.
type DeliveryPromotion struct {
	Tag     string `json:"tag,omitempty"`
	Version string `json:"version,omitempty"`
	Digest  string `json:"digest,omitempty"`
	// GitOpsPR is the GitOps update pull request that locked this bundle.
	GitOpsPR   int    `json:"gitopsPr,omitempty"`
	PromotedAt string `json:"promotedAt"`
	// URL is the promotion's own pull request on the environment branch.
	URL     string `json:"url,omitempty"`
	Outcome string `json:"outcome"`
	// Gate is the gate that failed, or that was still running.
	Gate string `json:"gate,omitempty"`
	// GatesPassedAt is when the last gate of a passed promotion completed: the
	// environment finished the change then (FLT-DLV-08). Empty when unknown.
	GatesPassedAt string `json:"gatesPassedAt,omitempty"`
}

// histPromotion is one cached promotion on an environment branch.
type histPromotion struct {
	EnvPromotion
	digest string
	// outcome and gate of a replaced promotion, from its own gate check runs.
	outcome string
	gate    string
	settled bool
	// gatesAt is the latest completion of a passed promotion's gate runs, and
	// rerun is true when a gate ran more than once on the commit, so its first
	// finish time is lost (FLT-DLV-09).
	gatesAt string
	rerun   bool
	timed   bool
	// attempt and attemptOf are the running gate's attempt on the active
	// promotion, read each refresh while it runs (FLT-DLV-23).
	attempt   int
	attemptOf int
}

type envHistory struct {
	branch string
	active string // the environment's active hydrated commit when last listed
	since  time.Time
	promos []histPromotion // oldest first
}

// readHistory refreshes every environment's promotion history and returns it
// keyed by environment. Only the refresh goroutine calls it, so the cache needs
// no lock. Environments are read in parallel; within one, a branch is listed
// again only when its active commit changed or the window reaches further back,
// and resolved bundles and settled outcomes are kept.
func (d *Delivery) readHistory(ctx context.Context, promo *PromotionPayload, since, now time.Time) (map[string][]histPromotion, DeliverySourceStatus) {
	if promo == nil {
		return d.historySnapshot(), status(d.histAt, "promotion data unavailable")
	}
	type result struct {
		key  string
		h    *envHistory
		errs []string
	}
	results := make(chan result, len(promo.Environments))
	var wg sync.WaitGroup
	for key, env := range promo.Environments {
		if env.Branch == "" {
			continue
		}
		wg.Add(1)
		go func(key string, env Environment, old *envHistory) {
			defer wg.Done()
			h, errs := d.readEnvHistory(ctx, env, old, since, now)
			results <- result{key, h, errs}
		}(key, env, d.hist[key])
	}
	wg.Wait()
	close(results)
	var errs []string
	for r := range results {
		if r.h != nil {
			d.hist[r.key] = r.h
		}
		errs = append(errs, r.errs...)
	}
	if len(errs) == 0 {
		d.histAt, d.histErr = now, ""
	} else {
		sort.Strings(errs)
		d.histErr = strings.Join(errs, "; ")
	}
	return d.historySnapshot(), status(d.histAt, d.histErr)
}

// readEnvHistory refreshes one environment's history. It works on a copy, so a
// failed read keeps the previous history intact.
func (d *Delivery) readEnvHistory(ctx context.Context, env Environment, old *envHistory, since, now time.Time) (*envHistory, []string) {
	var errs []string
	h := old
	listSince := since
	if d.historyDays > 0 {
		if deep := now.Add(-time.Duration(d.historyDays) * 24 * time.Hour); deep.Before(listSince) {
			listSince = deep
		}
	}
	if h == nil || h.branch != env.Branch || h.active != env.ActiveHydrated || listSince.Before(h.since) {
		list, err := d.gh.EnvPromotions(ctx, env.Branch, listSince)
		if err != nil {
			if h == nil {
				return nil, []string{err.Error()}
			}
			errs = append(errs, err.Error())
		} else {
			h = mergeHistory(h, env, listSince, list)
		}
	}
	h = &envHistory{branch: h.branch, active: h.active, since: h.since, promos: append([]histPromotion(nil), h.promos...)}
	keys := gateKeys(env)
	// Newest first: every promotion since the oldest listed change, then older
	// ones only until the environment holds enough passed promotions for its
	// estimate (FLT-DLV-09, FLT-DLV-18).
	passed := 0
	for i := len(h.promos) - 1; i >= 0; i-- {
		p := &h.promos[i]
		if t, ok := parseTime(p.MergedAt); ok && t.Before(since) && passed >= estimateRuns {
			break
		}
		if err := d.resolvePromotion(ctx, h, i, env, keys, now); err != "" {
			errs = append(errs, err)
		}
		if p.gatesAt != "" && !p.rerun {
			passed++
		}
	}
	return h, errs
}

// resolvePromotion fills one promotion's bundle, outcome, and gate times. It
// returns an error text, or "".
func (d *Delivery) resolvePromotion(ctx context.Context, h *envHistory, i int, env Environment, keys []string, now time.Time) string {
	p := &h.promos[i]
	if p.digest == "" && d.lock != nil {
		if r := d.lock.ResolveHydrated(ctx, p.SHA); r != nil {
			p.digest = r.Digest
		}
	}
	if isActive(p.SHA, env) {
		// The active promotion's gates come from the promotion payload. While a
		// gate runs, its check run names the attempt (FLT-DLV-23). Once they
		// passed, read their finish time once (FLT-DLV-08).
		o, gate := activeOutcome(env)
		p.attempt, p.attemptOf = 0, 0
		switch {
		case o == OutcomeRunning:
			runs, err := d.gh.GateRuns(ctx, p.SHA)
			if err != nil {
				return err.Error()
			}
			p.attempt, p.attemptOf = gateAttempt(runs, gate)
		case o == OutcomePassed && !p.timed:
			runs, err := d.gh.GateRuns(ctx, p.SHA)
			if err != nil {
				return err.Error()
			}
			// Read once: runs that do not show the pass leave the time unknown.
			if o, _ := gateOutcome(runs, keys); o == OutcomePassed {
				p.gatesAt, p.rerun = gateTimes(runs, keys)
			}
			p.timed = true
		}
		return ""
	}
	if p.settled || i == len(h.promos)-1 {
		return ""
	}
	runs, err := d.gh.GateRuns(ctx, p.SHA)
	if err != nil {
		return err.Error()
	}
	p.outcome, p.gate = gateOutcome(runs, keys)
	if p.outcome == OutcomePassed {
		p.gatesAt, p.rerun = gateTimes(runs, keys)
		p.timed = true
	}
	p.settled = p.outcome != OutcomeReplaced || replacedLongAgo(h.promos[i+1].MergedAt, now)
	return ""
}

// mergeHistory replaces a branch's promotion list, keeping what was already
// resolved for promotions it still lists.
func mergeHistory(old *envHistory, env Environment, since time.Time, newestFirst []EnvPromotion) *envHistory {
	known := map[string]histPromotion{}
	if old != nil && old.branch == env.Branch {
		for _, p := range old.promos {
			known[p.SHA] = p
		}
	}
	h := &envHistory{branch: env.Branch, active: env.ActiveHydrated, since: since}
	for i := len(newestFirst) - 1; i >= 0; i-- {
		p := newestFirst[i]
		if k, ok := known[p.SHA]; ok {
			h.promos = append(h.promos, k)
			continue
		}
		h.promos = append(h.promos, histPromotion{EnvPromotion: p})
	}
	return h
}

func (d *Delivery) historySnapshot() map[string][]histPromotion {
	out := make(map[string][]histPromotion, len(d.hist))
	for k, h := range d.hist {
		out[k] = append([]histPromotion(nil), h.promos...)
	}
	return out
}

// isActive reports whether a promotion's merge commit is the environment's
// active hydrated commit. The promotion payload carries a shortened commit ID,
// so the full ID is matched by prefix.
func isActive(sha string, env Environment) bool {
	return env.ActiveHydrated != "" && strings.HasPrefix(sha, env.ActiveHydrated)
}

func replacedLongAgo(replacedAt string, now time.Time) bool {
	t, err := time.Parse(time.RFC3339, replacedAt)
	return err == nil && now.Sub(t) > historySettleAfter
}

// gateKeys are the environment's gates: the commit statuses the promotion
// strategy defines for it, sorted for a stable report.
func gateKeys(env Environment) []string {
	keys := make([]string, 0, len(env.ActiveGates))
	for _, g := range env.ActiveGates {
		keys = append(keys, g.Key)
	}
	sort.Strings(keys)
	return keys
}

// latestRuns keeps the latest run per check name and counts the runs per name.
// GateRuns lists every run (re-runs included), in no promised order.
func latestRuns(runs []CheckRun) (map[string]CheckRun, map[string]int) {
	byName, count := map[string]CheckRun{}, map[string]int{}
	for _, r := range runs {
		count[r.Name]++
		if cur, ok := byName[r.Name]; !ok || r.StartedAt > cur.StartedAt {
			byName[r.Name] = r
		}
	}
	return byName, count
}

// gateTimes returns when a passed promotion's gates passed: each gate's first
// successful completion, and the latest of those across gates (FLT-DLV-08). A
// gate re-posted after it passed (a health check that reports again) does not
// move the time. rerun is true when a gate ran more than once before its first
// pass, so the promotion's duration includes a retry and is not sampled
// (FLT-DLV-09).
func gateTimes(runs []CheckRun, keys []string) (at string, rerun bool) {
	for _, k := range keys {
		first := ""
		for _, r := range runs {
			if r.Name == k && r.Status == "completed" && checkSucceeded(r.Conclusion) && r.CompletedAt != "" && (first == "" || r.CompletedAt < first) {
				first = r.CompletedAt
			}
		}
		before := 0
		for _, r := range runs {
			if r.Name == k && (first == "" || r.StartedAt <= first) {
				before++
			}
		}
		if before > 1 {
			rerun = true
		}
		if first > at {
			at = first
		}
	}
	return at, rerun
}

// gateAttempt reads "Attempt n of m" from the latest run of a gate: the
// release analysis names its attempt in the check run summary (FLT-DLV-23).
func gateAttempt(runs []CheckRun, gate string) (n, of int) {
	byName, _ := latestRuns(runs)
	r, ok := byName[gate]
	if !ok {
		return 0, 0
	}
	m := attemptText.FindStringSubmatch(r.Summary)
	if m == nil {
		return 0, 0
	}
	n, _ = strconv.Atoi(m[1])
	of, _ = strconv.Atoi(m[2])
	return n, of
}

var attemptText = regexp.MustCompile(`(?i)\battempt (\d{1,2}) of (\d{1,2})\b`)

// gateOutcome classifies a replaced promotion from the check runs named after
// each gate key on its own commit (the latest run per name).
func gateOutcome(runs []CheckRun, keys []string) (outcome, gate string) {
	byName, _ := latestRuns(runs)
	open := ""
	for _, k := range keys {
		r, ok := byName[k]
		switch {
		case !ok || r.Status != "completed" || checkIgnorable(r.Conclusion):
			if open == "" {
				open = k
			}
		case checkSucceeded(r.Conclusion):
		default:
			return OutcomeFailed, k
		}
	}
	if open != "" {
		return OutcomeReplaced, open
	}
	return OutcomePassed, ""
}

// activeOutcome classifies the active promotion from the promotion payload's
// live gate phases.
func activeOutcome(env Environment) (outcome, gate string) {
	running := ""
	for _, g := range env.ActiveGates {
		switch g.Phase {
		case "success":
		case "failure":
			return OutcomeFailed, g.Key
		default:
			if running == "" {
				running = g.Key
			}
		}
	}
	if running != "" {
		return OutcomeRunning, running
	}
	return OutcomePassed, ""
}

// envAttempts lists the promotions in one environment whose bundle contains
// the change, oldest first, up to and including the first that passed or the
// active one (FLT-DLV-18).
func envAttempts(in deliveryInput, pl placement, key string, first int) []DeliveryPromotion {
	if in.promo == nil || first < 0 {
		return nil
	}
	env := in.promo.Environments[key]
	var out []DeliveryPromotion
	for _, p := range in.history[key] {
		if !containsIdx(pl, p.digest, first) {
			continue
		}
		dp := DeliveryPromotion{Digest: p.digest, PromotedAt: p.MergedAt, URL: p.URL, GitOpsPR: lockPR(in, p.digest)}
		if b := in.bundles[pl.digests[p.digest]]; b != nil {
			dp.Tag, dp.Version = b.Tag, b.Version
		}
		active := isActive(p.SHA, env)
		switch {
		case active:
			dp.Outcome, dp.Gate = activeOutcome(env)
		case p.outcome != "":
			dp.Outcome, dp.Gate = p.outcome, p.gate
		default:
			dp.Outcome = OutcomeUnknown
		}
		if dp.Outcome == OutcomePassed {
			dp.GatesPassedAt = p.gatesAt
		}
		out = append(out, dp)
		if active || dp.Outcome == OutcomePassed {
			break
		}
	}
	return out
}

// lockPR is the GitOps update pull request that first locked a bundle.
func lockPR(in deliveryInput, digest string) int {
	for i := len(in.lockCommits) - 1; i >= 0; i-- {
		lc := in.lockCommits[i]
		if lc.Digest != digest {
			continue
		}
		if n, _, ok := extractPR(lc.Message); ok {
			return n
		}
		return 0
	}
	return 0
}

// failedAttempts counts the failed promotions across a group's environments.
func failedAttempts(envs []DeliveryEnvState) int {
	n := 0
	for _, e := range envs {
		for _, a := range e.History {
			if a.Outcome == OutcomeFailed {
				n++
			}
		}
	}
	return n
}
