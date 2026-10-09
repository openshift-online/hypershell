package e2e

import (
	"sync"
	"testing"
	"time"
)

func TestRunParallelSubtestsHonorsConcurrencyLimit(t *testing.T) {
	var mu sync.Mutex
	running := 0
	peak := 0

	steps := make([]parallelSubtest, 6)
	for i := range steps {
		steps[i] = parallelSubtest{
			name: "worker-" + string(rune('a'+i)),
			run: func(*testing.T) {
				mu.Lock()
				running++
				peak = max(peak, running)
				mu.Unlock()

				time.Sleep(20 * time.Millisecond)

				mu.Lock()
				running--
				mu.Unlock()
			},
		}
	}

	t.Run("bounded", func(t *testing.T) {
		runParallelSubtests(t, 2, steps)
	})

	if peak != 2 {
		t.Fatalf("peak concurrency = %d, want 2", peak)
	}
}

func TestRunParallelSubtestsRunsSeriallyAtOne(t *testing.T) {
	var mu sync.Mutex
	order := make([]string, 0, 3)
	steps := []parallelSubtest{
		{name: "first", run: func(*testing.T) { mu.Lock(); order = append(order, "first"); mu.Unlock() }},
		{name: "second", run: func(*testing.T) { mu.Lock(); order = append(order, "second"); mu.Unlock() }},
		{name: "third", run: func(*testing.T) { mu.Lock(); order = append(order, "third"); mu.Unlock() }},
	}

	runParallelSubtests(t, 1, steps)

	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	want := []string{"first", "second", "third"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("execution order = %v, want %v", got, want)
		}
	}
}
