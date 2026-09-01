package reconciler

import (
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// managedDatabaseNotFoundServer answers GetManagedDatabase with NotFound to
// simulate a ManagedDatabase that has already been deleted by the time the
// gateway delete-reconcile resolves its database config.
type managedDatabaseNotFoundServer struct {
	pb.UnimplementedManagedDatabaseServiceServer
}

func (managedDatabaseNotFoundServer) GetManagedDatabase(context.Context, *pb.GetManagedDatabaseRequest) (*pb.GetManagedDatabaseResponse, error) {
	return nil, status.Error(codes.NotFound, "ManagedDatabase with id='e2e-db' not found")
}

// dialManagedDatabaseNotFound stands up an in-process gRPC server whose
// ManagedDatabaseService always returns NotFound and returns a client
// connection to it.
func dialManagedDatabaseNotFound(t *testing.T) *grpc.ClientConn {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for test: %v", err)
	}
	server := grpc.NewServer()
	pb.RegisterManagedDatabaseServiceServer(server, managedDatabaseNotFoundServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial managed database service: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// TestGatewayDeleteWithAlreadyDeletedDatabaseIsIdempotent covers P1-1: deleting
// a gateway whose ManagedDatabase is already gone must finalize cleanly instead
// of failing the reconcile forever. Before the fix the NotFound was appended to
// deleteErrs and returned, so the watcher retried every 30s indefinitely because
// the ManagedDatabase never comes back.
func TestGatewayDeleteWithAlreadyDeletedDatabaseIsIdempotent(t *testing.T) {
	conn := dialManagedDatabaseNotFound(t)

	r := &GatewayReconciler{
		active:   make(map[string]struct{}),
		grpcConn: conn,
	}

	// No namespace -> gatewayNamespace() errors, which skips all K8s cleanup
	// (dynamicClient/clientset are nil here) and leaves only the ManagedDatabase
	// resolution, which is exactly the code path P1-1 fixes.
	gw := &pb.Gateway{
		Metadata:   &pb.ObjectReference{Id: "3IPluBbA8q2mMMMAZVvVxz1FGeN"},
		Name:       "e2e-gw",
		DatabaseId: "e2e-db",
	}
	event := watcher.Event[*pb.Gateway]{
		Type:       watcher.EventDeleted,
		ResourceID: "3IPluBbA8q2mMMMAZVvVxz1FGeN",
		Resource:   gw,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.Handle(ctx, event); err != nil {
		t.Fatalf("delete of gateway with already-deleted ManagedDatabase must be idempotent, got: %v", err)
	}
}

// TestGatewayDeleteDatabaseResolveErrorStillFails ensures the idempotency carve
// out is scoped to NotFound: a non-NotFound resolve error still fails the
// reconcile so the watcher retries a transient condition.
func TestGatewayDeleteDatabaseResolveErrorStillFails(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for test: %v", err)
	}
	server := grpc.NewServer()
	pb.RegisterManagedDatabaseServiceServer(server, managedDatabaseUnavailableServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial managed database service: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	r := &GatewayReconciler{
		active:   make(map[string]struct{}),
		grpcConn: conn,
	}
	gw := &pb.Gateway{
		Metadata:   &pb.ObjectReference{Id: "gw-transient"},
		Name:       "e2e-gw",
		DatabaseId: "e2e-db",
	}
	event := watcher.Event[*pb.Gateway]{
		Type:       watcher.EventDeleted,
		ResourceID: "gw-transient",
		Resource:   gw,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.Handle(ctx, event); err == nil {
		t.Fatal("non-NotFound database resolve error during delete must fail so the watcher retries")
	}
}

// managedDatabaseUnavailableServer returns a transient (retryable) error rather
// than NotFound.
type managedDatabaseUnavailableServer struct {
	pb.UnimplementedManagedDatabaseServiceServer
}

func (managedDatabaseUnavailableServer) GetManagedDatabase(context.Context, *pb.GetManagedDatabaseRequest) (*pb.GetManagedDatabaseResponse, error) {
	return nil, status.Error(codes.Unavailable, "database service temporarily unavailable")
}
