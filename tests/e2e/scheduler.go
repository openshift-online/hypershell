package e2e

import "testing"

// parallelSubtest is one independently runnable phase step. Slice order is the
// order steps are released to the Go test scheduler; with more than one slot it
// is only a hint, because released subtests compete for the bound.
type parallelSubtest struct {
	name string
	tag  string
	run  func(t *testing.T)
}

// runParallelSubtests runs steps as ordinary Go subtests, bounded by limit. A
// limit of one deliberately avoids t.Parallel so debugging preserves strict
// canonical execution order.
func runParallelSubtests(t *testing.T, limit int, steps []parallelSubtest) {
	t.Helper()
	if limit < 1 {
		limit = 1
	}
	if limit == 1 {
		for _, step := range steps {
			step := step
			t.Run(step.name, step.run)
		}
		return
	}

	sem := make(chan struct{}, limit)
	for _, step := range steps {
		step := step
		t.Run(step.name, func(t *testing.T) {
			t.Parallel()
			select {
			case sem <- struct{}{}:
			case <-t.Context().Done():
				return
			}
			defer func() { <-sem }()
			step.run(t)
		})
	}
}
