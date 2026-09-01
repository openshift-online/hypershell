package managedClusters

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/util"
)

func ConvertManagedCluster(managedCluster openapi.ManagedCluster) *ManagedCluster {
	c := &ManagedCluster{
		Meta: api.Meta{
			ID: util.NilToEmptyString(managedCluster.Id),
		},
	}
	c.Name = managedCluster.Name
	c.Provider = managedCluster.Provider
	c.Region = managedCluster.Region
	c.KubeconfigSecret = managedCluster.KubeconfigSecret
	c.Status = managedCluster.Status
	c.ApiServerUrl = managedCluster.ApiServerUrl

	if managedCluster.CreatedAt != nil {
		c.CreatedAt = *managedCluster.CreatedAt
		c.UpdatedAt = *managedCluster.UpdatedAt
	}

	return c
}

func PresentManagedCluster(managedCluster *ManagedCluster) openapi.ManagedCluster {
	reference := presenters.PresentReference(managedCluster.ID, managedCluster)
	return openapi.ManagedCluster{
		Id:               reference.Id,
		Kind:             reference.Kind,
		Href:             reference.Href,
		CreatedAt:        openapi.PtrTime(managedCluster.CreatedAt),
		UpdatedAt:        openapi.PtrTime(managedCluster.UpdatedAt),
		Name:             managedCluster.Name,
		Provider:         managedCluster.Provider,
		Region:           managedCluster.Region,
		KubeconfigSecret: managedCluster.KubeconfigSecret,
		Status:           managedCluster.Status,
		ApiServerUrl:     managedCluster.ApiServerUrl,
	}
}
