package inferenceRoutes

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type InferenceRoute struct {
	api.Meta
	WorkspaceId      string `json:"workspace_id"`
	ProviderBindingId string `json:"provider_binding_id"`
	Model            string `json:"model"`
}

type InferenceRouteList []*InferenceRoute

func (d *InferenceRoute) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}
