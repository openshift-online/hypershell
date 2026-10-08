package providerBindings

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type ProviderBinding struct {
	api.Meta
	Name            string  `json:"name"`
	WorkspaceId     string  `json:"workspace_id"`
	ProviderSpecId  string  `json:"provider_spec_id"`
	SecretSourceId  string  `json:"secret_source_id"`
	RefreshStrategy *string `json:"refresh_strategy" gorm:"type:jsonb"`
	Status          *string `json:"status"`
}

type ProviderBindingList []*ProviderBinding

type ProviderBindingPatchRequest struct {
	Name            *string `json:"name"`
	SecretSourceId  *string `json:"secret_source_id"`
	RefreshStrategy *string `json:"refresh_strategy"`
}

func (d *ProviderBinding) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
