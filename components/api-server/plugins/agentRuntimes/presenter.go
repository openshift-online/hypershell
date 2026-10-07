package agentRuntimes

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/util"
)

func ConvertAgentRuntime(ar openapi.AgentRuntime) *AgentRuntime {
	c := &AgentRuntime{
		Meta: api.Meta{
			ID: util.NilToEmptyString(ar.Id),
		},
		Name:      ar.Name,
		ClusterId: ar.ClusterId,
		GatewayId: ar.GatewayId,
	}
	if ar.SandboxTemplateId != nil {
		c.SandboxTemplateId = *ar.SandboxTemplateId
	}
	c.Description = ar.Description
	c.Cron = ar.Cron
	c.CoordinatorImage = ar.CoordinatorImage
	c.ConcurrencyPolicy = ar.ConcurrencyPolicy
	c.Parameters = ar.Parameters
	if ar.LoginRefreshSeconds != nil {
		v := *ar.LoginRefreshSeconds
		c.LoginRefreshSeconds = &v
	}
	if ar.CreatedAt != nil {
		c.CreatedAt = *ar.CreatedAt
		c.UpdatedAt = *ar.UpdatedAt
	}
	return c
}

func PresentAgentRuntime(ar *AgentRuntime) openapi.AgentRuntime {
	reference := presenters.PresentReference(ar.ID, ar)
	result := openapi.AgentRuntime{
		Id:                reference.Id,
		Kind:              reference.Kind,
		Href:              reference.Href,
		CreatedAt:         openapi.PtrTime(ar.CreatedAt),
		UpdatedAt:         openapi.PtrTime(ar.UpdatedAt),
		Name:              ar.Name,
		ClusterId:         ar.ClusterId,
		GatewayId:         ar.GatewayId,
		Description:       ar.Description,
		Cron:              ar.Cron,
		CoordinatorImage:  ar.CoordinatorImage,
		ConcurrencyPolicy: ar.ConcurrencyPolicy,
		Parameters:        ar.Parameters,
		Status:            ar.Status,
	}
	if ar.SandboxTemplateId != "" {
		result.SandboxTemplateId = &ar.SandboxTemplateId
	}
	if ar.LoginRefreshSeconds != nil {
		result.LoginRefreshSeconds = ar.LoginRefreshSeconds
	}
	return result
}
