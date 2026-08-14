package reconciler

import (
	"context"
	"strings"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
)

// fakeExposure is a stub Gateway Exposure port for driving the health
// reconciler's route-readiness decision logic.
type fakeExposure struct {
	readiness exposure.Readiness
	err       error
}

func (f fakeExposure) ResolveAddress(context.Context, exposure.Request) (string, error) {
	return "", nil
}

func (f fakeExposure) ObserveReadiness(context.Context, exposure.Request) (exposure.Readiness, error) {
	return f.readiness, f.err
}

func newHealthRec(exp exposure.Port, now func() time.Time, timeout time.Duration) *GatewayHealthReconciler {
	return &GatewayHealthReconciler{
		exposure:           exp,
		routeReadyTimeout:  timeout,
		now:                now,
		routeNotReadySince: make(map[string]time.Time),
	}
}

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestEvaluateRouteReadiness_ReadyBecomesRunning(t *testing.T) {
	h := newHealthRec(fakeExposure{readiness: exposure.Readiness{Ready: true}}, fixedClock(time.Unix(0, 0)), 10*time.Minute)
	for _, phase := range []string{"Provisioning", "Degraded"} {
		gotPhase, gotStatus := h.evaluateRouteReadiness(context.Background(), "id", "ns", phase)
		if gotPhase != "Running" || gotStatus != "Healthy" {
			t.Fatalf("from %s: got (%q,%q), want (Running,Healthy)", phase, gotPhase, gotStatus)
		}
	}
}

func TestEvaluateRouteReadiness_ProvisioningWithinGraceStaysProvisioning(t *testing.T) {
	h := newHealthRec(
		fakeExposure{readiness: exposure.Readiness{Ready: false, Reason: "gateway not programmed: Pending"}},
		fixedClock(time.Unix(1000, 0)),
		10*time.Minute,
	)
	gotPhase, gotStatus := h.evaluateRouteReadiness(context.Background(), "id", "ns", "Provisioning")
	if gotPhase != "Provisioning" {
		t.Fatalf("got phase %q, want Provisioning", gotPhase)
	}
	if gotStatus != "gateway not programmed: Pending" {
		t.Fatalf("got status %q, want the exposure reason", gotStatus)
	}
}

func TestEvaluateRouteReadiness_ProvisioningBeyondGraceBecomesDegraded(t *testing.T) {
	base := time.Unix(1000, 0)
	cur := base
	h := newHealthRec(
		fakeExposure{readiness: exposure.Readiness{Ready: false, Reason: "gateway not programmed: Pending"}},
		func() time.Time { return cur },
		10*time.Minute,
	)

	// First observation starts the grace window.
	if gotPhase, _ := h.evaluateRouteReadiness(context.Background(), "id", "ns", "Provisioning"); gotPhase != "Provisioning" {
		t.Fatalf("first tick: got %q, want Provisioning", gotPhase)
	}

	// Advance past the grace window; the gateway must move to Degraded.
	cur = base.Add(11 * time.Minute)
	gotPhase, gotStatus := h.evaluateRouteReadiness(context.Background(), "id", "ns", "Provisioning")
	if gotPhase != "Degraded" {
		t.Fatalf("after grace: got %q, want Degraded", gotPhase)
	}
	if !strings.Contains(gotStatus, "route not ready after") {
		t.Fatalf("status %q should record the grace-window expiry", gotStatus)
	}
}

func TestEvaluateRouteReadiness_RunningLosesReadinessBecomesDegraded(t *testing.T) {
	h := newHealthRec(
		fakeExposure{readiness: exposure.Readiness{Ready: false, Reason: "gateway has no assigned address"}},
		fixedClock(time.Unix(0, 0)),
		10*time.Minute,
	)
	// A Running gateway that loses readiness is Degraded immediately, with no
	// grace window.
	gotPhase, gotStatus := h.evaluateRouteReadiness(context.Background(), "id", "ns", "Running")
	if gotPhase != "Degraded" {
		t.Fatalf("got %q, want Degraded", gotPhase)
	}
	if gotStatus != "gateway has no assigned address" {
		t.Fatalf("got status %q, want the exposure reason", gotStatus)
	}
}

func TestEvaluateRouteReadiness_DegradedStaysDegraded(t *testing.T) {
	h := newHealthRec(
		fakeExposure{readiness: exposure.Readiness{Ready: false, Reason: "gateway not programmed: Pending"}},
		fixedClock(time.Unix(0, 0)),
		10*time.Minute,
	)
	if gotPhase, _ := h.evaluateRouteReadiness(context.Background(), "id", "ns", "Degraded"); gotPhase != "Degraded" {
		t.Fatalf("got %q, want Degraded", gotPhase)
	}
}

func TestEvaluateRouteReadiness_ObserveErrorLeavesPhaseUntouched(t *testing.T) {
	h := newHealthRec(
		fakeExposure{err: context.DeadlineExceeded},
		fixedClock(time.Unix(0, 0)),
		10*time.Minute,
	)
	gotPhase, gotStatus := h.evaluateRouteReadiness(context.Background(), "id", "ns", "Provisioning")
	if gotPhase != "" || gotStatus != "" {
		t.Fatalf("on observe error got (%q,%q), want empty (leave untouched)", gotPhase, gotStatus)
	}
}

func TestIsRoutedGateway(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name  string
		route *string
		want  bool
	}{
		{"nil route", nil, false},
		{"empty route", str(""), false},
		{"whitespace route", str("  "), false},
		{"null route", str("null"), false},
		{"empty object", str("{}"), true},
		{"enabled route", str(`{"enabled":true}`), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isRoutedGateway(&pb.Gateway{Route: c.route}); got != c.want {
				t.Fatalf("isRoutedGateway(%v) = %v, want %v", c.route, got, c.want)
			}
		})
	}
}
