package watcher

import (
	"errors"
	"fmt"
	"testing"

	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
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
