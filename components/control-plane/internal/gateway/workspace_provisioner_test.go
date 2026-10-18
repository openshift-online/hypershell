package gateway

import (
	"context"
	"errors"
	"net"
	"testing"

	gw "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/gateway/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// fakeGatewayServer records the provisioning calls it receives and can be told to
// return a specific error for each RPC, so tests assert the control-plane client's
// call sequence, idempotency, and error classification.
type fakeGatewayServer struct {
	gw.UnimplementedAgentWorkspaceProvisioningServer

	ensureWorkspace    []*gw.EnsureWorkspaceRequest
	reconcileMembers   []*gw.ReconcileMembershipsRequest
	ensureProvider     []*gw.EnsureProviderRequest
	ensureRoute        []*gw.EnsureInferenceRouteRequest
	deleteWorkspace    []*gw.DeleteWorkspaceRequest
	ensureWorkspaceErr error
	ensureProviderErr  error
	deleteWorkspaceErr error
}

func (s *fakeGatewayServer) EnsureWorkspace(_ context.Context, req *gw.EnsureWorkspaceRequest) (*gw.Workspace, error) {
	s.ensureWorkspace = append(s.ensureWorkspace, req)
	if s.ensureWorkspaceErr != nil {
		return nil, s.ensureWorkspaceErr
	}
	return &gw.Workspace{AgentRuntimeId: req.GetAgentRuntimeId(), WorkspaceName: req.GetWorkspaceName()}, nil
}

func (s *fakeGatewayServer) ReconcileMemberships(_ context.Context, req *gw.ReconcileMembershipsRequest) (*gw.MembershipSet, error) {
	s.reconcileMembers = append(s.reconcileMembers, req)
	return &gw.MembershipSet{Memberships: req.GetMemberships()}, nil
}

func (s *fakeGatewayServer) EnsureProvider(_ context.Context, req *gw.EnsureProviderRequest) (*gw.Provider, error) {
	s.ensureProvider = append(s.ensureProvider, req)
	if s.ensureProviderErr != nil {
		return nil, s.ensureProviderErr
	}
	return &gw.Provider{ProviderName: req.GetProviderName()}, nil
}

func (s *fakeGatewayServer) EnsureInferenceRoute(_ context.Context, req *gw.EnsureInferenceRouteRequest) (*gw.InferenceRoute, error) {
	s.ensureRoute = append(s.ensureRoute, req)
	return &gw.InferenceRoute{ProviderName: req.GetProviderName(), ModelAlias: req.GetModelAlias()}, nil
}

func (s *fakeGatewayServer) DeleteWorkspace(_ context.Context, req *gw.DeleteWorkspaceRequest) (*gw.DeleteWorkspaceResponse, error) {
	s.deleteWorkspace = append(s.deleteWorkspace, req)
	if s.deleteWorkspaceErr != nil {
		return nil, s.deleteWorkspaceErr
	}
	return &gw.DeleteWorkspaceResponse{}, nil
}

// newFakeProvisioner wires a WorkspaceProvisioner to an in-process fake gateway server
// over bufconn. The dialer re-dials per operation, mirroring production.
func newFakeProvisioner(t *testing.T, server *fakeGatewayServer) *WorkspaceProvisioner {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	gw.RegisterAgentWorkspaceProvisioningServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)

	dial := func(ctx context.Context, _ string) (gw.AgentWorkspaceProvisioningClient, func() error, error) {
		conn, err := grpc.NewClient(
			"passthrough:///bufnet",
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return nil, nil, err
		}
		return gw.NewAgentWorkspaceProvisioningClient(conn), conn.Close, nil
	}
	p, err := NewWorkspaceProvisioner(dial)
	if err != nil {
		t.Fatalf("NewWorkspaceProvisioner: %v", err)
	}
	return p
}

func sampleSpec() WorkspaceSpec {
	return WorkspaceSpec{
		GatewayID:      "gw-1",
		AgentRuntimeID: "ar-1",
		WorkspaceName:  "amber-reviews",
		Memberships:    []MembershipSpec{{Subject: "subject-1", Admin: true}},
		Providers: []ProviderConfig{{
			Name:                  "github",
			RefreshType:           "app-installation-token",
			RefreshSecretMaterial: map[string][]byte{"app-private-key": []byte("PEM")},
		}},
		Route: &InferenceRouteConfig{ProviderName: "vertex", ModelAlias: "claude-sonnet-4-5"},
	}
}

func TestWorkspaceProvisioner_RealizeCallsFullSequence(t *testing.T) {
	server := &fakeGatewayServer{}
	p := newFakeProvisioner(t, server)

	if err := p.Realize(context.Background(), sampleSpec()); err != nil {
		t.Fatalf("Realize: %v", err)
	}
	if len(server.ensureWorkspace) != 1 {
		t.Fatalf("EnsureWorkspace calls = %d, want 1", len(server.ensureWorkspace))
	}
	if len(server.reconcileMembers) != 1 || len(server.reconcileMembers[0].GetMemberships()) != 1 {
		t.Fatalf("ReconcileMemberships not called with the full membership set: %+v", server.reconcileMembers)
	}
	if server.reconcileMembers[0].GetMemberships()[0].GetRole() != gw.Role_ROLE_ADMIN {
		t.Fatalf("admin membership not propagated")
	}
	if len(server.ensureProvider) != 1 || server.ensureProvider[0].GetRefreshStrategy().GetType() != "app-installation-token" {
		t.Fatalf("EnsureProvider not called with the refresh strategy: %+v", server.ensureProvider)
	}
	if len(server.ensureRoute) != 1 {
		t.Fatalf("EnsureInferenceRoute calls = %d, want 1", len(server.ensureRoute))
	}
}

