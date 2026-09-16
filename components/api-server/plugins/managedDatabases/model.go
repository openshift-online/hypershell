package managedDatabases

import (
	hypershellapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"gorm.io/gorm"
)

// ManagedDatabase registers an externally provisioned PostgreSQL server that
// gateways are placed onto. HyperShell never creates, resizes or deletes the
// server; it provisions one database and login role per gateway inside it, so
// the record carries no provider selector and owns no Kubernetes namespace.
type ManagedDatabase struct {
	api.Meta
	hypershellapi.TraceMeta
	Name             string  `json:"name"`
	Region           *string `json:"region"`
	Engine           *string `json:"engine"`
	EngineVersion    *string `json:"engine_version"`
	InstanceClass    *string `json:"instance_class"`
	ConnectionSecret *string `json:"connection_secret"`
	Status           *string `json:"status"`
}

type ManagedDatabaseList []*ManagedDatabase
type ManagedDatabaseIndex map[string]*ManagedDatabase

func (l ManagedDatabaseList) Index() ManagedDatabaseIndex {
	index := ManagedDatabaseIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (d *ManagedDatabase) BeforeCreate(tx *gorm.DB) error {
	d.ID = api.NewID()
	return nil
}

type ManagedDatabasePatchRequest struct {
	Name             *string `json:"name,omitempty"`
	Region           *string `json:"region,omitempty"`
	Engine           *string `json:"engine,omitempty"`
	EngineVersion    *string `json:"engine_version,omitempty"`
	InstanceClass    *string `json:"instance_class,omitempty"`
	ConnectionSecret *string `json:"connection_secret,omitempty"`
	Status           *string `json:"status,omitempty"`
}
