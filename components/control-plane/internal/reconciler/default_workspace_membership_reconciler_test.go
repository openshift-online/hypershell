package reconciler

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
)

// recordingDefaultWorkspaceMembershipServer is a minimal fake
// DefaultWorkspaceMembershipService: it serves a scripted List response and
// records every UpdateDefaultWorkspaceMembershipStatus call so a test can
// assert the reconciler's status write-back without a real API server.
type recordingDefaultWorkspaceMembershipServer struct {
	pb.UnimplementedDefaultWorkspaceMembershipServiceServer

	mu            sync.Mutex
	items         []*pb.DefaultWorkspaceMembership
	listErr       error
	statusUpdates []*pb.UpdateDefaultWorkspaceMembershipStatusRequest
}

func (s *recordingDefaultWorkspaceMembershipServer) ListDefaultWorkspaceMemberships(_ context.Context, req *pb.ListDefaultWorkspaceMembershipsRequest) (*pb.ListDefaultWorkspaceMembershipsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	var out []*pb.DefaultWorkspaceMembership
	for _, item := range s.items {
		if req.GatewayId != nil && item.GetGatewayId() != req.GetGatewayId() {
			continue
		}
		out = append(out, item)
	}
	return &pb.ListDefaultWorkspaceMembershipsResponse{Items: out}, nil
}

func (s *recordingDefaultWorkspaceMembershipServer) UpdateDefaultWorkspaceMembershipStatus(_ context.Context, req *pb.UpdateDefaultWorkspaceMembershipStatusRequest) (*pb.UpdateDefaultWorkspaceMembershipStatusResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusUpdates = append(s.statusUpdates, req)
	return &pb.UpdateDefaultWorkspaceMembershipStatusResponse{}, nil
}

func (s *recordingDefaultWorkspaceMembershipServer) setItems(items ...*pb.DefaultWorkspaceMembership) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = items
}

func (s *recordingDefaultWorkspaceMembershipServer) snapshotStatusUpdates() []*pb.UpdateDefaultWorkspaceMembershipStatusRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*pb.UpdateDefaultWorkspaceMembershipStatusRequest(nil), s.statusUpdates...)
}

func newRecordingDefaultWorkspaceMembershipConn(t *testing.T) (*grpc.ClientConn, *recordingDefaultWorkspaceMembershipServer) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	svc := &recordingDefaultWorkspaceMembershipServer{}
	pb.RegisterDefaultWorkspaceMembershipServiceServer(server, svc)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial recording default workspace membership server: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, svc
}

// fakeOpenShellAPI implements gateway.OpenShellAPI, recording ReconcileMemberships
// calls and failing the test if any other method is invoked: the
// DefaultWorkspaceMembership reconciler must call ONLY ReconcileMemberships
// against the "default" workspace (CTRL-CORE-16: never EnsureWorkspace or any
// other operation).
type fakeOpenShellAPI struct {
	t                     *testing.T
	reconcileErr          error
	reconciledWorkspace   string
	reconciledMemberships []gateway.MembershipSpec
	reconcileCalls        int
}

func (f *fakeOpenShellAPI) EnsureWorkspace(context.Context, string, string) error {
	f.t.Fatal("EnsureWorkspace must never be called for the default workspace (CTRL-CORE-16)")
	return nil
}

func (f *fakeOpenShellAPI) ReconcileMemberships(_ context.Context, workspaceName string, memberships []gateway.MembershipSpec) error {
	f.reconcileCalls++
	f.reconciledWorkspace = workspaceName
	f.reconciledMemberships = memberships
	return f.reconcileErr
}

func (f *fakeOpenShellAPI) EnsureProvider(context.Context, string, gateway.ProviderRealization) error {
	f.t.Fatal("EnsureProvider must never be called by the DefaultWorkspaceMembership reconciler")
	return nil
}

func (f *fakeOpenShellAPI) ConfigureProviderRefresh(context.Context, string, gateway.RefreshRealization) error {
	f.t.Fatal("ConfigureProviderRefresh must never be called by the DefaultWorkspaceMembership reconciler")
	return nil
}

func (f *fakeOpenShellAPI) SetInferenceRoute(context.Context, string, string, string) error {
	f.t.Fatal("SetInferenceRoute must never be called by the DefaultWorkspaceMembership reconciler")
	return nil
}

func (f *fakeOpenShellAPI) DeleteWorkspace(context.Context, string) error {
	f.t.Fatal("DeleteWorkspace must never be called by the DefaultWorkspaceMembership reconciler")
	return nil
}

var _ gateway.OpenShellAPI = &fakeOpenShellAPI{}

func membership(id, gatewayID, subject, status string) *pb.DefaultWorkspaceMembership {
	return &pb.DefaultWorkspaceMembership{
		Metadata:             &pb.ObjectReference{Id: id},
		GatewayId:            gatewayID,
		Subject:              subject,
		Role:                 "user",
		Source:               "gateway_access",
		ReconciliationStatus: status,
	}
}

