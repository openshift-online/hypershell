package secretSources

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type SecretSource struct {
	api.Meta
	Name           string  `json:"name"`
	AgentRuntimeId string  `json:"agent_runtime_id"`
	Purpose        string  `json:"purpose"`
	Backend        string  `json:"backend"`
	Path           string  `json:"path"`
	KeyMappings    *string `json:"key_mappings" gorm:"type:jsonb"`
}

type SecretSourceList []*SecretSource

type SecretSourcePatchRequest struct {
	Name        *string `json:"name"`
	Purpose     *string `json:"purpose"`
	Backend     *string `json:"backend"`
	Path        *string `json:"path"`
	KeyMappings *string `json:"key_mappings"`
}

func (d *SecretSource) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