func TestWorkspaceProvisioner_RealizeIsIdempotent(t *testing.T) {
	server := &fakeGatewayServer{}
	p := newFakeProvisioner(t, server)

	for i := 0; i < 2; i++ {
		if err := p.Realize(context.Background(), sampleSpec()); err != nil {
			t.Fatalf("Realize pass %d: %v", i, err)
		}
	}
	// The fake accepts a repeat ensure as a no-op success, so two passes make two
	// successful calls without error - the idempotent contract from the client's view.
	if len(server.ensureWorkspace) != 2 {
		t.Fatalf("EnsureWorkspace calls = %d, want 2 (idempotent re-ensure)", len(server.ensureWorkspace))
	}
}

func TestWorkspaceProvisioner_NameCollisionIsPermanent(t *testing.T) {
	server := &fakeGatewayServer{ensureWorkspaceErr: status.Error(codes.AlreadyExists, "name taken")}
	p := newFakeProvisioner(t, server)

	err := p.Realize(context.Background(), sampleSpec())
	if !errors.Is(err, ErrWorkspaceNameCollision) {
		t.Fatalf("error = %v, want ErrWorkspaceNameCollision", err)
	}
	if len(server.ensureProvider) != 0 {
		t.Fatalf("provider must not be ensured after a workspace collision")
	}
}

func TestWorkspaceProvisioner_AdminRequiredIsPermanent(t *testing.T) {
	server := &fakeGatewayServer{ensureProviderErr: status.Error(codes.FailedPrecondition, "admin required")}
	p := newFakeProvisioner(t, server)

	err := p.Realize(context.Background(), sampleSpec())
	if !errors.Is(err, ErrAdminMembershipRequired) {
		t.Fatalf("error = %v, want ErrAdminMembershipRequired", err)
	}
}

func TestWorkspaceProvisioner_TransientErrorIsRetryable(t *testing.T) {
	server := &fakeGatewayServer{ensureWorkspaceErr: status.Error(codes.Unavailable, "gateway down")}
	p := newFakeProvisioner(t, server)

	err := p.Realize(context.Background(), sampleSpec())
	if err == nil {
		t.Fatalf("expected a retryable error")
	}
	if errors.Is(err, ErrWorkspaceNameCollision) || errors.Is(err, ErrAdminMembershipRequired) {
		t.Fatalf("transient error must not be classified as a permanent rejection: %v", err)
	}
}

func TestWorkspaceProvisioner_UnexpectedCodeIsRetryable(t *testing.T) {
	// FAILED_PRECONDITION from EnsureWorkspace (not EnsureProvider) is a convergeable
	// condition, not the permanent admin-required rejection; it must be retryable.
	server := &fakeGatewayServer{ensureWorkspaceErr: status.Error(codes.FailedPrecondition, "workspace not ready")}
	p := newFakeProvisioner(t, server)
	err := p.Realize(context.Background(), sampleSpec())
	if err == nil {
		t.Fatalf("expected an error")
	}
	if errors.Is(err, ErrAdminMembershipRequired) || errors.Is(err, ErrWorkspaceNameCollision) {
		t.Fatalf("FAILED_PRECONDITION from ensure workspace must be retryable, got permanent: %v", err)
	}

	// ALREADY_EXISTS from EnsureProvider (not EnsureWorkspace) is likewise convergeable.
	server2 := &fakeGatewayServer{ensureProviderErr: status.Error(codes.AlreadyExists, "provider race")}
	p2 := newFakeProvisioner(t, server2)
	err2 := p2.Realize(context.Background(), sampleSpec())
	if err2 == nil {
		t.Fatalf("expected an error")
	}
	if errors.Is(err2, ErrWorkspaceNameCollision) || errors.Is(err2, ErrAdminMembershipRequired) {
		t.Fatalf("ALREADY_EXISTS from ensure provider must be retryable, got permanent: %v", err2)
	}
}

func TestWorkspaceProvisioner_DeleteCascades(t *testing.T) {
	server := &fakeGatewayServer{}
	p := newFakeProvisioner(t, server)

	if err := p.Delete(context.Background(), "gw-1", "ar-1", "amber-reviews"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(server.deleteWorkspace) != 1 {
		t.Fatalf("DeleteWorkspace calls = %d, want 1", len(server.deleteWorkspace))
	}
}

func TestWorkspaceProvisioner_DeleteAbsentIsNoOp(t *testing.T) {
	server := &fakeGatewayServer{deleteWorkspaceErr: status.Error(codes.NotFound, "already gone")}
	p := newFakeProvisioner(t, server)

	if err := p.Delete(context.Background(), "gw-1", "ar-1", "amber-reviews"); err != nil {
		t.Fatalf("Delete of an absent workspace must be a no-op success, got: %v", err)
	}
}

func TestNewWorkspaceProvisioner_RequiresDialer(t *testing.T) {
	if _, err := NewWorkspaceProvisioner(nil); err == nil {
		t.Fatalf("expected an error for a nil dialer")
	}
}
