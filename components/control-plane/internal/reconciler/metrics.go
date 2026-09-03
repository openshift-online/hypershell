package reconciler

import (
	"context"
	"sync"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
)

// observedGatewayProvisions coordinates the event-driven and health reconcile
// paths. These paths can observe the same first Running transition at the same
// time. The Gateway identifier stays in process memory and is not an OTel
// attribute.
var observedGatewayProvisions sync.Map

// observeGatewayProvisionDuration records the time from API creation until the
// first successful transition to Running. It ignores incomplete or invalid
// timestamps so telemetry cannot change reconciliation behavior.
func observeGatewayProvisionDuration(ctx context.Context, gw *pb.Gateway) {
	duration, ok := gatewayProvisionDuration(gw)
	if !ok {
		return
	}
	gatewayID := gw.GetMetadata().GetId()
	if !claimGatewayProvisionObservation(gatewayID) {
		return
	}
	cpotel.RecordGatewayProvisionDuration(ctx, duration)
}

func claimGatewayProvisionObservation(gatewayID string) bool {
	if gatewayID == "" {
		return false
	}
	_, loaded := observedGatewayProvisions.LoadOrStore(gatewayID, struct{}{})
	return !loaded
}

func forgetGatewayProvisionObservation(gatewayID string) {
	observedGatewayProvisions.Delete(gatewayID)
}

// suppressGatewayProvisionObservation prevents work on a previously Running or
// Degraded Gateway from producing a new provision observation. The caller uses
// the stored phase for a normal event. The retry adapter supplies the phase that
// it cleared when it bypasses the phase gate.
func suppressGatewayProvisionObservation(gatewayID, previousPhase string) {
	if previousPhase == gatewayPhaseRunning || previousPhase == gatewayPhaseDegraded {
		claimGatewayProvisionObservation(gatewayID)
	}
}

func gatewayProvisionDuration(gw *pb.Gateway) (time.Duration, bool) {
	if gw == nil || gw.GetMetadata() == nil {
		return 0, false
	}
	createdAt := gw.GetMetadata().GetCreatedAt()
	runningAt := gw.GetMetadata().GetUpdatedAt()
	if createdAt == nil || runningAt == nil {
		return 0, false
	}
	if err := createdAt.CheckValid(); err != nil {
		return 0, false
	}
	if err := runningAt.CheckValid(); err != nil {
		return 0, false
	}
	duration := runningAt.AsTime().Sub(createdAt.AsTime())
	if duration < 0 {
		return 0, false
	}
	return duration, true
}

// isGatewayProvisionCompletion excludes later recovery transitions. A new
// gateway can stay in Provisioning after the first reconcile while its route
// becomes ready. A Running gateway that fails moves through Degraded instead.
func isGatewayProvisionCompletion(currentPhase, desiredPhase string) bool {
	return currentPhase == gatewayPhaseProvisioning && desiredPhase == gatewayPhaseRunning
}
