package reconciler

import (
	"context"
	"errors"
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"testing"
	"time"
)

func TestOIDCHealthSuccessOnlyAdvancesAfterFullComparison(t *testing.T) {
	now := time.Unix(1000, 0)
	r := &SandboxAttentionReconciler{now: func() time.Time { return now }, publishOIDC: func(context.Context, int, int, bool) {}}
	r.publishOIDCObservation(t.Context(), 0, 2, false)
	if !r.oidcLastSuccess.IsZero() {
		t.Fatal("failed initial observation recorded success")
	}
	r.publishOIDCObservation(t.Context(), 1, 2, true)
	if !r.oidcLastSuccess.IsZero() {
		t.Fatal("partial synchronization recorded success")
	}
	r.publishOIDCObservation(t.Context(), 2, 2, true)
	success := r.oidcLastSuccess
	now = now.Add(time.Minute)
	r.publishOIDCObservation(t.Context(), 0, 0, false)
	if !r.oidcLastSuccess.Equal(success) {
		t.Fatal("failure advanced the last success time")
	}
	r.publishOIDCObservation(t.Context(), 0, 0, true)
	if !r.oidcLastSuccess.Equal(now) {
		t.Fatal("valid empty comparison did not record success")
	}
}

func TestOIDCHealthLoopBoundsChecksAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	checked := make(chan struct{})
	r := &SandboxAttentionReconciler{
		now: time.Now,
		listGateways: func(ctx context.Context) ([]*pb.Gateway, error) {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > oidcHealthTimeout {
				t.Error("comparison has no bounded deadline")
			}
			return nil, errors.New("API unavailable")
		},
		publishOIDC: func(_ context.Context, synced, expected int, connected bool) {
			if connected {
				t.Error("failed inventory comparison reported connected")
			}
			close(checked)
			cancel()
		},
	}
	done := make(chan struct{})
	go func() { defer close(done); r.runOIDCHealth(ctx) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("health loop did not stop")
	}
	select {
	case <-checked:
	default:
		t.Fatal("initial health check was skipped")
	}
	if !r.oidcLastSuccess.IsZero() {
		t.Fatal("failed check recorded success")
	}
}
