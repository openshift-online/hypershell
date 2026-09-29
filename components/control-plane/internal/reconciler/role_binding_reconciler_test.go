package reconciler

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
)

func roleBinding(id, roleName string) *pb.RoleBinding {
	return &pb.RoleBinding{
		Metadata: &pb.ObjectReference{Id: id},
		RoleName: roleName,
	}
}

// Deleting an owner binding while a viewer binding survives must NOT revoke
// openshell-user: both roles grant it, so it is still desired. Only
// openshell-admin (owner-exclusive) should end up revocable.
func TestUnionKcRoles_OverlapKeepsSharedRole(t *testing.T) {
	const deletedID = "rb-owner"
	remaining := []*pb.RoleBinding{
		roleBinding(deletedID, "gateway:owner"), // the one being deleted -- excluded
		roleBinding("rb-viewer", "gateway:viewer"),
	}

	stillDesired := unionKcRoles(remaining, deletedID)

	if !stillDesired["openshell-user"] {
		t.Error("openshell-user must remain desired: the viewer binding still grants it")
	}
	if stillDesired["openshell-admin"] {
		t.Error("openshell-admin must not be desired: only the deleted owner binding granted it")
	}

	// Confirm the revoke decision for the deleted owner binding: keep
	// openshell-user, revoke openshell-admin.
	var revoked []string
	for _, kcRole := range keycloakRoleMap["gateway:owner"] {
		if !stillDesired[kcRole] {
			revoked = append(revoked, kcRole)
		}
	}
	if len(revoked) != 1 || revoked[0] != "openshell-admin" {
		t.Errorf("revoked = %v, want [openshell-admin]", revoked)
	}
}

// With no surviving bindings, every role the deleted binding granted becomes
// revocable.
func TestUnionKcRoles_NoRemainingBindingsRevokesAll(t *testing.T) {
	const deletedID = "rb-owner"
	stillDesired := unionKcRoles([]*pb.RoleBinding{roleBinding(deletedID, "gateway:owner")}, deletedID)
	if len(stillDesired) != 0 {
		t.Errorf("stillDesired = %v, want empty", stillDesired)
	}
}

// A gateway owner must receive BOTH openshell-admin and openshell-user on the
// per-gateway console client. The gateway admin API enforces the two roles
// independently (admin does not imply user), so an owner missing openshell-user
// can read gateway info but is refused "list workspaces" with
// "role 'openshell-user' required". A viewer receives only openshell-user.
func TestKeycloakRoleMap_OwnerGetsAdminAndUser(t *testing.T) {
	cases := []struct {
		roleBinding string
		want        []string
	}{
		{"gateway:owner", []string{"openshell-admin", "openshell-user"}},
		{"gateway:viewer", []string{"openshell-user"}},
	}
	for _, tc := range cases {
		got, ok := keycloakRoleMap[tc.roleBinding]
		if !ok {
			t.Fatalf("keycloakRoleMap missing mapping for %q", tc.roleBinding)
		}
		if len(got) != len(tc.want) {
			t.Fatalf("keycloakRoleMap[%q] = %v, want %v", tc.roleBinding, got, tc.want)
		}
		for i, role := range tc.want {
			if got[i] != role {
				t.Errorf("keycloakRoleMap[%q][%d] = %q, want %q", tc.roleBinding, i, got[i], role)
			}
		}
	}
}

func TestHandle_EmptyUsernameIsError(t *testing.T) {
	r := NewRoleBindingReconciler(keycloak.NewClient("http://keycloak", "hypershell", "id", "secret"), nil, "cluster-1")
	gatewayID := "gw-1"
	err := r.Handle(context.Background(), watcher.Event[*pb.RoleBinding]{
		ResourceID: "rb-1",
		Type:       watcher.EventCreated,
		Resource: &pb.RoleBinding{
			RoleName:  "gateway:owner",
			GatewayId: &gatewayID,
			Username:  "",
		},
	})
	if err == nil {
		t.Fatal("empty username must be an error so the queue retries")
	}
	if !strings.Contains(err.Error(), "username not yet resolved") {
		t.Fatalf("error = %v, want username not yet resolved", err)
	}
}

func TestHandle_EmptyRoleNameIsError(t *testing.T) {
	r := NewRoleBindingReconciler(keycloak.NewClient("http://keycloak", "hypershell", "id", "secret"), nil, "cluster-1")
	gatewayID := "gw-1"
	err := r.Handle(context.Background(), watcher.Event[*pb.RoleBinding]{
		ResourceID: "rb-1",
		Type:       watcher.EventCreated,
		Resource: &pb.RoleBinding{
			RoleName:  "",
			GatewayId: &gatewayID,
			Username:  "service-account-hypershell-e2e",
		},
	})
	if err == nil {
		t.Fatal("empty role name must be an error so the queue retries")
	}
	if !strings.Contains(err.Error(), "role name not yet resolved") {
		t.Fatalf("error = %v, want role name not yet resolved", err)
	}
}

