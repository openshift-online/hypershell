package gatewayProfiles

import (
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func gatewayProfileToProto(d *GatewayProfile) *pb.GatewayProfile {
	return &pb.GatewayProfile{
		Metadata: &pb.ObjectReference{
			Id:        d.ID,
			CreatedAt: timestamppb.New(d.CreatedAt),
			UpdatedAt: timestamppb.New(d.UpdatedAt),
			Kind:      "GatewayProfile",
			Href:      "/api/hypershell/v1/gateway_profiles/" + d.ID,
		},
		Name:                          d.Name,
		Description:                   d.Description,
		CpuRequestTotal:               d.CpuRequestTotal,
		CpuLimitTotal:                 d.CpuLimitTotal,
		MemoryRequestTotal:            d.MemoryRequestTotal,
		MemoryLimitTotal:              d.MemoryLimitTotal,
		EphemeralStorageTotal:         d.EphemeralStorageTotal,
		PodCount:                      d.PodCount,
		PvcCount:                      d.PvcCount,
		ContainerCpuRequestDefault:    d.ContainerCpuRequestDefault,
		ContainerCpuLimitMax:          d.ContainerCpuLimitMax,
		ContainerMemoryRequestDefault: d.ContainerMemoryRequestDefault,
		ContainerMemoryLimitMax:       d.ContainerMemoryLimitMax,
	}
}
