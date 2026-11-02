package ciTrusts

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server/grpcutil"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type ciTrustGRPCHandler struct {
	pb.UnimplementedCITrustServiceServer
	service    CITrustService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewCITrustGRPCHandler(svc CITrustService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.CITrustServiceServer {
	return &ciTrustGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *ciTrustGRPCHandler) GetCITrust(ctx context.Context, req *pb.GetCITrustRequest) (*pb.GetCITrustResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	item, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetCITrustResponse{CiTrust: ciTrustToProto(item)}, nil
}

func (h *ciTrustGRPCHandler) CreateCITrust(ctx context.Context, req *pb.CreateCITrustRequest) (*pb.CreateCITrustResponse, error) {
	if err := grpcutil.ValidateStringField("ci_provider_id", req.CiProviderId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("gateway_id", req.GatewayId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("role", req.Role, true); err != nil {
		return nil, err
	}
	item := &CITrust{
		CIProviderID:        req.CiProviderId,
		GatewayID:           req.GatewayId,
		SubjectRepository:   req.SubjectRepository,
		SubjectProjectPath:  req.SubjectProjectPath,
		SubjectRef:          req.SubjectRef,
		SubjectEnvironment:  req.SubjectEnvironment,
		SubjectRefProtected: req.SubjectRefProtected,
		Role:                req.Role,
	}
	created, svcErr := h.service.Create(ctx, item)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateCITrustResponse{CiTrust: ciTrustToProto(created)}, nil
}

func (h *ciTrustGRPCHandler) UpdateCITrust(ctx context.Context, req *pb.UpdateCITrustRequest) (*pb.UpdateCITrustResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	found, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if req.SubjectRef != nil {
		found.SubjectRef = req.SubjectRef
	}
	if req.SubjectEnvironment != nil {
		found.SubjectEnvironment = req.SubjectEnvironment
	}
	if req.SubjectRefProtected != nil {
		found.SubjectRefProtected = req.SubjectRefProtected
	}
	if req.Role != nil {
		found.Role = *req.Role
	}
	if req.StatusExchangeEndpoint != nil {
		found.StatusExchangeEndpoint = req.StatusExchangeEndpoint
	}
	if req.StatusWorkspace != nil {
		found.StatusWorkspace = req.StatusWorkspace
	}
	if req.StatusWorkflowHint != nil {
		found.StatusWorkflowHint = req.StatusWorkflowHint
	}
	updated, svcErr := h.service.Replace(ctx, found)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateCITrustResponse{CiTrust: ciTrustToProto(updated)}, nil
}

func (h *ciTrustGRPCHandler) DeleteCITrust(ctx context.Context, req *pb.DeleteCITrustRequest) (*pb.DeleteCITrustResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	if svcErr := h.service.Delete(ctx, req.Id); svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteCITrustResponse{}, nil
}

func (h *ciTrustGRPCHandler) ListCITrusts(ctx context.Context, req *pb.ListCITrustsRequest) (*pb.ListCITrustsResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)
	listArgs := &services.ListArguments{Page: int(page), Size: int64(size)}

	var items []CITrust
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &items)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	out := make([]*pb.CITrust, len(items))
	for i := range items {
		out[i] = ciTrustToProto(&items[i])
	}
	return &pb.ListCITrustsResponse{
		Items:    out,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *ciTrustGRPCHandler) WatchCITrusts(_ *pb.WatchCITrustsRequest, stream grpc.ServerStreamingServer[pb.WatchCITrustsResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	slog.DebugContext(ctx, "WatchCITrusts subscriber connected", "subscriber_id", sub.ID)

	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			slog.DebugContext(ctx, "WatchCITrusts subscriber disconnected", "subscriber_id", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if evt.Source != ciTrustEventSource {
				continue
			}

			watchEvent := &pb.WatchCITrustsResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}
			if evt.EventType != api.DeleteEventType {
				item, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					slog.WarnContext(ctx, "WatchCITrusts failed to load CITrust", "resource_id", evt.SourceID, "error", svcErr)
					continue
				}
				watchEvent.CiTrust = ciTrustToProto(item)
			}
			if err := stream.Send(watchEvent); err != nil {
				slog.DebugContext(ctx, "WatchCITrusts send failed", "subscriber_id", sub.ID, "error", err)
				return err
			}
		}
	}
}
