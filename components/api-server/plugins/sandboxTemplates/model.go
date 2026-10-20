package sandboxTemplates

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type SandboxTemplate struct {
	api.Meta
	Name       string  `json:"name"`
	Image      string  `json:"image"`
	NamePrefix *string `json:"name_prefix"`
	Policy     *string `json:"policy" gorm:"type:jsonb"`
	// RepositoryId is the ownership mark set by the control-plane declarative
	// import reconciler when this resource originates from a Repository's
	// .hypershell import. It is nil for resources created directly through the
	// REST API or hsctl apply, which are never pruned by the import. It is
	// read-only in the client contract (set server-side, never by a client).
	RepositoryId *string `json:"repository_id"`
	Status       *string `json:"status"`
}

type SandboxTemplateList []*SandboxTemplate

type SandboxTemplatePatchRequest struct {
	Name       *string `json:"name"`
	Image      *string `json:"image"`
	NamePrefix *string `json:"name_prefix"`
	Policy     *string `json:"policy"`
}

func (d *SandboxTemplate) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
