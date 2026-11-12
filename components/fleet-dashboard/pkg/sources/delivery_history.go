package sources

import (
	"context"
	"sort"
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
}

// histPromotion is one cached promotion on an environment branch.
type histPromotion struct {
	EnvPromotion
	digest string
	// outcome and gate of a replaced promotion, from its own gate check runs.
	outcome string
	gate    string
	settled bool
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
	if h == nil || h.branch != env.Branch || h.active != env.ActiveHydrated || since.Before(h.since) {
		list, err := d.gh.EnvPromotions(ctx, env.Branch, since)
		if err != nil {
			if h == nil {
				return nil, []string{err.Error()}
			}
			errs = append(errs, err.Error())
		} else {
			h = mergeHistory(h, env, since, list)
		}
	}
	h = &envHistory{branch: h.branch, active: h.active, since: h.since, promos: append([]histPromotion(nil), h.promos...)}
	keys := gateKeys(env)
	for i := range h.promos {
		p := &h.promos[i]
		if p.digest == "" && d.lock != nil {
			if r := d.lock.ResolveHydrated(ctx, p.SHA); r != nil {
				p.digest = r.Digest
			}
		}
		if isActive(p.SHA, env) || p.settled || i == len(h.promos)-1 {
			continue // the active promotion's gates come from the promotion payload
		}
		runs, err := d.gh.GateRuns(ctx, p.SHA)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		p.outcome, p.gate = gateOutcome(runs, keys)
		p.settled = p.outcome != OutcomeReplaced || replacedLongAgo(h.promos[i+1].MergedAt, now)
	}
	return h, errs
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

// gateOutcome classifies a replaced promotion from the check runs named after
// each gate key on its own commit (the latest run per name).
func gateOutcome(runs []CheckRun, keys []string) (outcome, gate string) {
	byName := map[string]CheckRun{}
	for _, r := range runs {
		byName[r.Name] = r
	}
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
