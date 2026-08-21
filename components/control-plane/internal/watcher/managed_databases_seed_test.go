package watcher

import (
	"context"
	"errors"
	"sync"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
)

// fakeManagedDatabaseLister serves a fixed ManagedDatabase inventory across
// paginated ListManagedDatabases calls so seedManagedDatabases can be exercised
// without a real API server.
type fakeManagedDatabaseLister struct {
	pb.ManagedDatabaseServiceClient
	items    []*pb.ManagedDatabase
	pageSize int32
	err      error
	calls    int
}

func (f *fakeManagedDatabaseLister) ListManagedDatabases(_ context.Context, in *pb.ListManagedDatabasesRequest, _ ...grpc.CallOption) (*pb.ListManagedDatabasesResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	size := int(f.pageSize)
	if size == 0 {
		size = int(in.Size)
	}
	start := (int(in.Page) - 1) * size
	total := len(f.items)
	if start >= total {
		return &pb.ListManagedDatabasesResponse{
			Metadata: &pb.ListMeta{Page: in.Page, Size: in.Size, Total: int32(total)},
		}, nil
	}
	end := start + size
	if end > total {
		end = total
	}
	return &pb.ListManagedDatabasesResponse{
		Items:    f.items[start:end],
		Metadata: &pb.ListMeta{Page: in.Page, Size: in.Size, Total: int32(total)},
	}, nil
}

func md(id string) *pb.ManagedDatabase {
	return &pb.ManagedDatabase{Metadata: &pb.ObjectReference{Id: id}}
}

// recordingDBHandler captures the events seedManagedDatabases drives, so the seed
// can be asserted without a real reconciler.
type recordingDBHandler struct {
	mu     sync.Mutex
	events []Event[*pb.ManagedDatabase]
}

func (h *recordingDBHandler) Handle(_ context.Context, ev Event[*pb.ManagedDatabase]) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, ev)
	return nil
}

// seedManagedDatabases must drive an EventUpdated reconcile for every database in
// the inventory -- the LIST half of LIST-then-WATCH that recovers a create the
// watch stream never replays on (re)connect.
func TestSeedManagedDatabases_SeedsAll(t *testing.T) {
	lister := &fakeManagedDatabaseLister{items: []*pb.ManagedDatabase{
		md("openshell-db"), md("fleet-b-db"), md("fleet-c-db"),
	}}
	h := &recordingDBHandler{}

	if err := seedManagedDatabases(context.Background(), lister, h); err != nil {
		t.Fatalf("seedManagedDatabases: %v", err)
	}

	if len(h.events) != 3 {
		t.Fatalf("handled %d events, want all 3 seeded", len(h.events))
	}
	for _, ev := range h.events {
		if ev.Type != EventUpdated {
			t.Errorf("%s: seeded as event type %d, want EventUpdated", ev.ResourceID, ev.Type)
		}
		if ev.Resource == nil {
			t.Errorf("%s: seeded with nil resource", ev.ResourceID)
		}
	}
}

// A ManagedDatabase inventory larger than one page must be seeded in full.
func TestSeedManagedDatabases_Paginates(t *testing.T) {
	var items []*pb.ManagedDatabase
	for i := 0; i < managedDatabaseSeedPageSize+7; i++ {
		items = append(items, md(string(rune('a'+i%26))+string(rune(i))))
	}
	lister := &fakeManagedDatabaseLister{items: items, pageSize: managedDatabaseSeedPageSize}
	h := &recordingDBHandler{}

	if err := seedManagedDatabases(context.Background(), lister, h); err != nil {
		t.Fatalf("seedManagedDatabases: %v", err)
	}
	if len(h.events) != len(items) {
		t.Fatalf("seeded %d databases, want %d (all pages)", len(h.events), len(items))
	}
	if lister.calls < 2 {
		t.Fatalf("made %d list calls, want >= 2 (pagination)", lister.calls)
	}
}

// A list failure must propagate so watchLoop backs off and retries the seed,
// rather than proceeding to watch live traffic while a recoverable database stays
// unreconciled. No reconcile may be driven from a failed list.
func TestSeedManagedDatabases_ListErrorPropagates(t *testing.T) {
	lister := &fakeManagedDatabaseLister{err: errors.New("boom")}
	h := &recordingDBHandler{}

	if err := seedManagedDatabases(context.Background(), lister, h); err == nil {
		t.Fatal("want an error when ListManagedDatabases fails")
	}
	if len(h.events) != 0 {
		t.Fatalf("handled %d events on a failed list, want 0", len(h.events))
	}
}
