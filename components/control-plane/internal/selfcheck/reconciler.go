package selfcheck

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
)

// DefaultInterval is the documented default self-check cadence used when
// HYPERSHELL_SELF_CHECK_INTERVAL is unset, zero, or invalid (PLAT-HLTH-04).
const DefaultInterval = 2 * time.Minute

// CheckResult is the outcome of evaluating a single check.
type CheckResult struct {
	Status  Status
	Message string
}

// ProbeFunc evaluates one check. It SHALL NOT panic; any error condition is
// returned as a fail or unknown CheckResult so a single check never aborts the
// run (PLAT-HLTH-01). The runner also recovers from a panicking probe as a
// defensive backstop so a self-check run never crashes the control plane.
type ProbeFunc func(ctx context.Context) CheckResult

// Check is one catalog entry: a bounded check name and its probe. The catalog is
// ordered cheapest-first (PLAT-HLTH-02).
type Check struct {
	Name  CheckName
	Probe ProbeFunc
}

// Holder stores the most recent verdict for the registration/heartbeat path to
// read (PLAT-HLTH-05). It is safe for concurrent use: the self-check loop writes
// it and the heartbeat goroutine reads it.
type Holder struct {
	mu     sync.Mutex
	latest *Verdict
}

// NewHolder returns an empty Holder. Until the first verdict is published,
// LatestHealth returns nil so a heartbeat carries no health object and the hub
// preserves any stored verdict (PLAT-CLUS-16.02).
func NewHolder() *Holder {
	return &Holder{}
}

// publish stores a copy of the verdict as the latest.
func (h *Holder) publish(v Verdict) {
	cp := v
	cp.Conditions = append([]Condition(nil), v.Conditions...)
	h.mu.Lock()
	h.latest = &cp
	h.mu.Unlock()
}

// Latest returns a copy of the most recent verdict, or nil when none has been
// published yet.
func (h *Holder) Latest() *Verdict {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.latest == nil {
		return nil
	}
	cp := *h.latest
	cp.Conditions = append([]Condition(nil), h.latest.Conditions...)
	return &cp
}

// LatestHealth returns the most recent verdict as an untyped value for the
// registration client to serialize onto the heartbeat body, or an untyped nil
// when none has been published yet. Returning a literal nil (not a typed nil
// *Verdict) keeps the registration client's omitempty check correct, so a
// control plane that has not yet produced a verdict sends a health-less
// heartbeat (PLAT-CLUS-16.02). It satisfies the registration client's health
// provider seam without coupling that package to this one.
func (h *Holder) LatestHealth() any {
	v := h.Latest()
	if v == nil {
		return nil
	}
	return v
}

// Reconciler is the continuous on-cluster self-check loop (PLAT-HLTH-01). It runs
// the catalog on a configurable cadence, computes a verdict, publishes it to the
// Holder for the heartbeat path, and exports the health gauges (OBS-CPL-12).
type Reconciler struct {
	checks      []Check
	interval    time.Duration
	clusterName string
	holder      *Holder

	// now is the clock, overridable in tests. It is used for ObservedAt and for
	// last-transition stability; run duration is measured with the monotonic
	// clock independently so it stays meaningful under a fake clock.
	now func() time.Time

	// mu guards lastConditions, which records the previous run's condition per
	// check so last_transition_at changes only when a condition's status changes
	// (PLAT-HLTH-03). In-memory only: on restart the timestamps restart, which is
	// acceptable.
	mu             sync.Mutex
	lastConditions map[CheckName]Condition
}

// NewReconciler builds a self-check reconciler. interval <= 0 falls back to
// DefaultInterval (PLAT-HLTH-04). holder may be the one the registration client
// reads; it is always written so the latest verdict is available for reporting.
func NewReconciler(checks []Check, interval time.Duration, clusterName string, holder *Holder) *Reconciler {
	if interval <= 0 {
		interval = DefaultInterval
	}
	if holder == nil {
		holder = NewHolder()
	}
	return &Reconciler{
		checks:         checks,
		interval:       interval,
		clusterName:    clusterName,
		holder:         holder,
		now:            time.Now,
		lastConditions: make(map[CheckName]Condition),
	}
}

