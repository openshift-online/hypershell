package agentWorkspaces

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

// AgentWorkspace is the desired state of the gateway-side workspace an
// AgentRuntime owns. Exactly one AgentWorkspace exists per AgentRuntime, bound to
// agent_runtime_id, and it is placed on the AgentRuntime's gateway_id. The name is
// unique on a gateway (data-model.spec.md "Gateway-Side Provisioning", "Workspace
// Name Collision"). The control plane reconciles this desired state onto the
// gateway through the gateway provisioning gRPC contract
// (agent-workspace-gateway-api.spec.md); the gateway remains authoritative, so the
// collision check here is advisory, reported at request time.
type AgentWorkspace struct {
	api.Meta
	AgentRuntimeId string  `json:"agent_runtime_id"`
	GatewayId      string  `json:"gateway_id"`
	Name           string  `json:"name"`
	Status         *string `json:"status"`
}

type AgentWorkspaceList []*AgentWorkspace

type AgentWorkspacePatchRequest struct {
	Name   *string `json:"name"`
	Status *string `json:"status"`
}

func (d *AgentWorkspace) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
