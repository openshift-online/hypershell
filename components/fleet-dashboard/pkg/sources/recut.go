package sources

import (
	"maps"
	"sort"
)

// sameComponents reports whether a bundle is a re-cut of the bundle before it:
// both carry the same component revisions, so only the release name differs
// (FLT-DLV-23.06, SDLC-VIS-11.07). Bundles without component revisions are never
// called the same.
func sameComponents(prev, cur map[string]string) bool {
	return len(cur) > 0 && maps.Equal(prev, cur)
}

// markRecuts sets SameAs on each release whose component revisions equal those
// of the release published just before it, and clears it on every other one.
// Releases without a date or component revisions cannot be placed, so they are
// skipped and never compared.
func markRecuts(releases map[string]*Release) {
	var ordered []*Release
	for _, r := range releases {
		if r == nil {
			continue
		}
		r.SameAs = ""
		if r.Date != "" && len(r.Components) > 0 {
			ordered = append(ordered, r)
		}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Date != ordered[j].Date {
			return ordered[i].Date < ordered[j].Date
		}
		return ordered[i].Digest < ordered[j].Digest
	})
	for i := 1; i < len(ordered); i++ {
		if sameComponents(ordered[i-1].Components, ordered[i].Components) {
			ordered[i].SameAs = ordered[i-1].Digest
		}
	}
}
