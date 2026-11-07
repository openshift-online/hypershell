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

// listCITrustServer serves a fixed CITrust inventory across paginated ListCITrusts
// calls so seedCITrusts can be exercised without a real API server. It records that
// the request carries no cluster filter (CITrust is platform-scoped), which is
// structurally guaranteed because ListCITrustsRequest has no cluster_id field.
type listCITrustServer struct {
	pb.UnimplementedCITrustServiceServer
	items    []*pb.CITrust
	pageSize int32
	listErr  error
	calls    int
}

func (s *listCITrustServer) ListCITrusts(_ context.Context, req *pb.ListCITrustsRequest) (*pb.ListCITrustsResponse, error) {
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
		return &pb.ListCITrustsResponse{Metadata: meta}, nil
	}
	end := start + size
	if end > total {
		end = total
	}
	return &pb.ListCITrustsResponse{Items: s.items[start:end], Metadata: meta}, nil
}

func newCITrustWatchConn(t *testing.T, srv pb.CITrustServiceServer) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterCITrustServiceServer(server, srv)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial CI trust server: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// recordingCTSink captures the events seedCITrusts enqueues.
type recordingCTSink struct {
	events []Event[*pb.CITrust]
}

func (s *recordingCTSink) enqueue(ev Event[*pb.CITrust]) { s.events = append(s.events, ev) }

func ctWithID(id string) *pb.CITrust {
	return &pb.CITrust{Metadata: &pb.ObjectReference{Id: id}}
}

// seedCITrusts is the LIST half of the CI trust list-then-watch: it must enqueue
// every listed trust as an update so a trust whose create event the watch missed --
// or that predates the reconciler -- is realized.
// Verifies: CTRL-CORE-16.01
func TestSeedCITrusts_EnqueuesEveryTrust(t *testing.T) {
	srv := &listCITrustServer{items: []*pb.CITrust{ctWithID("ct-1"), ctWithID("ct-2"), ctWithID("ct-3")}}
	conn := newCITrustWatchConn(t, srv)
	client := pb.NewCITrustServiceClient(conn)
	sink := &recordingCTSink{}

	if err := seedCITrusts(context.Background(), client, sink); err != nil {
		t.Fatalf("seedCITrusts: %v", err)
	}

	wantIDs := []string{"ct-1", "ct-2", "ct-3"}
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

// A CITrust inventory larger than one page must be seeded in full: the platform
// list is paginated (DefaultPageSize=20 server-side), so a single page would strand
// trusts beyond it.
func TestSeedCITrusts_Paginates(t *testing.T) {
	var items []*pb.CITrust
	for i := 0; i < ciTrustSeedPageSize+7; i++ {
		items = append(items, ctWithID(string(rune('a'+i%26))+string(rune(i))))
	}
	srv := &listCITrustServer{items: items, pageSize: ciTrustSeedPageSize}
	conn := newCITrustWatchConn(t, srv)
	client := pb.NewCITrustServiceClient(conn)
	sink := &recordingCTSink{}

	if err := seedCITrusts(context.Background(), client, sink); err != nil {
		t.Fatalf("seedCITrusts: %v", err)
	}
	if len(sink.events) != len(items) {
		t.Fatalf("seeded %d trusts, want %d (all pages)", len(sink.events), len(items))
	}
	if srv.calls < 2 {
		t.Fatalf("made %d list calls, want >= 2 (pagination)", srv.calls)
	}
}

// A list failure is propagated so the caller (initial seed) aborts the attempt and
// reconnects rather than treating an empty result as authoritative.
func TestSeedCITrusts_ListErrorPropagates(t *testing.T) {
	srv := &listCITrustServer{listErr: status.Error(codes.Unavailable, "boom")}
	conn := newCITrustWatchConn(t, srv)
	client := pb.NewCITrustServiceClient(conn)
	sink := &recordingCTSink{}

	if err := seedCITrusts(context.Background(), client, sink); err == nil {
		t.Fatal("seedCITrusts should propagate a list error")
	}
	if len(sink.events) != 0 {
		t.Fatalf("no trusts should be enqueued on a list error, got %d", len(sink.events))
	}
}
