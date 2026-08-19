package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
)

var keycloakRoleMap = map[string]string{
	"gateway:owner":  "openshell-admin",
	"gateway:viewer": "openshell-user",
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

	kcClientID, err := r.resolveKeycloakClientID(ctx, *rb.GatewayId)
	if err != nil {
		return fmt.Errorf("resolve keycloak client id for role binding %s: %w", event.ResourceID, err)
	}

	switch event.Type {
	case watcher.EventCreated, watcher.EventUpdated:
		log.Printf("INFO assigning keycloak role %s to user %s on client %s", kcRole, username, kcClientID)
		if err := r.assignClientRoleWithRetry(ctx, kcClientID, username, kcRole); err != nil {
			return fmt.Errorf("assign keycloak role %s to user %s on client %s: %w", kcRole, username, kcClientID, err)
		}

	case watcher.EventDeleted:
		log.Printf("INFO removing keycloak role %s from user %s on client %s", kcRole, username, kcClientID)
		if err := r.keycloakClient.RemoveClientRole(ctx, kcClientID, username, kcRole); err != nil {
			return fmt.Errorf("remove keycloak role %s from user %s on client %s: %w", kcRole, username, kcClientID, err)
		}
	}

	return nil
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

// assignClientRoleWithRetry retries AssignClientRole to handle the race where
// the RoleBinding event arrives before the Gateway reconciler has finished
// provisioning the Keycloak client. Only ClientNotFoundError is retried;
// permanent failures (bad user, auth error) are returned immediately.
func (r *RoleBindingReconciler) assignClientRoleWithRetry(ctx context.Context, kcClientID, username, kcRole string) error {
	const maxAttempts = 10
	backoff := 2 * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr = r.keycloakClient.AssignClientRole(ctx, kcClientID, username, kcRole)
		if lastErr == nil {
			return nil
		}

		var notFound *keycloak.ClientNotFoundError
		if !errors.As(lastErr, &notFound) {
			return lastErr
		}

		if attempt == maxAttempts {
			break
		}

		log.Printf("INFO keycloak role assignment attempt %d/%d: client %s not yet provisioned (retrying in %v)",
			attempt, maxAttempts, kcClientID, backoff)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
	return lastErr
}
