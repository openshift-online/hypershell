package managedDatabases_test

import (
	"context"
	"fmt"

	"github.com/openshift-online/hypershell/components/api-server/plugins/managedDatabases"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
)

func newManagedDatabase(id string) (*managedDatabases.ManagedDatabase, error) {
	managedDatabaseService := managedDatabases.Service(&environments.Environment().Services)

	managedDatabase := &managedDatabases.ManagedDatabase{
		Name:             "test-name",
		FleetId:          "test-fleet_id",
		Provider:         "deployment",
		Region:           stringPtr("test-region"),
		Engine:           stringPtr("test-engine"),
		EngineVersion:    stringPtr("test-engine_version"),
		InstanceClass:    stringPtr("test-instance_class"),
		ConnectionSecret: stringPtr("test-connection_secret"),
		Status:           stringPtr("test-status"),
	}

	sub, err := managedDatabaseService.Create(context.Background(), managedDatabase)
	if err != nil {
		return nil, err
	}

	return sub, nil
}

func newManagedDatabaseList(namePrefix string, count int) ([]*managedDatabases.ManagedDatabase, error) {
	var items []*managedDatabases.ManagedDatabase
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("%s_%d", namePrefix, i)
		c, err := newManagedDatabase(name)
		if err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, nil
}
func stringPtr(s string) *string { return &s }
