package gatewayProfiles

import (
	"context"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/pkg/server/grpcutil"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

type gatewayProfileGRPCHandler struct {
	pb.UnimplementedGatewayProfileServiceServer
	service GatewayProfileService
	generic services.GenericService
}

func NewGatewayProfileGRPCHandler(svc GatewayProfileService, generic services.GenericService) pb.GatewayProfileServiceServer {
	return &gatewayProfileGRPCHandler{service: svc, generic: generic}
}

func (h *gatewayProfileGRPCHandler) GetGatewayProfile(ctx context.Context, req *pb.GetGatewayProfileRequest) (*pb.GetGatewayProfileResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	gatewayProfile, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetGatewayProfileResponse{GatewayProfile: gatewayProfileToProto(gatewayProfile)}, nil
}

func (h *gatewayProfileGRPCHandler) CreateGatewayProfile(ctx context.Context, req *pb.CreateGatewayProfileRequest) (*pb.CreateGatewayProfileResponse, error) {
	if err := grpcutil.ValidateStringField("name", req.Name, true); err != nil {
		return nil, err
	}

	gatewayProfile := &GatewayProfile{
		Name:                          req.Name,
		Description:                   req.Description,
		CpuRequestTotal:               req.CpuRequestTotal,
		CpuLimitTotal:                 req.CpuLimitTotal,
		MemoryRequestTotal:            req.MemoryRequestTotal,
		MemoryLimitTotal:              req.MemoryLimitTotal,
		EphemeralStorageTotal:         req.EphemeralStorageTotal,
		PodCount:                      req.PodCount,
		PvcCount:                      req.PvcCount,
		ContainerCpuRequestDefault:    req.ContainerCpuRequestDefault,
		ContainerCpuLimitMax:          req.ContainerCpuLimitMax,
		ContainerMemoryRequestDefault: req.ContainerMemoryRequestDefault,
		ContainerMemoryLimitMax:       req.ContainerMemoryLimitMax,
	}
	result, svcErr := h.service.Create(ctx, gatewayProfile)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateGatewayProfileResponse{GatewayProfile: gatewayProfileToProto(result)}, nil
}

func (h *gatewayProfileGRPCHandler) UpdateGatewayProfile(ctx context.Context, req *pb.UpdateGatewayProfileRequest) (*pb.UpdateGatewayProfileResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	if req.Name != nil {
		if err := grpcutil.ValidateStringField("name", *req.Name, false); err != nil {
			return nil, err
		}
	}

	gatewayProfile, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if req.Name != nil {
		gatewayProfile.Name = *req.Name
	}
	if req.Description != nil {
		gatewayProfile.Description = req.Description
	}
	if req.CpuRequestTotal != nil {
		gatewayProfile.CpuRequestTotal = req.CpuRequestTotal
	}
	if req.CpuLimitTotal != nil {
		gatewayProfile.CpuLimitTotal = req.CpuLimitTotal
	}
	if req.MemoryRequestTotal != nil {
		gatewayProfile.MemoryRequestTotal = req.MemoryRequestTotal
	}
	if req.MemoryLimitTotal != nil {
		gatewayProfile.MemoryLimitTotal = req.MemoryLimitTotal
	}
	if req.EphemeralStorageTotal != nil {
		gatewayProfile.EphemeralStorageTotal = req.EphemeralStorageTotal
	}
	if req.PodCount != nil {
		gatewayProfile.PodCount = req.PodCount
	}
	if req.PvcCount != nil {
		gatewayProfile.PvcCount = req.PvcCount
	}
	if req.ContainerCpuRequestDefault != nil {
		gatewayProfile.ContainerCpuRequestDefault = req.ContainerCpuRequestDefault
	}
	if req.ContainerCpuLimitMax != nil {
		gatewayProfile.ContainerCpuLimitMax = req.ContainerCpuLimitMax
	}
	if req.ContainerMemoryRequestDefault != nil {
		gatewayProfile.ContainerMemoryRequestDefault = req.ContainerMemoryRequestDefault
	}
	if req.ContainerMemoryLimitMax != nil {
		gatewayProfile.ContainerMemoryLimitMax = req.ContainerMemoryLimitMax
	}
	result, svcErr := h.service.Replace(ctx, gatewayProfile)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateGatewayProfileResponse{GatewayProfile: gatewayProfileToProto(result)}, nil
}

func (h *gatewayProfileGRPCHandler) DeleteGatewayProfile(ctx context.Context, req *pb.DeleteGatewayProfileRequest) (*pb.DeleteGatewayProfileResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	svcErr := h.service.Delete(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteGatewayProfileResponse{}, nil
}

func (h *gatewayProfileGRPCHandler) ListGatewayProfiles(ctx context.Context, req *pb.ListGatewayProfilesRequest) (*pb.ListGatewayProfilesResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)

	listArgs := &services.ListArguments{
		Page: int(page),
		Size: int64(size),
	}

	var gatewayProfiles []GatewayProfile
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &gatewayProfiles)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	items := make([]*pb.GatewayProfile, len(gatewayProfiles))
	for i, d := range gatewayProfiles {
		items[i] = gatewayProfileToProto(&d)
	}

	return &pb.ListGatewayProfilesResponse{
		Items:    items,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}
