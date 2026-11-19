package openshellclient

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	openshellv1 "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/openshellv1"
)

// fakeDiscoveryServer is a real gRPC server registered on a bufconn listener, so
// the test exercises the generated client stubs, the bearer-token metadata and
// the stream semantics instead of a hand-written client double.
type fakeDiscoveryServer struct {
	openshellv1.UnimplementedServiceDiscoveryServer

	snapshot    *openshellv1.ServiceRouteSnapshot
	snapshotErr error

	// watchResponses are sent in order; watchErr is returned afterwards, so a
	// nil watchErr is a clean end of stream.
	watchResponses []*openshellv1.WatchServiceRoutesResponse
	watchErr       error

	snapshotWorkspaces []string
	watchWorkspaces    []string
	authHeaders        []string
}

func (f *fakeDiscoveryServer) GetServiceRouteSnapshot(ctx context.Context, req *openshellv1.GetServiceRouteSnapshotRequest) (*openshellv1.GetServiceRouteSnapshotResponse, error) {
	f.snapshotWorkspaces = append(f.snapshotWorkspaces, req.GetWorkspaceScope().GetWorkspace())
	f.recordAuth(ctx)
	if f.snapshotErr != nil {
		return nil, f.snapshotErr
	}
	return &openshellv1.GetServiceRouteSnapshotResponse{Snapshot: f.snapshot}, nil
}

