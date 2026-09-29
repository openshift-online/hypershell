package watcher

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestIsMissingKeycloakClient(t *testing.T) {
	notFound := fmt.Errorf("assign role: %w", &keycloak.ClientNotFoundError{ClientID: "gateway-1"})
	if !isMissingKeycloakClient(notFound) {
		t.Fatal("a wrapped client-not-found error must be retried")
	}
	if isMissingKeycloakClient(errors.New("permission denied")) {
		t.Fatal("a permanent error must not be retried")
	}
}

func TestIsRoleBindingRetryable(t *testing.T) {
	notFound := fmt.Errorf("assign role: %w", &keycloak.ClientNotFoundError{ClientID: "gateway-1"})
	if !isRoleBindingRetryable(notFound) {
		t.Fatal("a missing Keycloak client must be retried")
	}
	if !isRoleBindingRetryable(errors.New("role binding rb-1: username not yet resolved")) {
		t.Fatal("an unresolved username must be retried")
	}
	if !isRoleBindingRetryable(fmt.Errorf("get role UUID: keycloak GET returned 404")) {
		t.Fatal("a half-provisioned Keycloak client (role 404) must be retried")
	}
	if !isRoleBindingRetryable(errors.New("role binding rb-1: role name not yet resolved")) {
		t.Fatal("an unresolved role name must be retried")
	}

	gatewayMissing := status.Error(codes.NotFound, "Gateway with id='gw-1' not found")
	wrappedGateway := fmt.Errorf("get gateway gw-1: %w", gatewayMissing)
	if isRoleBindingRetryable(wrappedGateway) {
		t.Fatal("a missing gateway must not be retried")
	}
	if isRoleBindingRetryable(nil) {
		t.Fatal("a nil error must not be retried")
	}
	if isRoleBindingRetryable(errors.New("permission denied")) {
		t.Fatal("an auth error must not be retried")
	}
	if isRoleBindingRetryable(fmt.Errorf("authenticate to keycloak: %w", errors.New("invalid client credentials"))) {
		t.Fatal("a Keycloak credential error must not be retried")
	}
}

// recordingRoleBindingServer records the cluster_id each WatchRoleBindings
// stream is opened with, then holds the stream open until the client leaves.
type recordingRoleBindingServer struct {
	pb.UnimplementedRoleBindingServiceServer
	opened chan *string
}

func (s *recordingRoleBindingServer) WatchRoleBindings(req *pb.WatchRoleBindingsRequest, stream pb.RoleBindingService_WatchRoleBindingsServer) error {
	s.opened <- req.ClusterId
	<-stream.Context().Done()
	return stream.Context().Err()
}

func newRoleBindingWatchConn(t *testing.T, srv pb.RoleBindingServiceServer) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterRoleBindingServiceServer(server, srv)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial role binding server: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

type noopRoleBindingHandler struct{}

func (noopRoleBindingHandler) Handle(context.Context, Event[*pb.RoleBinding]) error { return nil }

// The RoleBinding watch is scoped server-side to this control plane's cluster:
// the stream must be opened with its registered cluster_id.
func TestWatchRoleBindings_SendsClusterID(t *testing.T) {
	srv := &recordingRoleBindingServer{opened: make(chan *string, 1)}
	conn := newRoleBindingWatchConn(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- WatchRoleBindings(ctx, conn, noopRoleBindingHandler{}, "cluster-x") }()

	select {
	case got := <-srv.opened:
		if got == nil || *got != "cluster-x" {
			t.Fatalf("WatchRoleBindings cluster_id = %v, want %q", got, "cluster-x")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("WatchRoleBindings never opened the stream")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("WatchRoleBindings did not return after cancel")
	}
}

// There is no unfiltered mode: an empty identity is refused before any stream
// is opened.
func TestWatchRoleBindings_RequiresClusterID(t *testing.T) {
	srv := &recordingRoleBindingServer{opened: make(chan *string, 1)}
	conn := newRoleBindingWatchConn(t, srv)

	err := WatchRoleBindings(context.Background(), conn, noopRoleBindingHandler{}, "")
	if !errors.Is(err, ErrMissingClusterID) {
		t.Fatalf("WatchRoleBindings with empty cluster id: err = %v, want ErrMissingClusterID", err)
	}
	select {
	case got := <-srv.opened:
		t.Fatalf("stream opened with cluster_id %v; an empty identity must never watch", got)
	default:
	}
}
