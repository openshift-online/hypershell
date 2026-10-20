package managedClusters

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/util"
)

func ConvertManagedCluster(managedCluster openapi.ManagedCluster) *ManagedCluster {
	c := &ManagedCluster{
		Meta: api.Meta{
			ID: util.NilToEmptyString(managedCluster.Id),
		},
	}
	c.Name = managedCluster.Name
	c.Provider = managedCluster.Provider
	c.Visibility = managedCluster.GetVisibility()
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
	var visibility *string
	if managedCluster.Visibility != "" {
		visibility = &managedCluster.Visibility
	}
	// Registered clusters self-declare provider/region through the capability
	// model and no longer write the legacy Provider/Region columns. Present the
	// column when set (manually created records) and fall back to the provider/
	// and region/ capability labels, so the inventory dashboard that buckets by
	// these fields does not report every registered cluster as "unknown".
	result := openapi.ManagedCluster{
		Id:               reference.Id,
		Kind:             reference.Kind,
		Href:             reference.Href,
		CreatedAt:        openapi.PtrTime(managedCluster.CreatedAt),
		UpdatedAt:        openapi.PtrTime(managedCluster.UpdatedAt),
		Name:             managedCluster.Name,
		Provider:         clusterProvider(managedCluster),
		Visibility:       visibility,
		Region:           presentedRegion(managedCluster),
		KubeconfigSecret: managedCluster.KubeconfigSecret,
		Status:           managedCluster.Status,
		ApiServerUrl:     managedCluster.ApiServerUrl,
	}
	if managedCluster.OIDCSubject != "" {
		result.OidcSubject = &managedCluster.OIDCSubject
	}
	result.LastSeenAt = managedCluster.LastSeenAt
	result.Capabilities = append([]string(nil), managedCluster.Capabilities...)
	return result
}

// presentedRegion returns the region to expose on the API: the column when it is
// set, otherwise the region/ capability value, preserving a nil column when no
// region is known either way.
func presentedRegion(managedCluster *ManagedCluster) *string {
	region := clusterRegion(managedCluster)
	if region == "" {
		return managedCluster.Region
	}
	return &region
}

func PresentRegistrationResponse(clusterID string) openapi.ManagedClusterRegistrationResponse {
	return openapi.ManagedClusterRegistrationResponse{
		ClusterId: clusterID,
	}
}
