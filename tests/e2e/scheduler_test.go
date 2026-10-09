package e2e

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestP2ParallelStepsIncludeManagedClusterLifecycle(t *testing.T) {
	s := &E2ESuite{mode: modeLong, concurrency: 4}
	steps := s.p2ParallelSteps()
	names := make([]string, 0, len(steps))
	for _, step := range steps {
		names = append(names, step.name)
	}

	want := []string{
		"P2.1-sandbox-lifecycle",
		"P2.3-deletion-namespace-gc",
		"P2.5-gateway-access-management",
		"P2.6-gateway-service-accounts",
		"P2.2-rbac-enforcement",
		"P2.4-managedcluster-lifecycle",
	}
	if !slices.Equal(names, want) {
		t.Fatalf("parallel P2 schedule = %v, want %v", names, want)
	}
}

func TestCompletionBarrierWaitsForAllPeers(t *testing.T) {
	barrier := newCompletionBarrier(2)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	barrier.Done()
	select {
	case <-barrier.done:
		t.Fatal("barrier opened before all peers completed")
	default:
	}

	barrier.Done()
	if err := barrier.Wait(ctx); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

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
