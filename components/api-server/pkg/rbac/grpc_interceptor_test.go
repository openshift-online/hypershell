package rbac

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/auth"
)

// fakeLookup and fakeProvisioner stand in for the DB-backed role-binding lookup
// and JWT user provisioner so the interceptor can be exercised without a live
// database or auth stack.
type fakeLookup struct {
	bindings []BindingSummary
}

func (f fakeLookup) FindBindingsByUserID(_ context.Context, _ string) ([]BindingSummary, error) {
	return f.bindings, nil
}

type fakeProvisioner struct{ userID string }

func (f fakeProvisioner) UpsertFromJWT(_ context.Context, _ *auth.Payload) (string, error) {
	return f.userID, nil
}

var (
	creatorBinding = BindingSummary{RoleName: "gateway:creator", Scope: "global"}
	adminBinding   = BindingSummary{RoleName: "platform:admin", Scope: "global"}
)

func ownerOf(gatewayID string) BindingSummary {
	return BindingSummary{RoleName: "gateway:owner", Scope: "gateway", GatewayID: strPtr(gatewayID)}
}

func viewerOf(gatewayID string) BindingSummary {
	return BindingSummary{RoleName: "gateway:viewer", Scope: "gateway", GatewayID: strPtr(gatewayID)}
}

func gatewaySvc(method string) string { return "/hypershell.v1.GatewayService/" + method }

