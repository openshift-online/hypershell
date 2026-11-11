package reconciler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/exposure"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// scriptedExposure returns a readiness/err drawn from a per-call script,
// clamping to the last entry so a "ready" or "error" state persists.
type scriptedExposure struct {
	steps   []exposure.Readiness
	err     error
	calls   int32
	lastReq exposure.Request
}

func (s *scriptedExposure) ResolveAddress(context.Context, exposure.Request) (string, error) {
	return "", nil
}

func (s *scriptedExposure) ObserveReadiness(_ context.Context, req exposure.Request) (exposure.Readiness, error) {
	s.lastReq = req
	i := int(atomic.AddInt32(&s.calls, 1)) - 1
	if s.err != nil && i == 0 {
		// First observation errors; subsequent ones fall through to the script.
		return exposure.Readiness{}, s.err
	}
	if i >= len(s.steps) {
		i = len(s.steps) - 1
	}
	return s.steps[i], nil
}

func TestPollRouteReady(t *testing.T) {
	notReady := exposure.Readiness{Ready: false, Reason: "gateway not programmed: Pending"}
	ready := exposure.Readiness{Ready: true}

	t.Run("ready on first observation returns true", func(t *testing.T) {
		exp := &scriptedExposure{steps: []exposure.Readiness{ready}}
		r := &GatewayReconciler{exposure: exp}
		if !r.pollRouteReady(t.Context(), "ns", "grpcs://ns.example.com:443", time.Millisecond, time.Second) {
			t.Fatal("expected route to be observed ready")
		}
		if got := atomic.LoadInt32(&exp.calls); got != 1 {
			t.Errorf("expected exactly 1 observation, got %d", got)
		}
	})

	t.Run("becomes ready after polling returns true", func(t *testing.T) {
		exp := &scriptedExposure{steps: []exposure.Readiness{notReady, notReady, ready}}
		r := &GatewayReconciler{exposure: exp}
		if !r.pollRouteReady(t.Context(), "ns", "grpcs://ns.example.com:443", time.Millisecond, time.Second) {
			t.Fatal("expected route to become ready after polling")
		}
		if got := atomic.LoadInt32(&exp.calls); got < 3 {
			t.Errorf("expected at least 3 observations, got %d", got)
		}
	})

	t.Run("retries past a transient error", func(t *testing.T) {
		exp := &scriptedExposure{steps: []exposure.Readiness{ready}, err: context.DeadlineExceeded}
		r := &GatewayReconciler{exposure: exp}
		if !r.pollRouteReady(t.Context(), "ns", "grpcs://ns.example.com:443", time.Millisecond, time.Second) {
			t.Fatal("expected route to become ready after a transient error")
		}
	})

	t.Run("window elapses without readiness returns false", func(t *testing.T) {
		exp := &scriptedExposure{steps: []exposure.Readiness{notReady}}
		r := &GatewayReconciler{exposure: exp}
		if r.pollRouteReady(t.Context(), "ns", "grpcs://ns.example.com:443", time.Millisecond, 20*time.Millisecond) {
			t.Fatal("expected timeout to report not ready")
		}
	})

	t.Run("cancelled context returns false", func(t *testing.T) {
		exp := &scriptedExposure{steps: []exposure.Readiness{notReady}}
		r := &GatewayReconciler{exposure: exp}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if r.pollRouteReady(ctx, "ns", "grpcs://ns.example.com:443", time.Millisecond, time.Minute) {
			t.Fatal("expected cancelled context to report not ready")
		}
	})

	// The provisioning path forwards the routeAddress it just resolved to the
	// exposure port, so the port can gate readiness on publication rather than
	// on Gateway API resource status alone.
	//
	// Verifies: OSG-ROUT-08.05
	t.Run("forwards routeAddress to the exposure port", func(t *testing.T) {
		exp := &scriptedExposure{steps: []exposure.Readiness{ready}}
		r := &GatewayReconciler{exposure: exp}
		r.pollRouteReady(t.Context(), "ns", "grpcs://gw-ns.example.com:443", time.Millisecond, time.Second)
		if got, want := exp.lastReq.RouteAddress, "grpcs://gw-ns.example.com:443"; got != want {
			t.Errorf("RouteAddress = %q, want %q", got, want)
		}
	})
}

