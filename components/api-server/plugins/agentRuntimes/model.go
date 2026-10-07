package agentRuntimes

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type AgentRuntime struct {
	api.Meta
	Name                string  `json:"name"`
	ClusterId           string  `json:"cluster_id"`
	GatewayId           string  `json:"gateway_id"`
	SandboxTemplateId   string  `json:"sandbox_template_id"`
	Description         *string `json:"description"`
	Cron                *string `json:"cron"`
	CoordinatorImage    *string `json:"coordinator_image"`
	ConcurrencyPolicy   *string `json:"concurrency_policy"`
	LoginRefreshSeconds *int32  `json:"login_refresh_seconds"`
	Parameters          *string `json:"parameters" gorm:"type:jsonb"`
	Status              *string `json:"status"`
}

type AgentRuntimeList []*AgentRuntime

type AgentRuntimePatchRequest struct {
	Name                *string `json:"name"`
	SandboxTemplateId   *string `json:"sandbox_template_id"`
	Description         *string `json:"description"`
	Cron                *string `json:"cron"`
	CoordinatorImage    *string `json:"coordinator_image"`
	ConcurrencyPolicy   *string `json:"concurrency_policy"`
	LoginRefreshSeconds *int32  `json:"login_refresh_seconds"`
	Parameters          *string `json:"parameters"`
}

func (d *AgentRuntime) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