// TestAuthorizeUserGRPC pins the HTTP rules on gRPC (rbac-enforcement.spec.md,
// "gRPC Authorization"): a global gateway:creator binding no longer authorizes
// every method, per-gateway methods need a binding on that gateway, and the
// unfiltered list/watch and the control-plane-only writes are not user methods.
func TestAuthorizeUserGRPC(t *testing.T) {
	get := &pb.GetGatewayRequest{Id: "gw-1"}
	update := &pb.UpdateGatewayRequest{Id: "gw-1"}
	del := &pb.DeleteGatewayRequest{Id: "gw-1"}

	tests := []struct {
		name     string
		method   string
		req      interface{}
		bindings []BindingSummary
		want     codes.Code
	}{
		{"creator creates", gatewaySvc("CreateGateway"), &pb.CreateGatewayRequest{}, []BindingSummary{creatorBinding}, codes.OK},
		{"admin alone cannot create", gatewaySvc("CreateGateway"), &pb.CreateGatewayRequest{}, []BindingSummary{adminBinding}, codes.PermissionDenied},
		{"creator cannot read another's gateway", gatewaySvc("GetGateway"), get, []BindingSummary{creatorBinding}, codes.NotFound},
		{"creator cannot update another's gateway", gatewaySvc("UpdateGateway"), update, []BindingSummary{creatorBinding}, codes.PermissionDenied},
		{"creator cannot delete another's gateway", gatewaySvc("DeleteGateway"), del, []BindingSummary{creatorBinding}, codes.PermissionDenied},
		{"owner reads", gatewaySvc("GetGateway"), get, []BindingSummary{ownerOf("gw-1")}, codes.OK},
		{"owner updates", gatewaySvc("UpdateGateway"), update, []BindingSummary{ownerOf("gw-1")}, codes.OK},
		{"owner deletes", gatewaySvc("DeleteGateway"), del, []BindingSummary{ownerOf("gw-1")}, codes.OK},
		{"owner of another gateway is denied", gatewaySvc("UpdateGateway"), update, []BindingSummary{ownerOf("gw-2")}, codes.PermissionDenied},
		{"viewer reads", gatewaySvc("GetGateway"), get, []BindingSummary{viewerOf("gw-1")}, codes.OK},
		{"viewer cannot update", gatewaySvc("UpdateGateway"), update, []BindingSummary{viewerOf("gw-1")}, codes.PermissionDenied},
		{"admin reads any", gatewaySvc("GetGateway"), get, []BindingSummary{adminBinding}, codes.OK},
		{"admin deletes any", gatewaySvc("DeleteGateway"), del, []BindingSummary{adminBinding}, codes.OK},
		{"admin cannot update without ownership", gatewaySvc("UpdateGateway"), update, []BindingSummary{adminBinding}, codes.PermissionDenied},
		{"admin lists", gatewaySvc("ListGateways"), &pb.ListGatewaysRequest{}, []BindingSummary{adminBinding}, codes.OK},
		{"admin watches", gatewaySvc("WatchGateways"), nil, []BindingSummary{adminBinding}, codes.OK},
		{"creator cannot list every gateway", gatewaySvc("ListGateways"), &pb.ListGatewaysRequest{}, []BindingSummary{creatorBinding, ownerOf("gw-1")}, codes.PermissionDenied},
		{"creator cannot watch every gateway", gatewaySvc("WatchGateways"), nil, []BindingSummary{creatorBinding}, codes.PermissionDenied},
		{"owner cannot adjust sandbox counts", gatewaySvc("AdjustActiveSandboxCount"), &pb.AdjustActiveSandboxCountRequest{}, []BindingSummary{ownerOf("gw-1")}, codes.PermissionDenied},
		{"owner cannot set sandbox counts", gatewaySvc("SetActiveSandboxCount"), &pb.SetActiveSandboxCountRequest{}, []BindingSummary{ownerOf("gw-1")}, codes.PermissionDenied},
		{"owner cannot set the gateway version", gatewaySvc("SetGatewayVersion"), &pb.SetGatewayVersionRequest{Id: "gw-1"}, []BindingSummary{ownerOf("gw-1")}, codes.PermissionDenied},
		{"creator reads clusters", "/hypershell.v1.ManagedClusterService/ListManagedClusters", &pb.ListManagedClustersRequest{}, []BindingSummary{creatorBinding}, codes.OK},
		{"creator cannot delete a cluster", "/hypershell.v1.ManagedClusterService/DeleteManagedCluster", &pb.DeleteManagedClusterRequest{Id: "c"}, []BindingSummary{creatorBinding}, codes.PermissionDenied},
		{"creator cannot create a cluster", "/hypershell.v1.ManagedClusterService/CreateManagedCluster", &pb.CreateManagedClusterRequest{}, []BindingSummary{creatorBinding}, codes.PermissionDenied},
		{"admin deletes a cluster", "/hypershell.v1.ManagedClusterService/DeleteManagedCluster", &pb.DeleteManagedClusterRequest{Id: "c"}, []BindingSummary{adminBinding}, codes.OK},
		{"no RoleBinding service for users", "/hypershell.v1.RoleBindingService/ListRoleBindings", &pb.ListRoleBindingsRequest{}, []BindingSummary{creatorBinding, adminBinding}, codes.PermissionDenied},
		{"no RoleBinding watch for users", "/hypershell.v1.RoleBindingService/WatchRoleBindings", nil, []BindingSummary{adminBinding}, codes.PermissionDenied},
		{"unknown service denied", "/hypershell.v1.SomethingNew/DoIt", nil, []BindingSummary{creatorBinding, adminBinding}, codes.PermissionDenied},
		{"health with a binding", "/grpc.health.v1.Health/Check", nil, []BindingSummary{creatorBinding}, codes.OK},
		{"no bindings denied", gatewaySvc("GetGateway"), get, nil, codes.NotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := status.Code(authorizeUserGRPC(tt.method, tt.req, tt.bindings)); got != tt.want {
				t.Fatalf("authorizeUserGRPC(%s) = %s, want %s", tt.method, got, tt.want)
			}
		})
	}
}

func TestIsServiceAccount_MatchesConfiguredAccount(t *testing.T) {
	accounts := []string{"service-account-hypershell-control-plane"}
	if !isServiceAccount("service-account-hypershell-control-plane", accounts) {
		t.Error("configured service account should match")
	}
}

