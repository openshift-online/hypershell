package managedDatabases

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/util"
)

func ConvertManagedDatabase(managedDatabase openapi.ManagedDatabase) *ManagedDatabase {
	c := &ManagedDatabase{
		Meta: api.Meta{
			ID: util.NilToEmptyString(managedDatabase.Id),
		},
	}
	c.Name = managedDatabase.Name
	c.FleetId = managedDatabase.FleetId
	c.Provider = managedDatabase.Provider
	c.Region = managedDatabase.Region
	c.Engine = managedDatabase.Engine
	c.EngineVersion = managedDatabase.EngineVersion
	c.InstanceClass = managedDatabase.InstanceClass
	c.ConnectionSecret = managedDatabase.ConnectionSecret
	c.Status = managedDatabase.Status

	if managedDatabase.CreatedAt != nil {
		c.CreatedAt = *managedDatabase.CreatedAt
		c.UpdatedAt = *managedDatabase.UpdatedAt
	}

	return c
}

func PresentManagedDatabase(managedDatabase *ManagedDatabase) openapi.ManagedDatabase {
	reference := presenters.PresentReference(managedDatabase.ID, managedDatabase)
	return openapi.ManagedDatabase{
		Id:               reference.Id,
		Kind:             reference.Kind,
		Href:             reference.Href,
		CreatedAt:        openapi.PtrTime(managedDatabase.CreatedAt),
		UpdatedAt:        openapi.PtrTime(managedDatabase.UpdatedAt),
		Name:             managedDatabase.Name,
		FleetId:          managedDatabase.FleetId,
		Provider:         managedDatabase.Provider,
		Namespace:        &managedDatabase.Namespace,
		Region:           managedDatabase.Region,
		Engine:           managedDatabase.Engine,
		EngineVersion:    managedDatabase.EngineVersion,
		InstanceClass:    managedDatabase.InstanceClass,
		ConnectionSecret: managedDatabase.ConnectionSecret,
		Status:           managedDatabase.Status,
	}
}
