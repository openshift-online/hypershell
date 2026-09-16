package gateways_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedDatabases"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
)

// testManagedDatabaseSecret is the credentials namespace reference of the
// ManagedDatabase every gateway test is placed onto. It only has to pass the
// API server's reference validation; no Kubernetes Secret is read here.
const testManagedDatabaseSecret = "hypershell-managed-db-test"

// registerIntegration resets the database like test.RegisterIntegration and
// then registers one ManagedDatabase. Gateway placement assigns every new
// gateway to the oldest registered ManagedDatabase and rejects creation when
// none exists, so gateway tests need this fixture before they can create a
// gateway. Tests that assert the rejection itself call
// test.RegisterIntegration directly.
func registerIntegration(t *testing.T) (*test.Helper, *openapi.APIClient) {
	t.Helper()
	h, client := test.RegisterIntegration(t)
	seedManagedDatabase(t)
	return h, client
}

// seedManagedDatabase registers a ManagedDatabase with a valid connection_secret
// reference and returns it.
func seedManagedDatabase(t *testing.T) *managedDatabases.ManagedDatabase {
	t.Helper()
	svc := managedDatabases.Service(&environments.Environment().Services)
	db, err := svc.Create(context.Background(), &managedDatabases.ManagedDatabase{
		Name:             "test-managed-database",
		ConnectionSecret: stringPtr(testManagedDatabaseSecret),
	})
	if err != nil {
		t.Fatalf("seed ManagedDatabase for gateway placement: %v", err)
	}
	return db
}

func newGateway(id string) (*gateways.Gateway, error) {
	gatewayService := gateways.Service(&environments.Environment().Services)

	gateway := &gateways.Gateway{
		Name:           "test-name",
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