func TestIsServiceAccount_RejectsUnknownUsername(t *testing.T) {
	accounts := []string{"service-account-hypershell-control-plane"}
	if isServiceAccount("some-human-user", accounts) {
		t.Error("unknown username must not match service accounts")
	}
}

func TestIsServiceAccount_EmptyUsernameNeverMatches(t *testing.T) {
	accounts := []string{"service-account-hypershell-control-plane"}
	if isServiceAccount("", accounts) {
		t.Error("empty username must not match")
	}
}

func TestIsServiceAccount_EmptyListNeverMatches(t *testing.T) {
	if isServiceAccount("service-account-hypershell-control-plane", nil) {
		t.Error("nil service accounts list must not match")
	}
}

// The bootstrap allowlist keeps the control-plane-only writes, which ordinary
// role bindings never reach, but not ManagedCluster record writes.
func TestUnaryInterceptor_AllowlistScope(t *testing.T) {
	const sa = "service-account-hypershell-control-plane"
	owner := fakeLookup{bindings: []BindingSummary{ownerOf("gw-1"), creatorBinding}}

	tests := []struct {
		name     string
		username string
		method   string
		req      interface{}
		want     codes.Code
	}{
		{"allowlisted adjusts sandbox counts", sa, gatewaySvc("AdjustActiveSandboxCount"), &pb.AdjustActiveSandboxCountRequest{Namespace: "ns"}, codes.OK},
		{"allowlisted sets the gateway version", sa, gatewaySvc("SetGatewayVersion"), &pb.SetGatewayVersionRequest{Id: "gw-1"}, codes.OK},
		{"allowlisted cannot delete a cluster record", sa, "/hypershell.v1.ManagedClusterService/DeleteManagedCluster", &pb.DeleteManagedClusterRequest{Id: "c"}, codes.PermissionDenied},
		{"allowlisted cannot rename a cluster record", sa, "/hypershell.v1.ManagedClusterService/UpdateManagedCluster", &pb.UpdateManagedClusterRequest{Id: "c"}, codes.PermissionDenied},
		{"owner cannot adjust sandbox counts", "human-owner", gatewaySvc("AdjustActiveSandboxCount"), &pb.AdjustActiveSandboxCountRequest{Namespace: "ns"}, codes.PermissionDenied},
		{"owner cannot set the gateway version", "human-owner", gatewaySvc("SetGatewayVersion"), &pb.SetGatewayVersionRequest{Id: "gw-1"}, codes.PermissionDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interceptor := RBACUnaryInterceptor(owner, fakeProvisioner{userID: "user-1"}, nil, nil, nil, nil, AuthzConfig{EnforceRBAC: true, ServiceAccounts: []string{sa}})
			called := false
			_, err := interceptor(auth.SetUsernameContext(context.Background(), tt.username), tt.req, &grpc.UnaryServerInfo{FullMethod: tt.method},
				func(context.Context, interface{}) (interface{}, error) {
					called = true
					return nil, nil
				})
			if got := status.Code(err); got != tt.want {
				t.Fatalf("code = %s, want %s", got, tt.want)
			}
			if called != (tt.want == codes.OK) {
				t.Fatalf("handler called = %v", called)
			}
		})
	}
}

// Under enforcement an unauthenticated call reaches no hypershell.v1 method.
func TestUnaryInterceptor_UnauthenticatedDenied(t *testing.T) {
	interceptor := RBACUnaryInterceptor(fakeLookup{}, fakeProvisioner{}, nil, nil, nil, nil, AuthzConfig{EnforceRBAC: true})
	handler := func(context.Context, interface{}) (interface{}, error) { return nil, nil }
	_, err := interceptor(context.Background(), &pb.ListGatewaysRequest{}, &grpc.UnaryServerInfo{FullMethod: gatewaySvc("ListGateways")}, handler)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unauthenticated ListGateways: %v, want PermissionDenied", err)
	}
	if _, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}, handler); err != nil {
		t.Fatalf("unauthenticated health check: %v", err)
	}
}
