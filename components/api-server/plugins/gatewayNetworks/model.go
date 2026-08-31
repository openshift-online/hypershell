package gatewayNetworks

import (
	hypershellapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"gorm.io/gorm"
)

type GatewayNetwork struct {
	api.Meta
	hypershellapi.TraceMeta
	Name         string  `json:"name"`
	FleetId      string  `json:"fleet_id"`
	Topology     *string `json:"topology"`
	TunnelMode   *string `json:"tunnel_mode"`
	HubGatewayId *string `json:"hub_gateway_id"`
	Status       *string `json:"status"`
}

type GatewayNetworkList []*GatewayNetwork
type GatewayNetworkIndex map[string]*GatewayNetwork

func (l GatewayNetworkList) Index() GatewayNetworkIndex {
	index := GatewayNetworkIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (d *GatewayNetwork) BeforeCreate(tx *gorm.DB) error {
	d.ID = api.NewID()
	return nil
}

type GatewayNetworkPatchRequest struct {
	Name         *string `json:"name,omitempty"`
	FleetId      *string `json:"fleet_id,omitempty"`
	Topology     *string `json:"topology,omitempty"`
	TunnelMode   *string `json:"tunnel_mode,omitempty"`
	HubGatewayId *string `json:"hub_gateway_id,omitempty"`
	Status       *string `json:"status,omitempty"`
}