func (f *fakeDiscoveryServer) WatchServiceRoutes(req *openshellv1.WatchServiceRoutesRequest, stream openshellv1.ServiceDiscovery_WatchServiceRoutesServer) error {
	f.watchWorkspaces = append(f.watchWorkspaces, req.GetWorkspaceScope().GetWorkspace())
	f.recordAuth(stream.Context())
	for _, resp := range f.watchResponses {
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
	return f.watchErr
}

func (f *fakeDiscoveryServer) recordAuth(ctx context.Context) {
	md, _ := metadata.FromIncomingContext(ctx)
	if vals := md.Get("authorization"); len(vals) > 0 {
		f.authHeaders = append(f.authHeaders, vals[0])
	}
}

// startDiscoveryServer serves srv on a bufconn listener and returns a Session
// dialed through the same generated client code a real gateway uses.
func startDiscoveryServer(t *testing.T, srv *fakeDiscoveryServer) *Session {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	openshellv1.RegisterServiceDiscoveryServer(server, srv)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	session := NewSessionWithConn(conn, "discovery-token")
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func discoveredRoute(hostname, sandbox string) *openshellv1.DiscoveredServiceRoute {
	return &openshellv1.DiscoveredServiceRoute{
		Workspace:           "gh-acme",
		Sandbox:             sandbox,
		Name:                "web",
		Hostname:            hostname,
		Endpoint:            &openshellv1.ServiceRouteEndpoint{Host: "10.128.2.11", Port: 9443},
		InstallationVersion: "installation-1",
		DeclarationRevision: 7,
	}
}

func snapshotWithRevision(revision string, routes ...*openshellv1.DiscoveredServiceRoute) *openshellv1.ServiceRouteSnapshot {
	return &openshellv1.ServiceRouteSnapshot{Routes: routes, Revision: revision}
}

// S0: the snapshot is authoritative and scope-explicit; the route fields the
// ingress reconcile needs (hostname, endpoint, installation identity) survive the
// wire intact.
func TestGetServiceRouteSnapshot_ReturnsAuthoritativeSnapshot(t *testing.T) {
	want := snapshotWithRevision("rev-1",
		discoveredRoute("ab12cd34ef56ab12.apps.example.com", "sbx-a"),
		discoveredRoute("cd34ef56ab12ab34.apps.example.com", "sbx-b"),
	)
	srv := &fakeDiscoveryServer{snapshot: want}
	session := startDiscoveryServer(t, srv)

	got, err := session.GetServiceRouteSnapshot(context.Background(), "gh-acme")
	if err != nil {
		t.Fatalf("GetServiceRouteSnapshot: %v", err)
	}
	if got.GetRevision() != "rev-1" {
		t.Fatalf("revision = %q, want rev-1", got.GetRevision())
	}
	routes := got.GetRoutes()
	if len(routes) != 2 {
		t.Fatalf("routes = %d, want 2", len(routes))
	}
	first := routes[0]
	if first.GetHostname() != "ab12cd34ef56ab12.apps.example.com" {
		t.Errorf("hostname = %q", first.GetHostname())
	}
	if first.GetEndpoint().GetHost() != "10.128.2.11" || first.GetEndpoint().GetPort() != 9443 {
		t.Errorf("endpoint = %s:%d", first.GetEndpoint().GetHost(), first.GetEndpoint().GetPort())
	}
	if first.GetInstallationVersion() != "installation-1" || first.GetDeclarationRevision() != 7 {
		t.Errorf("installation identity = %q/%d", first.GetInstallationVersion(), first.GetDeclarationRevision())
	}
	if len(srv.snapshotWorkspaces) != 1 || srv.snapshotWorkspaces[0] != "gh-acme" {
		t.Errorf("workspace selectors sent = %v, want exactly [gh-acme]", srv.snapshotWorkspaces)
	}
	if len(srv.authHeaders) != 1 || srv.authHeaders[0] != "Bearer discovery-token" {
		t.Errorf("authorization metadata = %v", srv.authHeaders)
	}
}

// A discovery error is never an empty replacement: the client reports the error
// and hands back no snapshot for the caller to mistake for "no routes".
func TestGetServiceRouteSnapshot_ErrorIsNotAnEmptySnapshot(t *testing.T) {
	srv := &fakeDiscoveryServer{snapshotErr: status.Error(codes.ResourceExhausted, "watch admission exhausted")}
	session := startDiscoveryServer(t, srv)

	got, err := session.GetServiceRouteSnapshot(context.Background(), "gh-acme")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got != nil {
		t.Fatalf("snapshot = %v, want nil on error", got)
	}
	if status.Code(errors.Unwrap(err)) != codes.ResourceExhausted {
		t.Fatalf("error %v does not wrap ResourceExhausted", err)
	}
}

func TestServiceDiscovery_RejectsEmptyWorkspaceWithoutCallingGateway(t *testing.T) {
	srv := &fakeDiscoveryServer{}
	session := startDiscoveryServer(t, srv)

	if _, err := session.GetServiceRouteSnapshot(context.Background(), ""); err == nil {
		t.Fatal("GetServiceRouteSnapshot with an empty workspace should fail")
	}
	if _, err := session.WatchServiceRoutes(context.Background(), ""); err == nil {
		t.Fatal("WatchServiceRoutes with an empty workspace should fail")
	}
	if len(srv.snapshotWorkspaces) != 0 || len(srv.watchWorkspaces) != 0 {
		t.Fatalf("gateway was called with an empty selector: %v %v", srv.snapshotWorkspaces, srv.watchWorkspaces)
	}
}

// The watch delivers replacement views in order and a clean end of stream (the
// bounded watch lifetime expiring) surfaces as io.EOF, which the sync loop treats
// as "relist now", not as an error.
func TestWatchServiceRoutes_DeliversReplacementsUntilCleanEndOfStream(t *testing.T) {
	srv := &fakeDiscoveryServer{watchResponses: []*openshellv1.WatchServiceRoutesResponse{
		{Snapshot: snapshotWithRevision("rev-1", discoveredRoute("ab12cd34ef56ab12.apps.example.com", "sbx-a"))},
		{Snapshot: snapshotWithRevision("rev-2",
			discoveredRoute("ab12cd34ef56ab12.apps.example.com", "sbx-a"),
			discoveredRoute("cd34ef56ab12ab34.apps.example.com", "sbx-b"),
		)},
	}}
	session := startDiscoveryServer(t, srv)

	stream, err := session.WatchServiceRoutes(context.Background(), "gh-acme")
	if err != nil {
		t.Fatalf("WatchServiceRoutes: %v", err)
	}

	var revisions []string
	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		revisions = append(revisions, resp.GetSnapshot().GetRevision())
	}
	if len(revisions) != 2 || revisions[0] != "rev-1" || revisions[1] != "rev-2" {
		t.Fatalf("revisions = %v, want [rev-1 rev-2]", revisions)
	}
	if len(srv.watchWorkspaces) != 1 || srv.watchWorkspaces[0] != "gh-acme" {
		t.Errorf("watch workspace selectors = %v, want exactly [gh-acme]", srv.watchWorkspaces)
	}
	if len(srv.authHeaders) != 1 || srv.authHeaders[0] != "Bearer discovery-token" {
		t.Errorf("authorization metadata = %v", srv.authHeaders)
	}
}

// A stream that fails mid-watch must surface the failure instead of looking like
// a finished watch, so the caller backs off and keeps its last known state.
func TestWatchServiceRoutes_StreamErrorIsNotACleanEnd(t *testing.T) {
	srv := &fakeDiscoveryServer{
		watchResponses: []*openshellv1.WatchServiceRoutesResponse{
			{Snapshot: snapshotWithRevision("rev-1", discoveredRoute("ab12cd34ef56ab12.apps.example.com", "sbx-a"))},
		},
		watchErr: status.Error(codes.PermissionDenied, "token revoked"),
	}
	session := startDiscoveryServer(t, srv)

	stream, err := session.WatchServiceRoutes(context.Background(), "gh-acme")
	if err != nil {
		t.Fatalf("WatchServiceRoutes: %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("first Recv: %v", err)
	}
	_, err = stream.Recv()
	if errors.Is(err, io.EOF) {
		t.Fatal("a failed watch must not look like a clean end of stream")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("error = %v, want PermissionDenied", err)
	}
}
