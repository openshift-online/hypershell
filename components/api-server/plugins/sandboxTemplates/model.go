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
	Status     *string `json:"status"`
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
