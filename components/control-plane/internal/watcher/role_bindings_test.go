package watcher

import (
	"errors"
	"fmt"
	"testing"

	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsMissingKeycloakClient(t *testing.T) {
	notFound := fmt.Errorf("assign role: %w", &keycloak.ClientNotFoundError{ClientID: "gateway-1"})
	if !isMissingKeycloakClient(notFound) {
		t.Fatal("a wrapped client-not-found error must be retried")
	}
	if isMissingKeycloakClient(errors.New("permission denied")) {
		t.Fatal("a permanent error must not be retried")
	}
}

func TestIsRoleBindingRetryable(t *testing.T) {
	notFound := fmt.Errorf("assign role: %w", &keycloak.ClientNotFoundError{ClientID: "gateway-1"})
	if !isRoleBindingRetryable(notFound) {
		t.Fatal("a missing Keycloak client must be retried")
	}
	if !isRoleBindingRetryable(errors.New("role binding rb-1: username not yet resolved")) {
		t.Fatal("an unresolved username must be retried")
	}
	if !isRoleBindingRetryable(fmt.Errorf("get role UUID: keycloak GET returned 404")) {
		t.Fatal("a half-provisioned Keycloak client (role 404) must be retried")
	}

	gatewayMissing := status.Error(codes.NotFound, "Gateway with id='gw-1' not found")
	wrappedGateway := fmt.Errorf("get gateway gw-1: %w", gatewayMissing)
	if isRoleBindingRetryable(wrappedGateway) {
		t.Fatal("a missing gateway must not be retried")
	}
	if isRoleBindingRetryable(nil) {
		t.Fatal("a nil error must not be retried")
	}
}
