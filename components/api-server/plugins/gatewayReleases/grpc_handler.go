package gatewayReleases

import (
	"context"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
	"github.com/openshift-online/rh-trex-ai/pkg/server/grpcutil"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

type gatewayReleaseGRPCHandler struct {
	pb.UnimplementedGatewayReleaseServiceServer
	service    GatewayReleaseService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewGatewayReleaseGRPCHandler(svc GatewayReleaseService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.GatewayReleaseServiceServer {
	return &gatewayReleaseGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *gatewayReleaseGRPCHandler) GetGatewayRelease(ctx context.Context, req *pb.GetGatewayReleaseRequest) (*pb.GetGatewayReleaseResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	gatewayRelease, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetGatewayReleaseResponse{GatewayRelease: gatewayReleaseToProto(gatewayRelease)}, nil
}

func (h *gatewayReleaseGRPCHandler) CreateGatewayRelease(ctx context.Context, req *pb.CreateGatewayReleaseRequest) (*pb.CreateGatewayReleaseResponse, error) {
	if err := grpcutil.ValidateStringField("name", req.Name, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("image", req.Image, true); err != nil {
		return nil, err
	}

	gatewayRelease := &GatewayRelease{
		Name:            req.Name,
		Image:           req.Image,
		RolloutStrategy: req.RolloutStrategy,
		CanaryPercent: func() *int {
			if req.CanaryPercent != nil {
				v := int(*req.CanaryPercent)
				return &v
			}
			return nil
		}(),
		CanaryDuration: req.CanaryDuration,
		Status:         req.Status,
	}
	result, svcErr := h.service.Create(ctx, gatewayRelease)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateGatewayReleaseResponse{GatewayRelease: gatewayReleaseToProto(result)}, nil
}

func (h *gatewayReleaseGRPCHandler) UpdateGatewayRelease(ctx context.Context, req *pb.UpdateGatewayReleaseRequest) (*pb.UpdateGatewayReleaseResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	if req.Name != nil {
		if err := grpcutil.ValidateStringField("name", *req.Name, false); err != nil {
			return nil, err
		}
	}
	if req.Image != nil {
		if err := grpcutil.ValidateStringField("image", *req.Image, false); err != nil {
			return nil, err
		}
	}
	if req.RolloutStrategy != nil {
		if err := grpcutil.ValidateStringField("rollout_strategy", *req.RolloutStrategy, false); err != nil {
			return nil, err
		}
	}
	if req.CanaryDuration != nil {
		if err := grpcutil.ValidateStringField("canary_duration", *req.CanaryDuration, false); err != nil {
			return nil, err
		}
	}
	if req.Status != nil {
		if err := grpcutil.ValidateStringField("status", *req.Status, false); err != nil {
			return nil, err
		}
	}

	gatewayRelease, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if req.Name != nil {
		gatewayRelease.Name = *req.Name
	}
	if req.Image != nil {
		gatewayRelease.Image = *req.Image
	}
	if req.RolloutStrategy != nil {
		gatewayRelease.RolloutStrategy = req.RolloutStrategy
	}
	if req.CanaryPercent != nil {
		gatewayRelease.CanaryPercent = func() *int { v := int(*req.CanaryPercent); return &v }()
	}
	if req.CanaryDuration != nil {
		gatewayRelease.CanaryDuration = req.CanaryDuration
	}
	if req.Status != nil {
		gatewayRelease.Status = req.Status
	}
	result, svcErr := h.service.Replace(ctx, gatewayRelease)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateGatewayReleaseResponse{GatewayRelease: gatewayReleaseToProto(result)}, nil
}

func (h *gatewayReleaseGRPCHandler) DeleteGatewayRelease(ctx context.Context, req *pb.DeleteGatewayReleaseRequest) (*pb.DeleteGatewayReleaseResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	svcErr := h.service.Delete(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteGatewayReleaseResponse{}, nil
}

func (h *gatewayReleaseGRPCHandler) ListGatewayReleases(ctx context.Context, req *pb.ListGatewayReleasesRequest) (*pb.ListGatewayReleasesResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)

	listArgs := &services.ListArguments{
		Page: int(page),
		Size: int64(size),
	}

	var gatewayReleases []GatewayRelease
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &gatewayReleases)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	items := make([]*pb.GatewayRelease, len(gatewayReleases))
	for i, d := range gatewayReleases {
		items[i] = gatewayReleaseToProto(&d)
	}

	return &pb.ListGatewayReleasesResponse{
		Items:    items,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *gatewayReleaseGRPCHandler) WatchGatewayReleases(req *pb.WatchGatewayReleasesRequest, stream grpc.ServerStreamingServer[pb.WatchGatewayReleasesResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	glog.V(4).Infof("WatchGatewayReleases: subscriber %s connected", sub.ID)

	for {
		select {
		case <-ctx.Done():
			glog.V(4).Infof("WatchGatewayReleases: subscriber %s disconnected", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}

			if evt.Source != "GatewayReleases" {
				continue
			}

			watchEvent := &pb.WatchGatewayReleasesResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}

			if evt.EventType != api.DeleteEventType {
				gatewayRelease, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					glog.Warningf("WatchGatewayReleases: failed to load gatewayRelease %s: %v", evt.SourceID, svcErr)
					continue
				}
				watchEvent.GatewayRelease = gatewayReleaseToProto(gatewayRelease)
			}

			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchGatewayReleases: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}
