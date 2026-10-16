package providerSpecs

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type ProviderSpec struct {
	api.Meta
	Name       string  `json:"name"`
	Category   string  `json:"category"`
	Capability *string `json:"capability" gorm:"type:jsonb"`
	Profile    *string `json:"profile" gorm:"type:jsonb"`
	// RepositoryId is the ownership mark set by the control-plane declarative
	// import reconciler when this resource originates from a Repository's
	// .hypershell import. It is nil for resources created directly through the
	// REST API or hsctl apply, which are never pruned by the import. It is
	// read-only in the client contract (set server-side, never by a client).
	RepositoryId *string `json:"repository_id"`
	Status       *string `json:"status"`
}

type ProviderSpecList []*ProviderSpec

type ProviderSpecPatchRequest struct {
	Name       *string `json:"name"`
	Capability *string `json:"capability"`
	Profile    *string `json:"profile"`
}

func (d *ProviderSpec) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
