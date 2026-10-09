package e2e

import "testing"

// parallelSubtest is one independently runnable phase step. Callers order the
// slice longest-first so work with the largest expected wall time is released to
// the Go test scheduler first.
type parallelSubtest struct {
	name string
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
			sem <- struct{}{}
			defer func() { <-sem }()
			step.run(t)
		})
	}
}