// CTRL-CORE-16 "nil session factory" posture (not yet active).
// Verifies: CTRL-CORE-16
func TestDefaultWorkspaceMembershipHandle_NilSessionIsNoop(t *testing.T) {
	conn, _ := newRecordingDefaultWorkspaceMembershipConn(t)
	r := NewDefaultWorkspaceMembershipReconciler(conn, "cluster-1", nil)
	err := r.Handle(context.Background(), watcher.Event[*pb.DefaultWorkspaceMembership]{ResourceID: "gw-1"})
	if err != nil {
		t.Fatalf("a nil session factory must be a no-op, got %v", err)
	}
}

// Full set reconciled, no EnsureWorkspace.
// Verifies: CTRL-CORE-16.01
func TestDefaultWorkspaceMembershipHandle_ReconcilesFullSetAgainstDefaultWorkspace(t *testing.T) {
	conn, svc := newRecordingDefaultWorkspaceMembershipConn(t)
	svc.setItems(
		membership("m-1", "gw-1", "sub-a", "pending"),
		membership("m-2", "gw-1", "sub-b", "pending"),
	)
	api := &fakeOpenShellAPI{t: t}
	session := func(context.Context, string) (gateway.OpenShellAPI, func() error, error) {
		return api, func() error { return nil }, nil
	}
	r := NewDefaultWorkspaceMembershipReconciler(conn, "cluster-1", session)

	err := r.Handle(context.Background(), watcher.Event[*pb.DefaultWorkspaceMembership]{ResourceID: "gw-1"})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if api.reconcileCalls != 1 {
		t.Fatalf("ReconcileMemberships calls = %d, want 1", api.reconcileCalls)
	}
	if api.reconciledWorkspace != "default" {
		t.Fatalf("reconciled workspace = %q, want %q", api.reconciledWorkspace, "default")
	}
	if len(api.reconciledMemberships) != 2 {
		t.Fatalf("reconciled memberships = %#v, want 2", api.reconciledMemberships)
	}
	for _, m := range api.reconciledMemberships {
		if m.Admin {
			t.Fatalf("membership %+v must never be admin (PLAT-DATA-27.03)", m)
		}
	}

	updates := svc.snapshotStatusUpdates()
	if len(updates) != 2 {
		t.Fatalf("status updates = %d, want 2", len(updates))
	}
	for _, u := range updates {
		if u.ReconciliationStatus != "ready" {
			t.Fatalf("status update = %q, want %q", u.ReconciliationStatus, "ready")
		}
		if u.GatewayId != "gw-1" {
			t.Fatalf("status update gateway_id = %q, want %q", u.GatewayId, "gw-1")
		}
	}
}

// CTRL-CORE-16 status transitions to failed with a stable,
// credential-free reason on an OpenShell error.
// Verifies: CTRL-CORE-16
func TestDefaultWorkspaceMembershipHandle_FailureMarksRowsFailed(t *testing.T) {
	conn, svc := newRecordingDefaultWorkspaceMembershipConn(t)
	svc.setItems(membership("m-1", "gw-1", "sub-a", "pending"))
	api := &fakeOpenShellAPI{t: t, reconcileErr: errors.New("openshell: invalid admin credentials sk-live-abc123")}
	session := func(context.Context, string) (gateway.OpenShellAPI, func() error, error) {
		return api, func() error { return nil }, nil
	}
	r := NewDefaultWorkspaceMembershipReconciler(conn, "cluster-1", session)

	err := r.Handle(context.Background(), watcher.Event[*pb.DefaultWorkspaceMembership]{ResourceID: "gw-1"})
	if err == nil {
		t.Fatal("expected Handle to return the OpenShell error")
	}

	updates := svc.snapshotStatusUpdates()
	if len(updates) != 1 {
		t.Fatalf("status updates = %d, want 1", len(updates))
	}
	if updates[0].ReconciliationStatus != "failed" {
		t.Fatalf("status = %q, want %q", updates[0].ReconciliationStatus, "failed")
	}
	if updates[0].LastReconcileError == nil {
		t.Fatal("expected last_reconcile_error to be set")
	}
	// PLAT-DATA-27: last_reconcile_error must be a stable, credential-free
	// summary, never the raw OpenShell error body.
	if *updates[0].LastReconcileError == api.reconcileErr.Error() {
		t.Fatalf("last_reconcile_error must not be the raw error, got %q", *updates[0].LastReconcileError)
	}
}

// A gateway with no live memberships is reconciled as a no-op (empty desired
// set), not skipped: OpenShell still converges to zero members if any were
// added outside HyperShell.
// Verifies: CTRL-CORE-16
func TestDefaultWorkspaceMembershipHandle_EmptySetStillReconciles(t *testing.T) {
	conn, _ := newRecordingDefaultWorkspaceMembershipConn(t)
	api := &fakeOpenShellAPI{t: t}
	session := func(context.Context, string) (gateway.OpenShellAPI, func() error, error) {
		return api, func() error { return nil }, nil
	}
	r := NewDefaultWorkspaceMembershipReconciler(conn, "cluster-1", session)

	err := r.Handle(context.Background(), watcher.Event[*pb.DefaultWorkspaceMembership]{ResourceID: "gw-1"})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if api.reconcileCalls != 1 {
		t.Fatalf("ReconcileMemberships calls = %d, want 1 (even for an empty desired set)", api.reconcileCalls)
	}
	if len(api.reconciledMemberships) != 0 {
		t.Fatalf("reconciled memberships = %#v, want empty", api.reconciledMemberships)
	}
}
