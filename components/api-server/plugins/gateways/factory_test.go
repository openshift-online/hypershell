package gateways_test

import (
	"context"
	"fmt"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// controlPlaneWriteContext marks a direct service write as the control-plane internal
// reconcile path so it bypasses the default-deny gateway field policy (PLAT-DATA-23).
// Test factories seed fully-provisioned gateways (platform-managed fields set) directly
// through the service, which only the control plane may author; a user/import write of
// those fields is rejected.
func controlPlaneWriteContext() context.Context {
	return rbac.WithGatewayWriterKind(context.Background(), rbac.GatewayWriterControlPlane)
}

func newGateway(id string) (*gateways.Gateway, error) {
	gatewayService := gateways.Service(&environments.Environment().Services)

	gateway := &gateways.Gateway{
		Name:           "test-name",
		ClusterId:      "test-cluster_id",
		ExternalDns:    stringPtr("test-external_dns"),
		TlsMode:        stringPtr("test-tls_mode"),
		ServiceType:    stringPtr("test-service_type"),
		Status:         stringPtr("test-status"),
		Phase:          stringPtr("test-phase"),
		GatewayVersion: stringPtr("0.0.109"),
	}

	sub, err := gatewayService.Create(controlPlaneWriteContext(), gateway)
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
