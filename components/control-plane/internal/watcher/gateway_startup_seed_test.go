package watcher

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// silentGatewayServer answers ListGateways and GetGateway from a fixed inventory
// and, in WatchGateways, sends the subscription header and then blocks without
// ever sending an event. A gateway can therefore reach the reconcile queue only
// through the startup seed, never through the watch stream. It also records
// whether a ListGateways call arrived before the subscription header was sent.
type silentGatewayServer struct {
	pb.UnimplementedGatewayServiceServer
	inventory    *fakeGatewayLister
	headerSent   atomic.Bool
	listTooEarly atomic.Bool
}

func (s *silentGatewayServer) ListGateways(ctx context.Context, in *pb.ListGatewaysRequest) (*pb.ListGatewaysResponse, error) {
	if !s.headerSent.Load() {
		s.listTooEarly.Store(true)
	}
	return s.inventory.ListGateways(ctx, in)
}

func (s *silentGatewayServer) GetGateway(ctx context.Context, in *pb.GetGatewayRequest) (*pb.GetGatewayResponse, error) {
	return s.inventory.GetGateway(ctx, in)
}

func (s *silentGatewayServer) WatchGateways(_ *pb.WatchGatewaysRequest, stream pb.GatewayService_WatchGatewaysServer) error {
	// Delay the header so a seed issued before the subscription is live would
	// reach ListGateways first and be detected.
	time.Sleep(50 * time.Millisecond)
	s.headerSent.Store(true)
	if err := stream.SendHeader(metadata.MD{}); err != nil {
		return err
	}
	<-stream.Context().Done()
	return nil
}

func newGatewayWatchConn(t *testing.T, srv pb.GatewayServiceServer) *grpc.ClientConn {
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

// A gateway created while the control plane was down must be reconciled when the
// control plane starts, after the watch subscription is live and without
// waiting for a watch event (control-plane.spec.md,
// "Startup seeds gateways created while the control plane was down"). This runs
// the real WatchGateways entry point, so it fails if WatchGateways stops seeding;
// the seedGateways tests call the seed directly and would not notice.
func TestWatchGateways_StartupSeedsPreExistingGateway(t *testing.T) {
	srv := &silentGatewayServer{inventory: &fakeGatewayLister{items: []*pb.Gateway{gw("gw-pre-existing", "")}}}
	conn := newGatewayWatchConn(t, srv)

	h := &gatewayRecordingHandler{}
	queue := NewGatewayReconcileQueue(context.Background(), h, 1)
	defer queue.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- WatchGateways(ctx, conn, queue, "cluster-x", 0) }()

	got := waitForGatewayCalls(t, h, 1)
	if id := got[0].GetMetadata().GetId(); id != "gw-pre-existing" {
		t.Fatalf("reconciled gateway = %q, want %q", id, "gw-pre-existing")
	}
	if srv.listTooEarly.Load() {
		t.Fatal("WatchGateways listed gateways before the watch subscription header arrived")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("WatchGateways did not return after cancel")
	}
}