// A binding for a gateway hosted by another cluster is not this control
// plane's to sync. The api-server's cluster-scoped watch should never deliver
// one; should one arrive anyway (defense in depth), the event is dropped
// without touching Keycloak and without an error, so the queue does not retry
// it.
func TestHandle_SkipsGatewayOfAnotherCluster(t *testing.T) {
	conn, recorder := newRecordingGatewayConn(t)
	recorder.setGateway(&pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: "gw-1"},
		Name:      "team-gateway",
		ClusterId: "cluster-2",
	})
	// An unroutable Keycloak: any attempt to assign a role fails loudly.
	r := NewRoleBindingReconciler(keycloak.NewClient("http://127.0.0.1:1", "hypershell", "id", "secret"), conn, "cluster-1")
	gatewayID := "gw-1"
	err := r.Handle(context.Background(), watcher.Event[*pb.RoleBinding]{
		ResourceID: "rb-1",
		Type:       watcher.EventCreated,
		Resource: &pb.RoleBinding{
			RoleName:  "gateway:owner",
			GatewayId: &gatewayID,
			Username:  "alice",
		},
	})
	if err != nil {
		t.Fatalf("binding for another cluster's gateway must be skipped without error, got %v", err)
	}
}

// The same binding on the control plane that owns the gateway proceeds to the
// Keycloak sync (which here fails because Keycloak is unroutable), proving the
// cluster check is what gates the skip above.
func TestHandle_OwnClusterGatewayReachesKeycloak(t *testing.T) {
	conn, recorder := newRecordingGatewayConn(t)
	recorder.setGateway(&pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: "gw-1"},
		Name:      "team-gateway",
		ClusterId: "cluster-1",
	})
	r := NewRoleBindingReconciler(keycloak.NewClient("http://127.0.0.1:1", "hypershell", "id", "secret"), conn, "cluster-1")
	gatewayID := "gw-1"
	err := r.Handle(context.Background(), watcher.Event[*pb.RoleBinding]{
		ResourceID: "rb-1",
		Type:       watcher.EventCreated,
		Resource: &pb.RoleBinding{
			RoleName:  "gateway:owner",
			GatewayId: &gatewayID,
			Username:  "alice",
		},
	})
	if err == nil {
		t.Fatal("binding for this cluster's gateway must reach Keycloak and surface its error")
	}
	if !strings.Contains(err.Error(), "assign keycloak role") {
		t.Fatalf("error = %v, want a keycloak assignment error", err)
	}
}

// recordingRoleBindingServer records each ListRoleBindings request so tests can
// assert the delete path scopes its recompute to this control plane's cluster.
type recordingRoleBindingServer struct {
	pb.UnimplementedRoleBindingServiceServer

	mu    sync.Mutex
	lists []*pb.ListRoleBindingsRequest
}

func (s *recordingRoleBindingServer) ListRoleBindings(_ context.Context, req *pb.ListRoleBindingsRequest) (*pb.ListRoleBindingsResponse, error) {
	s.mu.Lock()
	s.lists = append(s.lists, req)
	s.mu.Unlock()
	return &pb.ListRoleBindingsResponse{}, nil
}

func (s *recordingRoleBindingServer) snapshot() []*pb.ListRoleBindingsRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*pb.ListRoleBindingsRequest(nil), s.lists...)
}

// newRecordingGatewayAndRoleBindingConn serves both a recording gateway server
// and a recording role binding server on one in-memory connection.
func newRecordingGatewayAndRoleBindingConn(t *testing.T) (*grpc.ClientConn, *recordingGatewayServer, *recordingRoleBindingServer) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	gateways := &recordingGatewayServer{}
	bindings := &recordingRoleBindingServer{}
	pb.RegisterGatewayServiceServer(server, gateways)
	pb.RegisterRoleBindingServiceServer(server, bindings)
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
		t.Fatalf("dial recording servers: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, gateways, bindings
}

// On delete the reconciler recomputes the user's surviving roles with
// ListRoleBindings. The api-server requires a registered control plane to scope
// that list to its own cluster, so the request must carry this cluster's id.
func TestHandle_DeleteListsSurvivingBindingsScopedToCluster(t *testing.T) {
	conn, gateways, bindings := newRecordingGatewayAndRoleBindingConn(t)
	gateways.setGateway(&pb.Gateway{
		Metadata:  &pb.ObjectReference{Id: "gw-1"},
		Name:      "team-gateway",
		ClusterId: "cluster-1",
	})
	// An unroutable Keycloak: the revoke after the list fails loudly.
	r := NewRoleBindingReconciler(keycloak.NewClient("http://127.0.0.1:1", "hypershell", "id", "secret"), conn, "cluster-1")
	gatewayID := "gw-1"
	userID := "user-1"
	err := r.Handle(context.Background(), watcher.Event[*pb.RoleBinding]{
		ResourceID: "rb-1",
		Type:       watcher.EventDeleted,
		Resource: &pb.RoleBinding{
			Metadata:  &pb.ObjectReference{Id: "rb-1"},
			RoleName:  "gateway:owner",
			GatewayId: &gatewayID,
			UserId:    &userID,
			Username:  "alice",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "remove keycloak role") {
		t.Fatalf("error = %v, want a keycloak revoke error after the list", err)
	}

	lists := bindings.snapshot()
	if len(lists) != 1 {
		t.Fatalf("ListRoleBindings calls = %d, want 1", len(lists))
	}
	req := lists[0]
	if req.ClusterId == nil || *req.ClusterId != "cluster-1" {
		t.Fatalf("ListRoleBindings cluster_id = %v, want %q", req.ClusterId, "cluster-1")
	}
	if req.GetUserId() != userID || req.GetGatewayId() != gatewayID {
		t.Fatalf("ListRoleBindings user/gateway = %q/%q, want %q/%q", req.GetUserId(), req.GetGatewayId(), userID, gatewayID)
	}
}
