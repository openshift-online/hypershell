package e2e

import (
	"context"
	"sync"
	"testing"
)

// completionBarrier delays a disruptive parallel step until a fixed set of
// controller-sensitive peers has completed, without serializing the rest of the
// fan-out.
type completionBarrier struct {
	mu        sync.Mutex
	remaining int
	done      chan struct{}
}

func newCompletionBarrier(count int) *completionBarrier {
	b := &completionBarrier{remaining: count, done: make(chan struct{})}
	if count == 0 {
		close(b.done)
	}
	return b
}

func (b *completionBarrier) Done() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.remaining == 0 {
		return
	}
	b.remaining--
	if b.remaining == 0 {
		close(b.done)
	}
}

func (b *completionBarrier) Wait(ctx context.Context) error {
	select {
	case <-b.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

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
