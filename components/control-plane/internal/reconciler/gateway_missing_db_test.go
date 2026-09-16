package reconciler

import (
	"context"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/gatewayhealth"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
)

// TestGatewayMissingDatabaseIDSettlesFailed covers the external-db-only rule that
// a gateway whose database_id cannot be resolved is a non-recoverable error: the
// reconciler settles the phase to Failed before applying any workload rather than
// leaving it Provisioning for retry. See
// specs/platform/openshell-gateway-database.spec.md § Gateway Database Resolution.
func TestGatewayMissingDatabaseIDSettlesFailed(t *testing.T) {
	conn, recorder := newRecordingGatewayConn(t)

	r := &GatewayReconciler{
		active:   make(map[string]struct{}),
		grpcConn: conn,
	}

	pending := "Pending"
	gw := &pb.Gateway{
		Metadata:   &pb.ObjectReference{Id: "gw-no-db"},
		Name:       "e2e-gw",
		Phase:      &pending,
		DatabaseId: "",
	}
	event := watcher.Event[*pb.Gateway]{
		Type:       watcher.EventUpdated,
		ResourceID: "gw-no-db",
		Resource:   gw,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.Handle(ctx, event); err == nil {
		t.Fatal("gateway with empty database_id must fail the reconcile")
	}

	var settledPhase string
	for _, u := range recorder.snapshot() {
		if u.hasPhase {
			settledPhase = u.phase
		}
	}
	if settledPhase != string(gatewayhealth.PhaseFailed) {
		t.Fatalf("gateway with empty database_id must settle to phase %q, got %q", gatewayhealth.PhaseFailed, settledPhase)
	}
}
