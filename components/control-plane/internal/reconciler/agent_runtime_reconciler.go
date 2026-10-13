package reconciler

import (
	"context"
	"log"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
)

// AgentRuntimeReconciler is the Wave A no-op handler for AgentRuntime watch
// events (issue #521). It logs each delivered event and returns success. The
// real reconciler - cluster scheduling resources (Namespace, ServiceAccount,
// NetworkPolicy, ExternalSecrets, coordinator CronJob), gateway-side workspace
// provisioning, and the Pending -> Provisioning -> Running -> Degraded ->
// Terminating status lifecycle defined in data-model.spec.md ("AgentRuntime
// Cluster Resource Provisioning", "Gateway-Side Provisioning", "AgentRuntime
// Status Lifecycle") - is deferred to Wave B. Wiring the watch transport now lets
// the control plane observe AgentRuntime events without yet acting on them.
type AgentRuntimeReconciler struct{}

func NewAgentRuntimeReconciler() *AgentRuntimeReconciler {
	return &AgentRuntimeReconciler{}
}

func (r *AgentRuntimeReconciler) Handle(ctx context.Context, event watcher.Event[*pb.AgentRuntime]) error {
	clusterID := event.Resource.GetClusterId()
	gatewayID := event.Resource.GetGatewayId()
	log.Printf("INFO AgentRuntime watch event (no-op in Wave A): %s id=%s cluster_id=%q gateway_id=%q; cluster/gateway-side reconciliation is deferred to Wave B",
		event.Type, event.ResourceID, clusterID, gatewayID)
	return nil
}
