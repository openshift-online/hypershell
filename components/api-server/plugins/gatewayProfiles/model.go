package gatewayProfiles

import (
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"gorm.io/gorm"
)

type GatewayProfile struct {
	api.Meta
	Name                          string  `json:"name"`
	Description                   *string `json:"description"`
	CpuRequestTotal               *string `json:"cpu_request_total"`
	CpuLimitTotal                 *string `json:"cpu_limit_total"`
	MemoryRequestTotal            *string `json:"memory_request_total"`
	MemoryLimitTotal              *string `json:"memory_limit_total"`
	EphemeralStorageTotal         *string `json:"ephemeral_storage_total"`
	PodCount                      *int32  `json:"pod_count"`
	PvcCount                      *int32  `json:"pvc_count"`
	ContainerCpuRequestDefault    *string `json:"container_cpu_request_default"`
	ContainerCpuLimitMax          *string `json:"container_cpu_limit_max"`
	ContainerMemoryRequestDefault *string `json:"container_memory_request_default"`
	ContainerMemoryLimitMax       *string `json:"container_memory_limit_max"`
}

type GatewayProfileList []*GatewayProfile
type GatewayProfileIndex map[string]*GatewayProfile

func (l GatewayProfileList) Index() GatewayProfileIndex {
	index := GatewayProfileIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (d *GatewayProfile) BeforeCreate(tx *gorm.DB) error {
	d.ID = api.NewID()
	return nil
}

type GatewayProfilePatchRequest struct {
	Name                          *string `json:"name,omitempty"`
	Description                   *string `json:"description,omitempty"`
	CpuRequestTotal               *string `json:"cpu_request_total,omitempty"`
	CpuLimitTotal                 *string `json:"cpu_limit_total,omitempty"`
	MemoryRequestTotal            *string `json:"memory_request_total,omitempty"`
	MemoryLimitTotal              *string `json:"memory_limit_total,omitempty"`
	EphemeralStorageTotal         *string `json:"ephemeral_storage_total,omitempty"`
	PodCount                      *int32  `json:"pod_count,omitempty"`
	PvcCount                      *int32  `json:"pvc_count,omitempty"`
	ContainerCpuRequestDefault    *string `json:"container_cpu_request_default,omitempty"`
	ContainerCpuLimitMax          *string `json:"container_cpu_limit_max,omitempty"`
	ContainerMemoryRequestDefault *string `json:"container_memory_request_default,omitempty"`
	ContainerMemoryLimitMax       *string `json:"container_memory_limit_max,omitempty"`
}
