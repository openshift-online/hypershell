package agentRuntimes

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type AgentRuntime struct {
	api.Meta
	Name                string   `json:"name"`
	ClusterId           string   `json:"cluster_id"`
	GatewayId           string   `json:"gateway_id"`
	SandboxTemplateId   string   `json:"sandbox_template_id"`
	RepositoryId        *string  `json:"repository_id"`
	Selector            []string `json:"selector" gorm:"serializer:json;type:jsonb"`
	Description         *string  `json:"description"`
	Cron                *string  `json:"cron"`
	CoordinatorImage    *string  `json:"coordinator_image"`
	ConcurrencyPolicy   *string  `json:"concurrency_policy"`
	LoginRefreshSeconds *int32   `json:"login_refresh_seconds"`
	Parameters          *string  `json:"parameters" gorm:"type:jsonb"`
	Status              *string  `json:"status"`
	// StatusMessage carries the human-readable detail behind Status - for a
	// Degraded runtime, the failing SecretSource name and gateway-side error. It
	// is control-plane-owned, written only through the UpdateAgentRuntimeStatus
	// gRPC path (data-model.spec.md "AgentRuntime Status Lifecycle").
	StatusMessage *string `json:"status_message"`
}

type AgentRuntimeList []*AgentRuntime

type AgentRuntimePatchRequest struct {
	Name                *string  `json:"name"`
	SandboxTemplateId   *string  `json:"sandbox_template_id"`
	RepositoryId        *string  `json:"repository_id"`
	Selector            []string `json:"selector"`
	Description         *string  `json:"description"`
	Cron                *string  `json:"cron"`
	CoordinatorImage    *string  `json:"coordinator_image"`
	ConcurrencyPolicy   *string  `json:"concurrency_policy"`
	LoginRefreshSeconds *int32   `json:"login_refresh_seconds"`
	Parameters          *string  `json:"parameters"`
}

func (d *AgentRuntime) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
