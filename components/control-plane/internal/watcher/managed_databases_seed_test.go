package watcher

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

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

// recordingDBHandler captures the events seedManagedDatabases enqueues, so the
// seed can be asserted without a real reconciler.
type recordingDBHandler struct {
	mu     sync.Mutex
	events []Event[*pb.ManagedDatabase]
}

func (h *recordingDBHandler) enqueue(ev Event[*pb.ManagedDatabase]) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, ev)
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
		t.Fatalf("enqueued %d events, want all 3 seeded", len(h.events))
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
type changingManagedDatabaseLister struct {
	pb.ManagedDatabaseServiceClient
	calls int
}

func (f *changingManagedDatabaseLister) ListManagedDatabases(_ context.Context, in *pb.ListManagedDatabasesRequest, _ ...grpc.CallOption) (*pb.ListManagedDatabasesResponse, error) {
	f.calls++
	item := md(fmt.Sprintf("db-pass-%d", f.calls))
	return &pb.ListManagedDatabasesResponse{
		Items:    []*pb.ManagedDatabase{item},
		Metadata: &pb.ListMeta{Page: in.Page, Size: in.Size, Total: 1},
	}, nil
}

func TestListAllManagedDatabases_UnstableInventoryRetriesThenFails(t *testing.T) {
	lister := &changingManagedDatabaseLister{}
	if _, err := listAllManagedDatabases(context.Background(), lister); err == nil {
		t.Fatal("want unstable inventory error")
	}
	if lister.calls != 5 {
		t.Fatalf("list calls=%d, want 5 stabilization attempts", lister.calls)
	}
}

func TestSeedManagedDatabases_ListErrorPropagates(t *testing.T) {
	lister := &fakeManagedDatabaseLister{err: errors.New("boom")}
	h := &recordingDBHandler{}

	if err := seedManagedDatabases(context.Background(), lister, h); err == nil {
		t.Fatal("want an error when ListManagedDatabases fails")
	}
	if len(h.events) != 0 {
		t.Fatalf("enqueued %d events on a failed list, want 0", len(h.events))
	}
}

type retryingManagedDatabaseHandler struct {
	mu    sync.Mutex
	calls int
	done  chan struct{}
}

func (h *retryingManagedDatabaseHandler) Handle(_ context.Context, _ Event[*pb.ManagedDatabase]) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls++
	if h.calls == 1 {
		return errors.New("transient cleanup failure")
	}
	if h.calls == 2 {
		close(h.done)
	}
	return nil
}

func (h *retryingManagedDatabaseHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

// A failed delete must be retried from its retained tombstone without another
// API watch event; delete events are not replayed by the broker.
func TestManagedDatabaseReconcileQueue_RetriesDeleteUntilCleanupSucceeds(t *testing.T) {
	h := &retryingManagedDatabaseHandler{done: make(chan struct{})}
	q := newManagedDatabaseReconcileQueue(context.Background(), h,
		withRateLimiter[*pb.ManagedDatabase](fastLimiter()),
		withWorkers[*pb.ManagedDatabase](1),
	)
	defer q.stop()

	q.enqueue(Event[*pb.ManagedDatabase]{
		Type:       EventDeleted,
		ResourceID: "db-1",
		Resource: &pb.ManagedDatabase{
			Metadata:  &pb.ObjectReference{Id: "db-1"},
			Provider:  "deployment",
			Namespace: "openshell-db-test",
		},
	})

	select {
	case <-h.done:
	case <-time.After(2 * time.Second):
		t.Fatalf("delete cleanup was not retried; handler calls=%d", h.count())
	}
	if got := h.count(); got != 2 {
		t.Fatalf("handler calls=%d, want one failure and one successful retry", got)
	}
}
