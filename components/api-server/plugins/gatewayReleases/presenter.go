package gatewayReleases

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/util"
)

func ConvertGatewayRelease(gatewayRelease openapi.GatewayRelease) *GatewayRelease {
	c := &GatewayRelease{
		Meta: api.Meta{
			ID: util.NilToEmptyString(gatewayRelease.Id),
		},
	}
	c.Name = gatewayRelease.Name
	c.Image = gatewayRelease.Image
	c.RolloutStrategy = gatewayRelease.RolloutStrategy
	if gatewayRelease.CanaryPercent != nil {
		c.CanaryPercent = openapi.PtrInt(int(*gatewayRelease.CanaryPercent))
	}
	c.CanaryDuration = gatewayRelease.CanaryDuration
	c.Status = gatewayRelease.Status

	if gatewayRelease.CreatedAt != nil {
		c.CreatedAt = *gatewayRelease.CreatedAt
		c.UpdatedAt = *gatewayRelease.UpdatedAt
	}

	return c
}

func PresentGatewayRelease(gatewayRelease *GatewayRelease) openapi.GatewayRelease {
	reference := presenters.PresentReference(gatewayRelease.ID, gatewayRelease)
	return openapi.GatewayRelease{
		Id:              reference.Id,
		Kind:            reference.Kind,
		Href:            reference.Href,
		CreatedAt:       openapi.PtrTime(gatewayRelease.CreatedAt),
		UpdatedAt:       openapi.PtrTime(gatewayRelease.UpdatedAt),
		Name:            gatewayRelease.Name,
		Image:           gatewayRelease.Image,
		RolloutStrategy: gatewayRelease.RolloutStrategy,
		CanaryPercent: func() *int32 {
			if gatewayRelease.CanaryPercent != nil {
				return openapi.PtrInt32(int32(*gatewayRelease.CanaryPercent))
			}
			return nil
		}(),
		CanaryDuration: gatewayRelease.CanaryDuration,
		Status:         gatewayRelease.Status,
	}
}
