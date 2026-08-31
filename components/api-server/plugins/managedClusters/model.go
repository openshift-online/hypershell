package managedClusters

import (
	hypershellapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"gorm.io/gorm"
)

type ManagedCluster struct {
	api.Meta
	hypershellapi.TraceMeta
	Name             string  `json:"name"`
	FleetId          string  `json:"fleet_id"`
	Provider         string  `json:"provider"`
	Region           *string `json:"region"`
	KubeconfigSecret string  `json:"kubeconfig_secret"`
	Status           *string `json:"status"`
	ApiServerUrl     *string `json:"api_server_url"`
}

type ManagedClusterList []*ManagedCluster
type ManagedClusterIndex map[string]*ManagedCluster

func (l ManagedClusterList) Index() ManagedClusterIndex {
	index := ManagedClusterIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (d *ManagedCluster) BeforeCreate(tx *gorm.DB) error {
	d.ID = api.NewID()
	return nil
}

type ManagedClusterPatchRequest struct {
	Name             *string `json:"name,omitempty"`
	FleetId          *string `json:"fleet_id,omitempty"`
	Provider         *string `json:"provider,omitempty"`
	Region           *string `json:"region,omitempty"`
	KubeconfigSecret *string `json:"kubeconfig_secret,omitempty"`
	Status           *string `json:"status,omitempty"`
	ApiServerUrl     *string `json:"api_server_url,omitempty"`
}
