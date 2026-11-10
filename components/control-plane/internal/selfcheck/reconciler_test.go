package selfcheck

import (
	"context"
	"testing"
	"time"
)

// staticCheck builds a check whose probe always returns the given result and
// records how many times it ran.
func staticCheck(name CheckName, status Status, calls *int) Check {
	return Check{Name: name, Probe: func(_ context.Context) CheckResult {
		if calls != nil {
			*calls++
		}
		return CheckResult{Status: status, Message: string(status)}
	}}
}

func TestEvaluateProducesOneConditionPerCheck(t *testing.T) {
	// PLAT-HLTH-02.01: one condition per check, each carrying a bounded name.
	r := NewReconciler([]Check{
		staticCheck(CheckReconcilerHealthy, StatusPass, nil),
		staticCheck(CheckImagesPullable, StatusPass, nil),
		staticCheck(CheckOIDCIssuerReachable, StatusPass, nil),
	}, time.Minute, "mc1", NewHolder())

	v := r.evaluate(context.Background())
	if len(v.Conditions) != 3 {
		t.Fatalf("conditions = %d, want 3", len(v.Conditions))
	}
	if v.Overall != OverallHealthy {
		t.Fatalf("overall = %q, want healthy", v.Overall)
	}
	if v.ObservedAt.IsZero() {
		t.Fatalf("observed_at not set")
	}
	if v.DurationMs < 0 {
		t.Fatalf("duration_ms = %d, want >= 0", v.DurationMs)
	}
	names := map[CheckName]bool{}
	for _, c := range v.Conditions {
		names[c.Name] = true
	}
	for _, want := range []CheckName{CheckReconcilerHealthy, CheckImagesPullable, CheckOIDCIssuerReachable} {
		if !names[want] {
			t.Errorf("missing condition %q", want)
		}
	}
}

func TestEvaluateDoesNotAbortOnFailingCheck(t *testing.T) {
	// PLAT-HLTH-01.02: a failing check does not abort the run; every other check
	// still runs and records its own condition.
	var oidcCalls, dbCalls int
	r := NewReconciler([]Check{
		staticCheck(CheckOIDCIssuerReachable, StatusFail, &oidcCalls),
		staticCheck(CheckGatewayDatabaseReachable, StatusPass, &dbCalls),
	}, time.Minute, "mc1", NewHolder())

	v := r.evaluate(context.Background())
	if oidcCalls != 1 || dbCalls != 1 {
		t.Fatalf("calls: oidc=%d db=%d, want 1 and 1", oidcCalls, dbCalls)
	}
	if v.Overall != OverallFailing {
		t.Fatalf("overall = %q, want failing", v.Overall)
	}
	var sawFail, sawPass bool
	for _, c := range v.Conditions {
		if c.Name == CheckOIDCIssuerReachable && c.Status == StatusFail {
			sawFail = true
		}
		if c.Name == CheckGatewayDatabaseReachable && c.Status == StatusPass {
			sawPass = true
		}
	}
	if !sawFail || !sawPass {
		t.Fatalf("expected both the failing and the passing condition; sawFail=%v sawPass=%v", sawFail, sawPass)
	}
}

func TestPanickingProbeBecomesUnknownCondition(t *testing.T) {
	// A self-check run never crashes: a panicking probe becomes an unknown
	// condition and the remaining checks still run (PLAT-HLTH-01).
	var okCalls int
	r := NewReconciler([]Check{
		{Name: CheckReconcilerHealthy, Probe: func(_ context.Context) CheckResult { panic("boom") }},
		staticCheck(CheckImagesPullable, StatusPass, &okCalls),
	}, time.Minute, "mc1", NewHolder())

	v := r.evaluate(context.Background())
	if okCalls != 1 {
		t.Fatalf("second check ran %d times, want 1", okCalls)
	}
	var unknown bool
	for _, c := range v.Conditions {
		if c.Name == CheckReconcilerHealthy && c.Status == StatusUnknown {
			unknown = true
		}
	}
	if !unknown {
		t.Fatalf("panicking probe did not become an unknown condition")
	}
}

func TestLastTransitionStableWhileStatusHolds(t *testing.T) {
	// PLAT-HLTH-03.03: last_transition_at is unchanged while the status holds, and
	// advances when the status changes.
	status := StatusPass
	clock := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	r := NewReconciler([]Check{
		{Name: CheckReconcilerHealthy, Probe: func(_ context.Context) CheckResult {
			return CheckResult{Status: status}
		}},
	}, time.Minute, "mc1", NewHolder())
	r.now = func() time.Time { return clock }

	first := r.evaluate(context.Background())
	firstTransition := first.Conditions[0].LastTransitionAt

	// Second run: same status, later clock -> transition unchanged.
	clock = clock.Add(time.Minute)
	second := r.evaluate(context.Background())
	if !second.Conditions[0].LastTransitionAt.Equal(firstTransition) {
		t.Fatalf("last_transition_at changed while status held: %s -> %s", firstTransition, second.Conditions[0].LastTransitionAt)
	}

	// Third run: status changes, later clock -> transition advances to now.
	status = StatusFail
	clock = clock.Add(time.Minute)
	third := r.evaluate(context.Background())
	if third.Conditions[0].LastTransitionAt.Equal(firstTransition) {
		t.Fatalf("last_transition_at did not advance on a status change")
	}
	if !third.Conditions[0].LastTransitionAt.Equal(clock) {
		t.Fatalf("last_transition_at = %s, want %s", third.Conditions[0].LastTransitionAt, clock)
	}
}

func TestHolderLatestHealthNilUntilFirstVerdict(t *testing.T) {
	// PLAT-CLUS-16.02: no verdict yet -> a health-less heartbeat (nil), so the hub
	// preserves any stored verdict.
	h := NewHolder()
	if h.Latest() != nil {
		t.Fatalf("Latest() should be nil before any verdict")
	}
	if got := h.LatestHealth(); got != nil {
		t.Fatalf("LatestHealth() = %v, want untyped nil", got)
	}

	r := NewReconciler([]Check{staticCheck(CheckReconcilerHealthy, StatusPass, nil)}, time.Minute, "mc1", h)
	r.reconcileOnce(context.Background())

	if h.Latest() == nil {
		t.Fatalf("Latest() nil after a verdict was published")
	}
	if got := h.LatestHealth(); got == nil {
		t.Fatalf("LatestHealth() nil after a verdict was published")
	}
}

func TestHolderLatestIsACopy(t *testing.T) {
	h := NewHolder()
	h.publish(Verdict{Overall: OverallHealthy, Conditions: []Condition{{Name: CheckReconcilerHealthy, Status: StatusPass}}})
	got := h.Latest()
	got.Conditions[0].Status = StatusFail
	if again := h.Latest(); again.Conditions[0].Status != StatusPass {
		t.Fatalf("mutating a returned verdict mutated the stored one")
	}
}

func TestNewReconcilerZeroIntervalFallsBackToDefault(t *testing.T) {
	// PLAT-HLTH-04: a zero cadence falls back to the documented default.
	r := NewReconciler(nil, 0, "mc1", nil)
	if r.interval != DefaultInterval {
		t.Fatalf("interval = %s, want default %s", r.interval, DefaultInterval)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	r := NewReconciler([]Check{staticCheck(CheckReconcilerHealthy, StatusPass, nil)}, time.Minute, "mc1", NewHolder())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("Run returned nil, want context error")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Run did not return after cancel")
	}
}
