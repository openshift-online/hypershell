package agentWorkspaces

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/util"
)

func ConvertAgentWorkspace(ws openapi.AgentWorkspace) *AgentWorkspace {
	c := &AgentWorkspace{
		Meta: api.Meta{
			ID: util.NilToEmptyString(ws.Id),
		},
		AgentRuntimeId: ws.AgentRuntimeId,
		GatewayId:      ws.GatewayId,
		Name:           ws.Name,
		Status:         ws.Status,
	}
	if ws.CreatedAt != nil {
		c.CreatedAt = *ws.CreatedAt
		c.UpdatedAt = *ws.UpdatedAt
	}
	return c
}

func PresentAgentWorkspace(ws *AgentWorkspace) openapi.AgentWorkspace {
	reference := presenters.PresentReference(ws.ID, ws)
	return openapi.AgentWorkspace{
		Id:             reference.Id,
		Kind:           reference.Kind,
		Href:           reference.Href,
		CreatedAt:      openapi.PtrTime(ws.CreatedAt),
		UpdatedAt:      openapi.PtrTime(ws.UpdatedAt),
		AgentRuntimeId: ws.AgentRuntimeId,
		GatewayId:      ws.GatewayId,
		Name:           ws.Name,
		Status:         ws.Status,
	}
}
