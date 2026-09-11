package gatewayNetworks_test

import (
	"context"
	"fmt"

	"github.com/openshift-online/hypershell/components/api-server/plugins/gatewayNetworks"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
)

func newGatewayNetwork(id string) (*gatewayNetworks.GatewayNetwork, error) {
	gatewayNetworkService := gatewayNetworks.Service(&environments.Environment().Services)

	gatewayNetwork := &gatewayNetworks.GatewayNetwork{
		Name:         "test-name",
		Topology:     stringPtr("test-topology"),
		TunnelMode:   stringPtr("test-tunnel_mode"),
		HubGatewayId: stringPtr("test-hub_gateway_id"),
		Status:       stringPtr("test-status"),
	}

	sub, err := gatewayNetworkService.Create(context.Background(), gatewayNetwork)
	if err != nil {
		return nil, err
	}

	return sub, nil
}

func newGatewayNetworkList(namePrefix string, count int) ([]*gatewayNetworks.GatewayNetwork, error) {
	var items []*gatewayNetworks.GatewayNetwork
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("%s_%d", namePrefix, i)
		c, err := newGatewayNetwork(name)
		if err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, nil
}
func stringPtr(s string) *string { return &s }
