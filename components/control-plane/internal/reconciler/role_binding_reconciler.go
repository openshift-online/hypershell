package reconciler

import (
	"context"
	"fmt"
	"log"
	"sync"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
)

var keycloakRoleMap = map[string]string{
	"gateway:owner":  "openshell-admin",
	"gateway:viewer": "openshell-user",
}

type RoleBindingReconciler struct {
	mu             sync.Mutex
	active         map[string]struct{}
	keycloakClient *keycloak.Client
}

func NewRoleBindingReconciler(keycloakClient *keycloak.Client) *RoleBindingReconciler {
	return &RoleBindingReconciler{
		active:         make(map[string]struct{}),
		keycloakClient: keycloakClient,
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

	if rb.GatewayId == nil || *rb.GatewayId == "" {
		log.Printf("DEBUG role binding %s: no gateway_id (global scope), skipping keycloak sync", event.ResourceID)
		return nil
	}

	kcRole, ok := keycloakRoleMap[rb.RoleName]
	if !ok {
		log.Printf("DEBUG role binding %s: role %q has no keycloak mapping, skipping", event.ResourceID, rb.RoleName)
		return nil
	}

	username := rb.Username
	if username == "" {
		log.Printf("WARN role binding %s: no username available, skipping keycloak sync", event.ResourceID)
		return nil
	}

	gatewayName := *rb.GatewayId

	switch event.Type {
	case watcher.EventCreated, watcher.EventUpdated:
		log.Printf("INFO assigning keycloak role %s to user %s on gateway %s", kcRole, username, gatewayName)
		if err := r.keycloakClient.AssignClientRole(ctx, gatewayName, username, kcRole); err != nil {
			return fmt.Errorf("assign keycloak role %s to user %s on gateway %s: %w", kcRole, username, gatewayName, err)
		}

	case watcher.EventDeleted:
		log.Printf("INFO removing keycloak role %s from user %s on gateway %s", kcRole, username, gatewayName)
		if err := r.keycloakClient.RemoveClientRole(ctx, gatewayName, username, kcRole); err != nil {
			return fmt.Errorf("remove keycloak role %s from user %s on gateway %s: %w", kcRole, username, gatewayName, err)
		}
	}

	return nil
}
