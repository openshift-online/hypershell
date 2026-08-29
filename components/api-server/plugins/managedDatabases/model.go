package managedDatabases

import (
	"encoding/hex"
	"fmt"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/segmentio/ksuid"
	"gorm.io/gorm"
)

const dbNamespacePrefix = "openshell-db-"

type ManagedDatabase struct {
	api.Meta
	Name             string  `json:"name"`
	Provider         string  `json:"provider"`
	Namespace        string  `json:"namespace"`
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

	id, err := ksuid.Parse(d.ID)
	if err != nil {
		return fmt.Errorf("parse generated managed database ID: %w", err)
	}
	d.Namespace = dbNamespacePrefix + hex.EncodeToString(id.Payload()[:8])
	return nil
}

type ManagedDatabasePatchRequest struct {
	Name             *string `json:"name,omitempty"`
	Provider         *string `json:"provider,omitempty"`
	Region           *string `json:"region,omitempty"`
	Engine           *string `json:"engine,omitempty"`
	EngineVersion    *string `json:"engine_version,omitempty"`
	InstanceClass    *string `json:"instance_class,omitempty"`
	ConnectionSecret *string `json:"connection_secret,omitempty"`
	Status           *string `json:"status,omitempty"`
}
