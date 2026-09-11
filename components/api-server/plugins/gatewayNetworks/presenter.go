package gatewayNetworks

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/util"
)

func ConvertGatewayNetwork(gatewayNetwork openapi.GatewayNetwork) *GatewayNetwork {
	c := &GatewayNetwork{
		Meta: api.Meta{
			ID: util.NilToEmptyString(gatewayNetwork.Id),
		},
	}
	c.Name = gatewayNetwork.Name
	c.Topology = gatewayNetwork.Topology
	c.TunnelMode = gatewayNetwork.TunnelMode
	c.HubGatewayId = gatewayNetwork.HubGatewayId
	c.Status = gatewayNetwork.Status

	if gatewayNetwork.CreatedAt != nil {
		c.CreatedAt = *gatewayNetwork.CreatedAt
		c.UpdatedAt = *gatewayNetwork.UpdatedAt
	}

	return c
}

func PresentGatewayNetwork(gatewayNetwork *GatewayNetwork) openapi.GatewayNetwork {
	reference := presenters.PresentReference(gatewayNetwork.ID, gatewayNetwork)
	return openapi.GatewayNetwork{
		Id:           reference.Id,
		Kind:         reference.Kind,
		Href:         reference.Href,
		CreatedAt:    openapi.PtrTime(gatewayNetwork.CreatedAt),
		UpdatedAt:    openapi.PtrTime(gatewayNetwork.UpdatedAt),
		Name:         gatewayNetwork.Name,
		Topology:     gatewayNetwork.Topology,
		TunnelMode:   gatewayNetwork.TunnelMode,
		HubGatewayId: gatewayNetwork.HubGatewayId,
		Status:       gatewayNetwork.Status,
	}
}
