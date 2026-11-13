package watcher

import (
	"context"
	"net"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// listCIProviderServer serves a fixed CIProvider inventory across paginated
// ListCIProviders calls so seedCIProviders can be exercised without a real API
// server. CIProvider is platform-scoped, which is structurally guaranteed because
// ListCIProvidersRequest has no cluster_id field.
type listCIProviderServer struct {
	pb.UnimplementedCIProviderServiceServer
	items    []*pb.CIProvider
	pageSize int32
	listErr  error
	calls    int
}

func (s *listCIProviderServer) ListCIProviders(_ context.Context, req *pb.ListCIProvidersRequest) (*pb.ListCIProvidersResponse, error) {
	s.calls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	size := int(s.pageSize)
	if size == 0 {
		size = int(req.GetSize())
	}
	start := (int(req.GetPage()) - 1) * size
	total := len(s.items)
	meta := &pb.ListMeta{Page: req.GetPage(), Size: req.GetSize(), Total: int32(total)}
	if start >= total {
		return &pb.ListCIProvidersResponse{Metadata: meta}, nil
	}
	end := start + size
	if end > total {
		end = total
	}
	return &pb.ListCIProvidersResponse{Items: s.items[start:end], Metadata: meta}, nil
}

func newCIProviderWatchConn(t *testing.T, srv pb.CIProviderServiceServer) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterCIProviderServiceServer(server, srv)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial CI provider server: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// recordingCPSink captures the events seedCIProviders enqueues.
type recordingCPSink struct {
	events []Event[*pb.CIProvider]
}

func (s *recordingCPSink) enqueue(ev Event[*pb.CIProvider]) { s.events = append(s.events, ev) }

func cpWithID(id string) *pb.CIProvider {
	return &pb.CIProvider{Metadata: &pb.ObjectReference{Id: id}}
}

// seedCIProviders is the LIST half of the CI provider list-then-watch: it must
// enqueue every listed provider as an update so a provider whose create event the
// watch missed -- or that predates the reconciler -- is realized.
// Verifies: CTRL-CORE-21.01
func TestSeedCIProviders_EnqueuesEveryProvider(t *testing.T) {
	srv := &listCIProviderServer{items: []*pb.CIProvider{cpWithID("cp-1"), cpWithID("cp-2"), cpWithID("cp-3")}}
	conn := newCIProviderWatchConn(t, srv)
	client := pb.NewCIProviderServiceClient(conn)
	sink := &recordingCPSink{}

	if err := seedCIProviders(context.Background(), client, sink); err != nil {
		t.Fatalf("seedCIProviders: %v", err)
	}

	wantIDs := []string{"cp-1", "cp-2", "cp-3"}
	if len(sink.events) != len(wantIDs) {
		t.Fatalf("enqueued %d events, want %d", len(sink.events), len(wantIDs))
	}
	for i, ev := range sink.events {
		if ev.Type != EventUpdated {
			t.Errorf("event %d type = %v, want EventUpdated", i, ev.Type)
		}
		if ev.ResourceID != wantIDs[i] {
			t.Errorf("event %d id = %q, want %q", i, ev.ResourceID, wantIDs[i])
		}
	}
}

// A CIProvider inventory larger than one page must be seeded in full.
func TestSeedCIProviders_Paginates(t *testing.T) {
	var items []*pb.CIProvider
	for i := 0; i < ciProviderSeedPageSize+7; i++ {
		items = append(items, cpWithID(string(rune('a'+i%26))+string(rune(i))))
	}
	srv := &listCIProviderServer{items: items, pageSize: ciProviderSeedPageSize}
	conn := newCIProviderWatchConn(t, srv)
	client := pb.NewCIProviderServiceClient(conn)
	sink := &recordingCPSink{}

	if err := seedCIProviders(context.Background(), client, sink); err != nil {
		t.Fatalf("seedCIProviders: %v", err)
	}
	if len(sink.events) != len(items) {
		t.Fatalf("seeded %d providers, want %d (all pages)", len(sink.events), len(items))
	}
	if srv.calls < 2 {
		t.Fatalf("made %d list calls, want >= 2 (pagination)", srv.calls)
	}
}

// A list failure is propagated so the caller (initial seed) aborts the attempt and
// reconnects rather than treating an empty result as authoritative.
func TestSeedCIProviders_ListErrorPropagates(t *testing.T) {
	srv := &listCIProviderServer{listErr: status.Error(codes.Unavailable, "boom")}
	conn := newCIProviderWatchConn(t, srv)
	client := pb.NewCIProviderServiceClient(conn)
	sink := &recordingCPSink{}

	if err := seedCIProviders(context.Background(), client, sink); err == nil {
		t.Fatal("seedCIProviders should propagate a list error")
	}
	if len(sink.events) != 0 {
		t.Fatalf("no providers should be enqueued on a list error, got %d", len(sink.events))
	}
}
