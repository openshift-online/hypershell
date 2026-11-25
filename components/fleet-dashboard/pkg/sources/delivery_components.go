package sources

import "sort"

// Component-level delivery (fleet-dashboard-delivery-pipeline.spec FLT-DLV-05,
// FLT-DLV-22). An image is built from the whole repository at one commit, so a
// later successful build of a component carries every earlier change. And each
// finished build publishes its own bundle, so a change's components can reach
// a bundle one at a time.

// States of a component that is not in a bundle yet (FLT-DLV-06).
const (
	missingBuilding  = "building"
	missingPolicy    = "policy"
	missingReleasing = "releasing"
	missingFailed    = "failed"
)

// DeliveryCommitRef names an application commit.
type DeliveryCommitRef struct {
	SHA      string `json:"sha"`
	ShortSHA string `json:"shortSha"`
	PR       int    `json:"pr,omitempty"`
}

// DeliveryPartly counts a partly shipped change's components at one stage
// (FLT-DLV-22).
type DeliveryPartly struct {
	Done  int `json:"done"`
	Total int `json:"total"`
	// Missing names the components not in a bundle yet, with their state
	// (Release stage only).
	Missing []DeliveryMissing `json:"missing,omitempty"`
}

// DeliveryMissing is one component that has not reached a bundle.
type DeliveryMissing struct {
	Component string `json:"component"`
	State     string `json:"state"`
}

// componentRun is one component's effective build for a change: its own, or
// the later build that superseded a failed one (FLT-DLV-05).
type componentRun struct {
	build    componentCheck
	policies []componentCheck
	// by is the commit index of the superseding build, or -1 for the change's
	// own build.
	by int
}

// effectiveBuilds maps each component the change at commit index i built to
// its effective build. A failed build is superseded by the closest later
// commit on main whose build of the same component passed, among the check
// runs already read. It makes no call (FLT-DLV-05).
func effectiveBuilds(in deliveryInput, i int, builds, policies []componentCheck) map[string]componentRun {
	out := map[string]componentRun{}
	for _, b := range builds {
		run := componentRun{build: b, by: -1, policies: policiesOf(policies, b.component)}
		if checkState(b.run) == StageFailed {
			if j, sb, sp, ok := supersedingBuild(in, i, b.component); ok {
				run = componentRun{build: sb, by: j, policies: sp}
			}
		}
		out[b.component] = run
	}
	return out
}

// supersedingBuild finds the closest later commit with a passed build of
// component, and that commit's policy checks of it.
func supersedingBuild(in deliveryInput, i int, component string) (int, componentCheck, []componentCheck, bool) {
	for j := i - 1; j >= 0; j-- {
		runs, ok := in.checks[in.commits[j].SHA]
		if !ok {
			continue
		}
		builds, policies := classifyChecks(runs)
		for _, b := range builds {
			if b.component == component && checkState(b.run) == StagePassed {
				return j, b, policiesOf(policies, component), true
			}
		}
	}
	return 0, componentCheck{}, nil, false
}

func policiesOf(policies []componentCheck, component string) []componentCheck {
	var out []componentCheck
	for _, p := range policies {
		if p.component == component {
			out = append(out, p)
		}
	}
	return out
}

// supersededBy returns the newest commit index that superseded one of the
// change's failed builds, or -1.
func supersededBy(eff map[string]componentRun) int {
	newest := -1
	for _, r := range eff {
		if r.by >= 0 && (newest == -1 || r.by < newest) {
			newest = r.by
		}
	}
	return newest
}

// componentBundles maps each rebuilt component to the oldest bundle that
// carries it at the change at commit index i or a later commit (FLT-DLV-22).
// A component no bundle carries yet is absent.
func componentBundles(in deliveryInput, pl placement, i int, rebuilt []string) map[string]int {
	out := map[string]int{}
	for _, k := range rebuilt {
		for bi, b := range in.bundles {
			rev, ok := b.Components[k]
			if !ok {
				continue
			}
			if j, ok := pl.commitIdx[rev]; ok && j <= i {
				out[k] = bi
				break
			}
		}
	}
	return out
}

// missingState is what a component not in a bundle yet is doing.
func missingState(r componentRun) string {
	switch checkState(r.build.run) {
	case StageRunning:
		return missingBuilding
	case StageFailed:
		return missingFailed
	}
	if len(r.policies) == 0 {
		return missingPolicy
	}
	st, _, _, _ := aggregate(r.policies, "")
	switch st {
	case StagePassed, StageNotNeeded:
		return missingReleasing
	case StageFailed:
		return missingFailed
	default:
		return missingPolicy
	}
}

// partlyShipped returns the partly shipped view of a change: the bundle
// index that carries every shipped component, and the counts. ok is false
// when the change is not partly shipped (FLT-DLV-22).
func partlyShipped(in deliveryInput, pl placement, i int, rebuilt []string, eff map[string]componentRun, app string) (int, DeliveryPartly, bool) {
	cb := componentBundles(in, pl, i, rebuilt)
	if len(cb) == 0 || len(cb) == len(rebuilt) {
		return -1, DeliveryPartly{}, false
	}
	first := -1
	for _, bi := range cb {
		first = max(first, bi)
	}
	p := DeliveryPartly{Done: len(cb), Total: len(rebuilt)}
	for _, k := range rebuilt {
		if _, ok := cb[k]; ok {
			continue
		}
		p.Missing = append(p.Missing, DeliveryMissing{Component: shortComponent(k, app), State: missingState(eff[k])})
	}
	sort.Slice(p.Missing, func(a, b int) bool { return p.Missing[a].Component < p.Missing[b].Component })
	return first, p, true
}
