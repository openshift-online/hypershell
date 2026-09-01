package gatewayProfiles

import (
	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/api/presenters"
	"github.com/openshift-online/rh-trex-ai/pkg/util"
)

func ConvertGatewayProfile(gatewayProfile openapi.GatewayProfile) *GatewayProfile {
	c := &GatewayProfile{
		Meta: api.Meta{
			ID: util.NilToEmptyString(gatewayProfile.Id),
		},
	}
	c.Name = gatewayProfile.Name
	c.Description = gatewayProfile.Description
	c.CpuRequestTotal = gatewayProfile.CpuRequestTotal
	c.CpuLimitTotal = gatewayProfile.CpuLimitTotal
	c.MemoryRequestTotal = gatewayProfile.MemoryRequestTotal
	c.MemoryLimitTotal = gatewayProfile.MemoryLimitTotal
	c.EphemeralStorageTotal = gatewayProfile.EphemeralStorageTotal
	c.PodCount = gatewayProfile.PodCount
	c.PvcCount = gatewayProfile.PvcCount
	c.ContainerCpuRequestDefault = gatewayProfile.ContainerCpuRequestDefault
	c.ContainerCpuLimitMax = gatewayProfile.ContainerCpuLimitMax
	c.ContainerMemoryRequestDefault = gatewayProfile.ContainerMemoryRequestDefault
	c.ContainerMemoryLimitMax = gatewayProfile.ContainerMemoryLimitMax

	if gatewayProfile.CreatedAt != nil {
		c.CreatedAt = *gatewayProfile.CreatedAt
		c.UpdatedAt = *gatewayProfile.UpdatedAt
	}

	return c
}

func PresentGatewayProfile(gatewayProfile *GatewayProfile) openapi.GatewayProfile {
	reference := presenters.PresentReference(gatewayProfile.ID, gatewayProfile)
	return openapi.GatewayProfile{
		Id:                            reference.Id,
		Kind:                          reference.Kind,
		Href:                          reference.Href,
		CreatedAt:                     openapi.PtrTime(gatewayProfile.CreatedAt),
		UpdatedAt:                     openapi.PtrTime(gatewayProfile.UpdatedAt),
		Name:                          gatewayProfile.Name,
		Description:                   gatewayProfile.Description,
		CpuRequestTotal:               gatewayProfile.CpuRequestTotal,
		CpuLimitTotal:                 gatewayProfile.CpuLimitTotal,
		MemoryRequestTotal:            gatewayProfile.MemoryRequestTotal,
		MemoryLimitTotal:              gatewayProfile.MemoryLimitTotal,
		EphemeralStorageTotal:         gatewayProfile.EphemeralStorageTotal,
		PodCount:                      gatewayProfile.PodCount,
		PvcCount:                      gatewayProfile.PvcCount,
		ContainerCpuRequestDefault:    gatewayProfile.ContainerCpuRequestDefault,
		ContainerCpuLimitMax:          gatewayProfile.ContainerCpuLimitMax,
		ContainerMemoryRequestDefault: gatewayProfile.ContainerMemoryRequestDefault,
		ContainerMemoryLimitMax:       gatewayProfile.ContainerMemoryLimitMax,
	}
}