// A previously published routeAddress must survive a provisioning pass whose
// own resolve attempt errors or returns empty, or whose save fails: such a
// pass must not make an already-reachable gateway look unpublished.
//
// Verifies: OSG-ROUT-07.01
func TestRouteAddressForPoll(t *testing.T) {
	tests := []struct {
		name             string
		persisted        string
		resolvedAndSaved string
		want             string
	}{
		{
			name:             "pass resolved and saved a new address: use it",
			persisted:        "grpcs://gw-ns.example.com:443",
			resolvedAndSaved: "grpcs://gw-ns.example.com:443",
			want:             "grpcs://gw-ns.example.com:443",
		},
		{
			// Resolve errored this pass (e.g. ResolveAddress returned an error):
			// resolvedAndSaved is empty, but a previously published address must
			// still count as published.
			name:      "resolve errored this pass: fall back to the persisted address",
			persisted: "grpcs://gw-ns.example.com:443",
			want:      "grpcs://gw-ns.example.com:443",
		},
		{
			// Resolve returned empty this pass (e.g. GATEWAY_API_BASE_DOMAIN
			// unset): still fall back rather than reporting unpublished.
			name:      "resolve returned empty this pass: fall back to the persisted address",
			persisted: "grpcs://gw-ns.example.com:443",
			want:      "grpcs://gw-ns.example.com:443",
		},
		{
			// The save (updateRouteAddress) failed this pass: resolvedAndSaved is
			// empty even though a new address was resolved, so fall back.
			name:      "save failed this pass: fall back to the persisted address",
			persisted: "grpcs://gw-ns.example.com:443",
			want:      "grpcs://gw-ns.example.com:443",
		},
		{
			name: "never published and nothing resolved this pass: empty",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := routeAddressForPoll(tt.persisted, tt.resolvedAndSaved); got != tt.want {
				t.Errorf("routeAddressForPoll(%q, %q) = %q, want %q", tt.persisted, tt.resolvedAndSaved, got, tt.want)
			}
		})
	}
}

// currentRouteAddress re-reads the gateway live rather than trusting the
// watch-event snapshot captured when a provisioning pass started: that
// snapshot can be minutes stale by the time the routeAddress fallback runs
// (Helm apply plus readiness waits), during which the continuous health
// reconciler can concurrently clear or republish the persisted address.
//
// Verifies: OSG-ROUT-07.01
func TestCurrentRouteAddress(t *testing.T) {
	snapshot := &pb.Gateway{Name: "gw", RouteAddress: strPtr("grpcs://stale.example.com:443")}

	t.Run("live re-read returns the current persisted value, not the stale snapshot", func(t *testing.T) {
		fake := &fakeGatewayReReadClient{resp: &pb.GetGatewayResponse{
			Gateway: &pb.Gateway{Name: "gw", RouteAddress: strPtr("grpcs://current.example.com:443")},
		}}
		r := &GatewayReconciler{gatewayClientOverride: fake}
		if got, want := r.currentRouteAddress(t.Context(), "gw-1", snapshot), "grpcs://current.example.com:443"; got != want {
			t.Errorf("currentRouteAddress = %q, want %q (the live value, not the stale snapshot's %q)", got, want, snapshot.GetRouteAddress())
		}
		if fake.gotID != "gw-1" {
			t.Errorf("re-read id = %q, want gw-1", fake.gotID)
		}
	})

	t.Run("live re-read reports the address was cleared, not the stale snapshot's value", func(t *testing.T) {
		fake := &fakeGatewayReReadClient{resp: &pb.GetGatewayResponse{
			Gateway: &pb.Gateway{Name: "gw", RouteAddress: strPtr("")},
		}}
		r := &GatewayReconciler{gatewayClientOverride: fake}
		if got := r.currentRouteAddress(t.Context(), "gw-1", snapshot); got != "" {
			t.Errorf("currentRouteAddress = %q, want empty (route was torn down since the snapshot)", got)
		}
	})

	t.Run("a transient re-read error falls back to the snapshot rather than aborting", func(t *testing.T) {
		fake := &fakeGatewayReReadClient{err: status.Error(codes.Unavailable, "api down")}
		r := &GatewayReconciler{gatewayClientOverride: fake}
		if got, want := r.currentRouteAddress(t.Context(), "gw-1", snapshot), snapshot.GetRouteAddress(); got != want {
			t.Errorf("currentRouteAddress = %q, want the snapshot's %q on a transient error", got, want)
		}
	})
}

func TestPoll(t *testing.T) {
	t.Run("returns true as soon as attempt succeeds", func(t *testing.T) {
		calls := 0
		ok := poll(t.Context(), time.Millisecond, time.Second, func() bool {
			calls++
			return calls == 3
		})
		if !ok {
			t.Fatal("expected poll to report success")
		}
		if calls != 3 {
			t.Errorf("expected attempt to run 3 times, got %d", calls)
		}
	})

	t.Run("window elapses without success returns false", func(t *testing.T) {
		if poll(t.Context(), time.Millisecond, 20*time.Millisecond, func() bool {
			return false
		}) {
			t.Fatal("expected timeout to report failure")
		}
	})

	t.Run("cancelled context returns false", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if poll(ctx, time.Millisecond, time.Minute, func() bool { return false }) {
			t.Fatal("expected cancelled context to report failure")
		}
	})
}

func TestConsoleAddressFor(t *testing.T) {
	const url = "https://console-openshell-abc.gw.example.com"

	// Ready console publishes its URL so the web UI's console button appears.
	if got := consoleAddressFor(true, url); got != url {
		t.Errorf("consoleAddressFor(ready) = %q, want %q", got, url)
	}
	// A not-ready console clears the address so the button stays hidden (or is
	// retracted if the pod later goes unready).
	if got := consoleAddressFor(false, url); got != "" {
		t.Errorf("consoleAddressFor(not ready) = %q, want empty", got)
	}
}
