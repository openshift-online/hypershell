package watcher

import (
	"context"
	"sync"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
)

// gatewayRecordingHandler records the gateway payloads it is invoked with so a
// test can assert on the phase the reconciler would observe.
type gatewayRecordingHandler struct {
	mu   sync.Mutex
	seen []*pb.Gateway
}

func (h *gatewayRecordingHandler) Handle(_ context.Context, ev Event[*pb.Gateway]) error {
	h.mu.Lock()
	h.seen = append(h.seen, ev.Resource)
	h.mu.Unlock()
	return nil
}

func (h *gatewayRecordingHandler) snapshot() []*pb.Gateway {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*pb.Gateway(nil), h.seen...)
}

func waitForGatewayCalls(t *testing.T, h *gatewayRecordingHandler, want int) []*pb.Gateway {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := h.snapshot(); len(got) >= want {
			return got
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("handler called %d times, want >= %d", len(h.snapshot()), want)
	return nil
}

// A plain (non-forced) enqueue preserves the payload verbatim, so the normal
// watch-stream path still lets the reconciler's phase gate govern create/update
// traffic. This guards the boundary between the fan-out path (forced, gate-
// bypassing) and ordinary reconciliation.
func TestGatewayReconcileQueue_EnqueuePreservesPhase(t *testing.T) {
	h := &gatewayRecordingHandler{}
	q := NewGatewayReconcileQueue(context.Background(), h, 1)
	defer q.Stop()

	running := "Running"
	q.q.enqueue(Event[*pb.Gateway]{
		Type:       EventUpdated,
		ResourceID: "gw-2",
		Resource: &pb.Gateway{
			Metadata: &pb.ObjectReference{Id: "gw-2"},
			Phase:    &running,
		},
	})

	seen := waitForGatewayCalls(t, h, 1)
	if seen[0].GetPhase() != "Running" {
		t.Fatalf("expected phase preserved on plain enqueue, got %q", seen[0].GetPhase())
	}
}
