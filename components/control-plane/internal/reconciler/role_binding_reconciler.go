package reconciler

import (
	"context"
	"fmt"
	"log"
	"sync"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
)

// keycloakRoleMap maps a platform RoleBinding role to the gateway client roles a
// user must hold on the per-gateway Keycloak console client. The gateway admin
// API enforces openshell-admin and openshell-user independently (admin does not
// imply user), so an owner must be granted BOTH -- otherwise the console can read
// gateway info but is refused "list workspaces" with "role 'openshell-user' required".
var keycloakRoleMap = map[string][]string{
	"gateway:owner":  {"openshell-admin", "openshell-user"},
	"gateway:viewer": {"openshell-user"},
}

type RoleBindingReconciler struct {
	mu             sync.Mutex
	active         map[string]struct{}
	keycloakClient *keycloak.Client
	grpcConn       *grpc.ClientConn
}

func NewRoleBindingReconciler(keycloakClient *keycloak.Client, grpcConn *grpc.ClientConn) *RoleBindingReconciler {
	return &RoleBindingReconciler{
		active:         make(map[string]struct{}),
		keycloakClient: keycloakClient,
		grpcConn:       grpcConn,
	}
}

func (r *RoleBindingReconciler) Handle(ctx context.Context, event watcher.Event[*pb.RoleBinding]) error {
	r.mu.Lock()
	if _, ok := r.active[event.ResourceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.active[event.ResourceID] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.active, event.ResourceID)
		r.mu.Unlock()
	}()

	rb := event.Resource
	if rb == nil {
		log.Printf("WARN role binding event %s has nil resource, skipping", event.ResourceID)
		return nil
	}

	if r.keycloakClient == nil {
		log.Printf("DEBUG role binding %s: keycloak not configured, skipping", event.ResourceID)
		return nil
	}

	ctx, endSpan := cpotel.StartReconcileSpan(ctx, "RoleBinding", event.Type.String(), rb.GetMetadata().GetTraceparent())
	var reconcileErr error
	defer func() { endSpan(reconcileErr) }()

	if rb.GatewayId == nil || *rb.GatewayId == "" {
		log.Printf("DEBUG role binding %s: no gateway_id (global scope), skipping keycloak sync", event.ResourceID)
		return nil
	}

	kcRoles, ok := keycloakRoleMap[rb.RoleName]
	if !ok {
		log.Printf("DEBUG role binding %s: role %q has no keycloak mapping, skipping", event.ResourceID, rb.RoleName)
		return nil
	}

	username := rb.Username
	if username == "" {
		log.Printf("WARN role binding %s: no username available, skipping keycloak sync", event.ResourceID)
		return nil
	}

	kcClientID, err := r.resolveKeycloakClientID(ctx, *rb.GatewayId)
	if err != nil {
		reconcileErr = fmt.Errorf("resolve keycloak client id for role binding %s: %w", event.ResourceID, err)
		return reconcileErr
	}

	switch event.Type {
	case watcher.EventCreated, watcher.EventUpdated:
		for _, kcRole := range kcRoles {
			log.Printf("INFO assigning keycloak role %s to user %s on client %s", kcRole, username, kcClientID)
			if err := r.keycloakClient.AssignClientRole(ctx, kcClientID, username, kcRole); err != nil {
				reconcileErr = fmt.Errorf("assign keycloak role %s to user %s on client %s: %w", kcRole, username, kcClientID, err)
				return reconcileErr
			}
		}

	case watcher.EventDeleted:
		// A single Keycloak role can be granted by more than one platform
		// RoleBinding (both gateway:owner and gateway:viewer grant
		// openshell-user). Revoking blindly would strip a role the user still
		// holds via another surviving binding, so recompute the roles still
		// desired for this user+gateway and only revoke what is no longer
		// backed by any remaining binding.
		stillDesired, err := r.stillDesiredKcRoles(ctx, rb)
		if err != nil {
			reconcileErr = fmt.Errorf("recompute effective keycloak roles for role binding %s: %w", event.ResourceID, err)
			return reconcileErr
		}
		for _, kcRole := range kcRoles {
			if stillDesired[kcRole] {
				log.Printf("INFO keeping keycloak role %s for user %s on client %s: still granted by another role binding", kcRole, username, kcClientID)
				continue
			}
			log.Printf("INFO removing keycloak role %s from user %s on client %s", kcRole, username, kcClientID)
			if err := r.keycloakClient.RemoveClientRole(ctx, kcClientID, username, kcRole); err != nil {
				reconcileErr = fmt.Errorf("remove keycloak role %s from user %s on client %s: %w", kcRole, username, kcClientID, err)
				return reconcileErr
			}
		}
	}

	return nil
}

// stillDesiredKcRoles returns the set of Keycloak client roles the user should
// still hold on the deleted binding's gateway, computed as the union of the role
// mappings across every remaining (non-deleted) RoleBinding for that user and
// gateway. The deleted binding is excluded by ID in case the delete event races
// ahead of its soft-delete becoming visible.
func (r *RoleBindingReconciler) stillDesiredKcRoles(ctx context.Context, deleted *pb.RoleBinding) (map[string]bool, error) {
	desired := make(map[string]bool)
	if deleted.GatewayId == nil {
		return desired, nil
	}

	client := pb.NewRoleBindingServiceClient(r.grpcConn)
	resp, err := client.ListRoleBindings(ctx, &pb.ListRoleBindingsRequest{
		UserId:    deleted.UserId,
		GatewayId: deleted.GatewayId,
	})
	if err != nil {
		return nil, fmt.Errorf("list role bindings for user %s on gateway %s: %w", deleted.GetUserId(), *deleted.GatewayId, err)
	}

	return unionKcRoles(resp.GetItems(), deleted.GetMetadata().GetId()), nil
}

// unionKcRoles collects the Keycloak client roles granted by every binding in
// items except the one identified by excludeID (the binding being deleted).
func unionKcRoles(items []*pb.RoleBinding, excludeID string) map[string]bool {
	desired := make(map[string]bool)
	for _, rb := range items {
		if rb == nil || rb.GetMetadata().GetId() == excludeID {
			continue
		}
		for _, kcRole := range keycloakRoleMap[rb.RoleName] {
			desired[kcRole] = true
		}
	}
	return desired
}

// resolveKeycloakClientID looks up the gateway by ID and returns the Keycloak
// client ID in the {name}-{id} format specified by the Keycloak provisioning spec.
func (r *RoleBindingReconciler) resolveKeycloakClientID(ctx context.Context, gatewayID string) (string, error) {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	resp, err := client.GetGateway(ctx, &pb.GetGatewayRequest{Id: gatewayID})
	if err != nil {
		return "", fmt.Errorf("get gateway %s: %w", gatewayID, err)
	}
	return fmt.Sprintf("%s-%s", resp.GetGateway().GetName(), gatewayID), nil
}