// Run drives the self-check loop until the context is cancelled. It runs once
// immediately, then on every interval tick, mirroring the other continuous
// reconcilers. It never returns an error from a check: a self-check run never
// crashes, blocks, or degrades primary reconciliation (PLAT-HLTH-01).
func (r *Reconciler) Run(ctx context.Context) error {
	slog.InfoContext(ctx, "Managed-cluster self-check reconciler started", "interval", r.interval.String(), "checks", len(r.checks))
	if err := ctx.Err(); err != nil {
		return err
	}
	r.reconcileOnce(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.reconcileOnce(ctx)
		}
	}
}

func (r *Reconciler) reconcileOnce(ctx context.Context) {
	verdict := r.evaluate(ctx)
	r.holder.publish(verdict)
	r.export(ctx, verdict)
	slog.InfoContext(ctx, "Managed-cluster self-check verdict computed", "verdict", string(verdict.Overall), "duration_ms", verdict.DurationMs, "conditions", len(verdict.Conditions))
}

// evaluate runs every check in catalog order, collects one condition per check
// (never aborting on a failure), applies last-transition stability, and rolls up
// the overall verdict (PLAT-HLTH-01, PLAT-HLTH-02, PLAT-HLTH-03).
func (r *Reconciler) evaluate(ctx context.Context) Verdict {
	observedAt := r.now()
	start := time.Now()

	conditions := make([]Condition, 0, len(r.checks))
	for _, check := range r.checks {
		result := runProbe(ctx, check)
		conditions = append(conditions, r.stabilize(check.Name, result, observedAt))
	}

	return Verdict{
		Overall:    Rollup(conditions),
		ObservedAt: observedAt,
		DurationMs: time.Since(start).Milliseconds(),
		Conditions: conditions,
	}
}

// runProbe executes one probe, converting a panic into an unknown condition so a
// single misbehaving check never crashes the control plane (PLAT-HLTH-01).
func runProbe(ctx context.Context, check Check) (result CheckResult) {
	defer func() {
		if rec := recover(); rec != nil {
			result = CheckResult{Status: StatusUnknown, Message: fmt.Sprintf("check panicked: %v", rec)}
		}
	}()
	if check.Probe == nil {
		return CheckResult{Status: StatusUnknown, Message: "no probe configured"}
	}
	return check.Probe(ctx)
}

// stabilize builds the condition for a check result, keeping last_transition_at
// unchanged while the status holds and advancing it to observedAt on a change
// (PLAT-HLTH-03.03). It records the result for the next run.
func (r *Reconciler) stabilize(name CheckName, result CheckResult, observedAt time.Time) Condition {
	status := result.Status
	if status == "" {
		status = StatusUnknown
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	transition := observedAt
	if prev, ok := r.lastConditions[name]; ok && prev.Status == status {
		transition = prev.LastTransitionAt
	}
	cond := Condition{
		Name:             name,
		Status:           status,
		LastTransitionAt: transition,
		Message:          result.Message,
	}
	r.lastConditions[name] = cond
	return cond
}

// export emits the verdict as low-cardinality OpenTelemetry gauges (OBS-CPL-12).
// When metrics are disabled the gauges are nil and nothing is exported; when the
// managed-cluster name is empty nothing is exported; the loop and reporting still
// run (PLAT-HLTH-07.02).
func (r *Reconciler) export(ctx context.Context, verdict Verdict) {
	checks := make(map[string]int64, len(verdict.Conditions))
	for _, c := range verdict.Conditions {
		checks[string(c.Name)] = checkGauge(c.Status)
	}
	cpotel.RecordManagedClusterHealth(ctx, r.clusterName, overallGauge(verdict.Overall), checks)
}

// overallGauge encodes the rollup as the bounded numeric state OBS-CPL-12
// documents (2 healthy, 1 degraded, 0 failing).
func overallGauge(overall Overall) int64 {
	switch overall {
	case OverallHealthy:
		return 2
	case OverallDegraded:
		return 1
	default:
		return 0
	}
}

// checkGauge encodes a per-check status as the bounded numeric state OBS-CPL-12
// documents (1 pass, 0 otherwise). Only a passing check reports 1, so an alert
// can treat any 0 series as a check that is not currently passing.
func checkGauge(status Status) int64 {
	if status == StatusPass {
		return 1
	}
	return 0
}
