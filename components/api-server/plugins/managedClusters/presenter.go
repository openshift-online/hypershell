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
	result := openapi.ManagedCluster{
		Id:               reference.Id,
		Kind:             reference.Kind,
		Href:             reference.Href,
		CreatedAt:        openapi.PtrTime(managedCluster.CreatedAt),
		UpdatedAt:        openapi.PtrTime(managedCluster.UpdatedAt),
		Name:             managedCluster.Name,
		KubeconfigSecret: managedCluster.KubeconfigSecret,
		Status:           managedCluster.Status,
		ApiServerUrl:     managedCluster.ApiServerUrl,
	}
	if managedCluster.OIDCSubject != "" {
		result.OidcSubject = &managedCluster.OIDCSubject
	}
	result.LastSeenAt = managedCluster.LastSeenAt
	result.Capabilities = append([]string(nil), managedCluster.Capabilities...)
	result.Health = PresentHealthVerdict(managedCluster.Health)
	return result
}

// ConvertHealthVerdict maps the optional OpenAPI health object from a
// registration request into the stored domain verdict. A nil input (absent or
// null `health`) stays nil, so a health-less heartbeat is a no-op for the stored
// verdict (PLAT-CLUS-16.02).
func ConvertHealthVerdict(h *openapi.ManagedClusterHealth) *HealthVerdict {
	if h == nil {
		return nil
	}
	verdict := &HealthVerdict{
		Overall:    h.Overall,
		ObservedAt: h.ObservedAt,
	}
	if h.DurationMs != nil {
		verdict.DurationMs = *h.DurationMs
	}
	verdict.Conditions = make([]HealthCondition, 0, len(h.Conditions))
	for i := range h.Conditions {
		c := h.Conditions[i]
		condition := HealthCondition{
			Name:             c.Name,
			Status:           c.Status,
			LastTransitionAt: c.LastTransitionAt,
		}
		if c.Message != nil {
			condition.Message = *c.Message
		}
		verdict.Conditions = append(verdict.Conditions, condition)
	}
	return verdict
}

// PresentHealthVerdict maps the stored domain verdict onto the OpenAPI read
// model. A nil verdict (never reported) presents as null (PLAT-DATA-27.02).
func PresentHealthVerdict(v *HealthVerdict) *openapi.ManagedClusterHealth {
	if v == nil {
		return nil
	}
	health := &openapi.ManagedClusterHealth{
		Overall:    v.Overall,
		ObservedAt: v.ObservedAt,
		DurationMs: openapi.PtrInt64(v.DurationMs),
	}
	health.Conditions = make([]openapi.ManagedClusterHealthCondition, 0, len(v.Conditions))
	for i := range v.Conditions {
		c := v.Conditions[i]
		condition := openapi.ManagedClusterHealthCondition{
			Name:             c.Name,
			Status:           c.Status,
			LastTransitionAt: c.LastTransitionAt,
		}
		if c.Message != "" {
			condition.Message = openapi.PtrString(c.Message)
		}
		health.Conditions = append(health.Conditions, condition)
	}
	return health
}

func PresentRegistrationResponse(clusterID string) openapi.ManagedClusterRegistrationResponse {
	return openapi.ManagedClusterRegistrationResponse{
		ClusterId: clusterID,
	}
}
