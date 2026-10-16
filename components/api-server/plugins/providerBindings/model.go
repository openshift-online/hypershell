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
	// RepositoryId is the ownership mark set by the control-plane declarative
	// import reconciler when this resource originates from a Repository's
	// .hypershell import. It is nil for resources created directly through the
	// REST API or hsctl apply, which are never pruned by the import. It is
	// read-only in the client contract (set server-side, never by a client).
	RepositoryId *string `json:"repository_id"`
	Status       *string `json:"status"`
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
