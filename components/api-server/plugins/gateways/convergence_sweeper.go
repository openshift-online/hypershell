package gateways

import (
	"context"
	"log/slog"
	"time"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// convergenceSweepInterval is how often each replica attempts the deadline sweep.
// The sweep is cheap (one indexed query plus a resolve per expired window) and
// the ten-minute budget is coarse, so a one-minute cadence resolves a stalled
// change promptly after its deadline without polling pressure (OBS-SLI-02).
const convergenceSweepInterval = time.Minute

// StartConvergenceDeadlineSweeper launches the background deadline sweep that
// resolves stalled changes at their ten-minute deadline even when no status
// write arrives (OBS-SLI-02). Each tick runs one pass under a non-blocking
// advisory lock, so the sweep runs on one replica at a time and the resolution
// commits atomically with the counter increment, keeping outcomes exactly-once
// across replicas and restarts (OBS-SLI-02.06). The goroutine exits when ctx is
// cancelled. It is serve-only; the migrate command never calls it.
func StartConvergenceDeadlineSweeper(ctx context.Context, env *environments.Env) {
	// Called from the serve command's PreRunE, before the framework initializes
	// the database: pass the session factory by pointer and let the store build
	// its advisory-lock factory on the first sweep, once the database is up.
	store := newConvergenceStore(&env.Database.SessionFactory, nil)
	go runConvergenceSweepLoop(ctx, store, convergenceSweepInterval)
}

// RunConvergenceDeadlineSweepForTest runs exactly one deadline-sweep pass and
// returns the number of windows resolved. It lets integration tests drive the
// sweep deterministically (OBS-SLI-02.02 / 02.06) instead of waiting for the
// ticker.
func RunConvergenceDeadlineSweepForTest(ctx context.Context, env *environments.Env) (int, error) {
	store := newConvergenceStore(&env.Database.SessionFactory, nil)
	return store.runDeadlineSweep(ctx)
}

func runConvergenceSweepLoop(ctx context.Context, store *convergenceStore, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			resolved, err := store.runDeadlineSweep(ctx)
			if err != nil {
				slog.ErrorContext(ctx, "convergence deadline sweep failed", "error", err)
				continue
			}
			if resolved > 0 {
				slog.InfoContext(ctx, "convergence deadline sweep resolved stalled changes", "count", resolved)
			}
		}
	}
}
