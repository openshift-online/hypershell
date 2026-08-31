package gatewayReleases

import (
	hypershellapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"gorm.io/gorm"
)

type GatewayRelease struct {
	api.Meta
	hypershellapi.TraceMeta
	Name            string  `json:"name"`
	FleetId         string  `json:"fleet_id"`
	Image           string  `json:"image"`
	RolloutStrategy *string `json:"rollout_strategy"`
	CanaryPercent   *int    `json:"canary_percent"`
	CanaryDuration  *string `json:"canary_duration"`
	Status          *string `json:"status"`
}

type GatewayReleaseList []*GatewayRelease
type GatewayReleaseIndex map[string]*GatewayRelease

func (l GatewayReleaseList) Index() GatewayReleaseIndex {
	index := GatewayReleaseIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (d *GatewayRelease) BeforeCreate(tx *gorm.DB) error {
	d.ID = api.NewID()
	return nil
}

type GatewayReleasePatchRequest struct {
	Name            *string `json:"name,omitempty"`
	FleetId         *string `json:"fleet_id,omitempty"`
	Image           *string `json:"image,omitempty"`
	RolloutStrategy *string `json:"rollout_strategy,omitempty"`
	CanaryPercent   *int    `json:"canary_percent,omitempty"`
	CanaryDuration  *string `json:"canary_duration,omitempty"`
	Status          *string `json:"status,omitempty"`
}
