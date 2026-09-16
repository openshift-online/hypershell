package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// orphanRecord captures one RecordOrphan invocation for assertions.
type orphanRecord struct {
	kind   string
	name   string
	reason string
}

// bestEffortKeycloak succeeds at the retryable service-account step but fails
// the two best-effort parent-client deletions, exercising the orphan-recording
// branches without pinning the delete (which the service-account failure would).
type bestEffortKeycloak struct {
	KeycloakClientAPI
	consoleErr error
	gatewayErr error
}

func (k *bestEffortKeycloak) DeleteGatewayServiceAccountClients(context.Context, string) error {
	return nil
}
func (k *bestEffortKeycloak) DeleteConsoleClient(context.Context, string) error { return k.consoleErr }
func (k *bestEffortKeycloak) DeleteGatewayClient(context.Context, string) error { return k.gatewayErr }

// A Keycloak parent-client delete that fails is best-effort: it leaves an
// orphaned realm client with no cascading owner and no reclaiming reconciler.
// DeleteGatewayResources must still return nil (so the delete tombstone is not
// pinned forever) but must record each orphan durably via RecordOrphan, so the
// leak is operator-visible rather than silent.
func TestDeleteGatewayResources_RecordsOrphanedKeycloakClients(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	kc := &bestEffortKeycloak{
		consoleErr: errors.New("keycloak unavailable"),
		gatewayErr: errors.New("keycloak unavailable"),
	}

	var recorded []orphanRecord
	opts := ReconcileOpts{
		KeycloakClient:  kc,
		GatewayID:       "gateway-id",
		GatewayName:     "gw",
		GatewayClientID: "gw-gateway-id",
		RecordOrphan: func(_ context.Context, kind, name, reason string) {
			recorded = append(recorded, orphanRecord{kind, name, reason})
		},
	}

	if err := DeleteGatewayResources(context.Background(), client, nil, "gateway-ns", opts); err != nil {
		t.Fatalf("best-effort Keycloak failures must not fail deletion: %v", err)
	}

	// Expect an orphan record for both the console and the gateway realm client.
	wantNames := map[string]bool{"gw-gateway-id-console": false, "gw-gateway-id": false}
	for _, r := range recorded {
		if r.kind != "KeycloakClient" {
			t.Errorf("orphan kind = %q, want KeycloakClient", r.kind)
		}
		if _, ok := wantNames[r.name]; ok {
			wantNames[r.name] = true
		}
		if !strings.Contains(r.reason, "keycloak unavailable") {
			t.Errorf("orphan reason %q should carry the underlying delete error", r.reason)
		}
	}
	for name, seen := range wantNames {
		if !seen {
			t.Errorf("expected an orphan record for Keycloak client %q", name)
		}
	}
}

// When no RecordOrphan is wired (the backward-compatible default), a best-effort
// failure must still not fail deletion and must not panic.
func TestDeleteGatewayResources_NilRecorderIsNoOp(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	kc := &bestEffortKeycloak{
		consoleErr: errors.New("keycloak unavailable"),
		gatewayErr: errors.New("keycloak unavailable"),
	}
	opts := ReconcileOpts{
		KeycloakClient:  kc,
		GatewayID:       "gateway-id",
		GatewayName:     "gw",
		GatewayClientID: "gw-gateway-id",
	}

	if err := DeleteGatewayResources(context.Background(), client, nil, "gateway-ns", opts); err != nil {
		t.Fatalf("nil recorder must be a no-op, got error: %v", err)
	}
}
