package reconciler

import (
	"context"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestManagedDatabaseStatus(t *testing.T) {
	if got := managedDatabaseStatus(nil); got != "" {
		t.Fatalf("nil db: got %q, want empty", got)
	}

	ready := "Ready"
	if got := managedDatabaseStatus(&pb.ManagedDatabase{Status: &ready}); got != "Ready" {
		t.Fatalf("got %q, want Ready", got)
	}

	if got := managedDatabaseStatus(&pb.ManagedDatabase{}); got != "" {
		t.Fatalf("missing status: got %q, want empty", got)
	}
}

func TestCNPGClusterReadyFromObject(t *testing.T) {
	healthy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"phase": "Cluster in healthy state",
			},
		},
	}
	if !cnpgClusterReadyFromObject(healthy) {
		t.Fatal("healthy phase should be ready")
	}

	readyInstances := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"instances":      int64(1),
				"readyInstances": int64(1),
			},
		},
	}
	if !cnpgClusterReadyFromObject(readyInstances) {
		t.Fatal("ready instances should be ready")
	}

	provisioning := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"phase":          "Setting up primary",
				"instances":      int64(1),
				"readyInstances": int64(0),
			},
		},
	}
	if cnpgClusterReadyFromObject(provisioning) {
		t.Fatal("provisioning cluster should not be ready")
	}

	if cnpgClusterReadyFromObject(nil) {
		t.Fatal("nil object should not be ready")
	}
}

// A live event that arrives while a reconcile for the same ManagedDatabase is
// already in flight must be retained (not dropped) so the watch stream's
// no-replay guarantee never permanently loses it. A second live event while
// still busy must overwrite the first: only the latest pending state matters.
func TestManagedDatabaseReconciler_Handle_RetainsLatestPendingEventWhileBusy(t *testing.T) {
	r := &ManagedDatabaseReconciler{
		active:  make(map[string]struct{}),
		pending: make(map[string]watcher.Event[*pb.ManagedDatabase]),
	}
	const id = "db-1"

	// Simulate a reconcile already in flight for this resource.
	r.active[id] = struct{}{}

	first := watcher.Event[*pb.ManagedDatabase]{
		ResourceID: id,
		Type:       watcher.EventUpdated,
		Resource:   &pb.ManagedDatabase{Namespace: "first"},
	}
	if err := r.Handle(context.Background(), first); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	r.mu.Lock()
	pending, ok := r.pending[id]
	r.mu.Unlock()
	if !ok || pending.Resource.GetNamespace() != "first" {
		t.Fatalf("pending = %+v, ok=%v; want the first event retained", pending, ok)
	}

	second := watcher.Event[*pb.ManagedDatabase]{
		ResourceID: id,
		Type:       watcher.EventDeleted,
		Resource:   &pb.ManagedDatabase{Namespace: "second"},
	}
	if err := r.Handle(context.Background(), second); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	r.mu.Lock()
	pending, ok = r.pending[id]
	r.mu.Unlock()
	if !ok {
		t.Fatal("pending event was dropped, want the second event retained")
	}
	if pending.Type != watcher.EventDeleted || pending.Resource.GetNamespace() != "second" {
		t.Fatalf("pending = %+v, want the latest (second) event to survive, not the first", pending)
	}
}
