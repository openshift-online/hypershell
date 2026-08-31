package fleets

import (
	hypershellapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"gorm.io/gorm"
)

type Fleet struct {
	api.Meta
	hypershellapi.TraceMeta
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
}

type FleetList []*Fleet
type FleetIndex map[string]*Fleet

func (l FleetList) Index() FleetIndex {
	index := FleetIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (d *Fleet) BeforeCreate(tx *gorm.DB) error {
	d.ID = api.NewID()
	return nil
}

type FleetPatchRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
}
