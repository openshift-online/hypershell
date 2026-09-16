package reconciler

import (
	"context"
	"strings"
	"testing"

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
	r := NewRoleBindingReconciler(keycloak.NewClient("http://keycloak", "hypershell", "id", "secret"), nil)
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
	r := NewRoleBindingReconciler(keycloak.NewClient("http://keycloak", "hypershell", "id", "secret"), nil)
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
