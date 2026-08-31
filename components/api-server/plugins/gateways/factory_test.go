package gateways_test

import (
	"context"
	"fmt"

	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
)

func newGateway(id string) (*gateways.Gateway, error) {
	gatewayService := gateways.Service(&environments.Environment().Services)

	gateway := &gateways.Gateway{
		Name:           "test-name",
		FleetId:        "test-fleet_id",
		ClusterId:      "test-cluster_id",
		ReleaseId:      "test-release_id",
		DatabaseId:     "test-database_id",
		ExternalDns:    stringPtr("test-external_dns"),
		TlsMode:        stringPtr("test-tls_mode"),
		ServiceType:    stringPtr("test-service_type"),
		Status:         stringPtr("test-status"),
		Phase:          stringPtr("test-phase"),
		GatewayVersion: stringPtr("0.0.109"),
	}

	sub, err := gatewayService.Create(context.Background(), gateway)
	if err != nil {
		return nil, err
	}

	return sub, nil
}

func newGatewayList(namePrefix string, count int) ([]*gateways.Gateway, error) {
	var items []*gateways.Gateway
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("%s_%d", namePrefix, i)
		c, err := newGateway(name)
		if err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, nil
}
func stringPtr(s string) *string { return &s }
