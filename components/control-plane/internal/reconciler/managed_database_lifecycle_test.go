package reconciler

import (
	"context"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

// A ManagedDatabase registers a server HyperShell does not own: deleting the
// registration destroys nothing on the server and creates or removes no
// Kubernetes resource. Replaying the delete must stay a no-op success.
func TestManagedDatabaseDeleteLeavesServerUntouchedAndIsIdempotent(t *testing.T) {
	dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	typed := kubernetesfake.NewSimpleClientset()
	r := NewManagedDatabaseReconciler(dynamic, typed, nil, "hypershell")

	secret := "hypershell-managed-db-test"
	event := watcher.Event[*pb.ManagedDatabase]{
		Type:       watcher.EventDeleted,
		ResourceID: "db-1",
		Resource:   &pb.ManagedDatabase{Name: "database", ConnectionSecret: &secret},
	}
	if err := r.Handle(context.Background(), event); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := r.Handle(context.Background(), event); err != nil {
		t.Fatalf("duplicate delete: %v", err)
	}
	if actions := dynamic.Actions(); len(actions) != 0 {
		t.Fatalf("delete touched %d dynamic resources, want none", len(actions))
	}
	if r.lastSeenManagedDatabase("db-1") != nil {
		t.Fatal("cache was not removed after successful cleanup")
	}
}

// A delete event whose tombstone is missing falls back to the last-seen
// resource, so a mixed-version API server sending only the ID still resolves.
func TestManagedDatabaseDeleteNilTombstoneUsesLastSeen(t *testing.T) {
	dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	typed := kubernetesfake.NewSimpleClientset()
	r := NewManagedDatabaseReconciler(dynamic, typed, nil, "hypershell")

	secret := "hypershell-managed-db-test"
	r.rememberManagedDatabase("db-1", &pb.ManagedDatabase{Name: "database", ConnectionSecret: &secret})

	deleted := watcher.Event[*pb.ManagedDatabase]{Type: watcher.EventDeleted, ResourceID: "db-1"}
	if err := r.Handle(context.Background(), deleted); err != nil {
		t.Fatalf("delete with nil tombstone: %v", err)
	}
	if r.lastSeenManagedDatabase("db-1") != nil {
		t.Fatal("cache was not removed after successful cleanup")
	}
}

func TestManagedDatabaseReconcilerNilClientsReturnsError(t *testing.T) {
	r := NewManagedDatabaseReconciler(nil, nil, nil, "hypershell")
	secret := "hypershell-managed-db-test"
	err := r.Handle(context.Background(), watcher.Event[*pb.ManagedDatabase]{
		Type:       watcher.EventDeleted,
		ResourceID: "db-1",
		Resource:   &pb.ManagedDatabase{ConnectionSecret: &secret},
	})
	if err == nil {
		t.Fatal("want nil client error")
	}
}
