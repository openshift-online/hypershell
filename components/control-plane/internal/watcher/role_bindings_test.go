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
	go func() { done <- WatchRoleBindings(ctx, conn, noopRoleBindingHandler{}, "cluster-x", 0, 0) }()

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

// listRoleBindingServer serves a fixed ListRoleBindings response and records the
// cluster_id each call carried, so seedRoleBindings can be exercised without a
// real API server.
type listRoleBindingServer struct {
	pb.UnimplementedRoleBindingServiceServer
	items     []*pb.RoleBinding
	listErr   error
	gotFilter chan *string
}

func (s *listRoleBindingServer) ListRoleBindings(_ context.Context, req *pb.ListRoleBindingsRequest) (*pb.ListRoleBindingsResponse, error) {
	if s.gotFilter != nil {
		s.gotFilter <- req.ClusterId
	}
	if s.listErr != nil {
		return nil, s.listErr
	}
	return &pb.ListRoleBindingsResponse{Items: s.items}, nil
}

// recordingRBSink captures the events seedRoleBindings enqueues.
type recordingRBSink struct {
	events []Event[*pb.RoleBinding]
}

func (s *recordingRBSink) enqueue(ev Event[*pb.RoleBinding]) { s.events = append(s.events, ev) }

func rbWithID(id string) *pb.RoleBinding {
	return &pb.RoleBinding{Metadata: &pb.ObjectReference{Id: id}}
}

// seedRoleBindings is the LIST half of the role binding list-then-watch: it must
// enqueue every listed binding as an update so the owner grant is recovered even
// when the watch stream missed the create event.
func TestSeedRoleBindings_EnqueuesEveryBinding(t *testing.T) {
	srv := &listRoleBindingServer{
		items:     []*pb.RoleBinding{rbWithID("rb-1"), rbWithID("rb-2"), rbWithID("rb-3")},
		gotFilter: make(chan *string, 1),
	}
	conn := newRoleBindingWatchConn(t, srv)
	client := pb.NewRoleBindingServiceClient(conn)
	sink := &recordingRBSink{}

	if err := seedRoleBindings(context.Background(), client, sink, "cluster-x"); err != nil {
		t.Fatalf("seedRoleBindings: %v", err)
	}

	// Every listed binding is enqueued as an update.
	wantIDs := []string{"rb-1", "rb-2", "rb-3"}
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

	select {
	case got := <-srv.gotFilter:
		if got == nil || *got != "cluster-x" {
			t.Errorf("list cluster_id = %v, want %q", got, "cluster-x")
		}
	default:
		t.Error("ListRoleBindings was never called")
	}
}

// The resync is cluster-scoped and mandatory: an empty identity is refused before
// any list, so it can never issue an unfiltered fleet-wide list.
// Verifies: CTRL-CORE-08.03
func TestSeedRoleBindings_RequiresClusterID(t *testing.T) {
	srv := &listRoleBindingServer{gotFilter: make(chan *string, 1)}
	conn := newRoleBindingWatchConn(t, srv)
	client := pb.NewRoleBindingServiceClient(conn)

	err := seedRoleBindings(context.Background(), client, &recordingRBSink{}, "")
	if !errors.Is(err, ErrMissingClusterID) {
		t.Fatalf("seedRoleBindings with empty cluster id: err = %v, want ErrMissingClusterID", err)
	}
	select {
	case got := <-srv.gotFilter:
		t.Fatalf("list issued with cluster_id %v; an empty identity must never list", got)
	default:
	}
}

// A list failure is propagated so the caller (initial seed) aborts the attempt and
// reconnects rather than treating an empty result as authoritative.
func TestSeedRoleBindings_ListErrorPropagates(t *testing.T) {
	srv := &listRoleBindingServer{listErr: status.Error(codes.Unavailable, "boom")}
	conn := newRoleBindingWatchConn(t, srv)
	client := pb.NewRoleBindingServiceClient(conn)
	sink := &recordingRBSink{}

	if err := seedRoleBindings(context.Background(), client, sink, "cluster-x"); err == nil {
		t.Fatal("seedRoleBindings should propagate a list error")
	}
	if len(sink.events) != 0 {
		t.Fatalf("no bindings should be enqueued on a list error, got %d", len(sink.events))
	}
}

// There is no unfiltered mode: an empty identity is refused before any stream
// is opened.
func TestWatchRoleBindings_RequiresClusterID(t *testing.T) {
	srv := &recordingRoleBindingServer{opened: make(chan *string, 1)}
	conn := newRoleBindingWatchConn(t, srv)

	err := WatchRoleBindings(context.Background(), conn, noopRoleBindingHandler{}, "", 0, 0)
	if !errors.Is(err, ErrMissingClusterID) {
		t.Fatalf("WatchRoleBindings with empty cluster id: err = %v, want ErrMissingClusterID", err)
	}
	select {
	case got := <-srv.opened:
		t.Fatalf("stream opened with cluster_id %v; an empty identity must never watch", got)
	default:
	}
}
