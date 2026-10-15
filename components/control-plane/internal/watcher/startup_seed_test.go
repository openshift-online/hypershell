package watcher

import (
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// startupGatewayServer is a GatewayService that already holds an inventory and
// whose watch stream subscribes (flushes its header) but never sends an event.
// Anything the control plane reconciles from it must therefore come from the
// startup seed LIST, not from the watch.
type startupGatewayServer struct {
	pb.UnimplementedGatewayServiceServer
	inventory *fakeGatewayLister
	watched   chan *string
}

func (s *startupGatewayServer) ListGateways(ctx context.Context, in *pb.ListGatewaysRequest) (*pb.ListGatewaysResponse, error) {
	return s.inventory.ListGateways(ctx, in)
}

func (s *startupGatewayServer) GetGateway(ctx context.Context, in *pb.GetGatewayRequest) (*pb.GetGatewayResponse, error) {
	return s.inventory.GetGateway(ctx, in)
}

func (s *startupGatewayServer) WatchGateways(req *pb.WatchGatewaysRequest, stream pb.GatewayService_WatchGatewaysServer) error {
	if err := stream.SendHeader(nil); err != nil {
		return err
	}
	s.watched <- req.ClusterId
	<-stream.Context().Done()
	return stream.Context().Err()
}

func newGatewayServiceConn(t *testing.T, srv pb.GatewayServiceServer) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterGatewayServiceServer(server, srv)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial gateway server: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// A freshly started control plane must reconcile gateways that already exist
// when it connects, such as one created while the controller was down. The watch
// stream never replays existing state, so this drives the real WatchGateways
// entry point against a server that never sends an event and checks the
// pre-existing gateway still reaches the reconcile handler. Unlike the
// seedGateways unit tests, it fails if WatchGateways stops calling the seed.
func TestWatchGateways_StartupSeedsExistingGateways(t *testing.T) {
	srv := &startupGatewayServer{
		inventory: &fakeGatewayLister{items: []*pb.Gateway{gw("gw-missed", "")}},
		watched:   make(chan *string, 1),
	}
	conn := newGatewayServiceConn(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	h := &gatewayRecordingHandler{}
	q := NewGatewayReconcileQueue(ctx, h, 1)
	done := make(chan error, 1)
	go func() { done <- WatchGateways(ctx, conn, q, "cluster-x", 0) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("WatchGateways did not return after cancel")
		}
		q.Stop()
	})

	select {
	case got := <-srv.watched:
		if got == nil || *got != "cluster-x" {
			t.Fatalf("WatchGateways cluster_id = %v, want %q", got, "cluster-x")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("WatchGateways never opened the stream")
	}

	seen := waitForGatewayCalls(t, h, 1)
	if id := seen[0].GetMetadata().GetId(); id != "gw-missed" {
		t.Fatalf("first reconciled gateway = %q, want %q", id, "gw-missed")
	}
}
