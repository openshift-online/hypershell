package ciProviders

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

type ciProviderGRPCHandler struct {
	pb.UnimplementedCIProviderServiceServer
	service    CIProviderService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewCIProviderGRPCHandler(svc CIProviderService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.CIProviderServiceServer {
	return &ciProviderGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *ciProviderGRPCHandler) GetCIProvider(ctx context.Context, req *pb.GetCIProviderRequest) (*pb.GetCIProviderResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	item, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetCIProviderResponse{CiProvider: ciProviderToProto(item)}, nil
}

func (h *ciProviderGRPCHandler) CreateCIProvider(ctx context.Context, req *pb.CreateCIProviderRequest) (*pb.CreateCIProviderResponse, error) {
	if err := grpcutil.ValidateStringField("forge", req.Forge, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("issuer", req.Issuer, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("audience", req.Audience, true); err != nil {
		return nil, err
	}
	item := &CIProvider{
		Forge:    req.Forge,
		Issuer:   req.Issuer,
		JwksUrl:  req.JwksUrl,
		Audience: req.Audience,
	}
	created, svcErr := h.service.Create(ctx, item)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateCIProviderResponse{CiProvider: ciProviderToProto(created)}, nil
}

func (h *ciProviderGRPCHandler) UpdateCIProvider(ctx context.Context, req *pb.UpdateCIProviderRequest) (*pb.UpdateCIProviderResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	found, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if req.JwksUrl != nil {
		found.JwksUrl = *req.JwksUrl
	}
	if req.StatusIdpAlias != nil {
		found.StatusIdpAlias = req.StatusIdpAlias
	}
	updated, svcErr := h.service.Replace(ctx, found)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateCIProviderResponse{CiProvider: ciProviderToProto(updated)}, nil
}

func (h *ciProviderGRPCHandler) DeleteCIProvider(ctx context.Context, req *pb.DeleteCIProviderRequest) (*pb.DeleteCIProviderResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	if svcErr := h.service.Delete(ctx, req.Id); svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteCIProviderResponse{}, nil
}

func (h *ciProviderGRPCHandler) ListCIProviders(ctx context.Context, req *pb.ListCIProvidersRequest) (*pb.ListCIProvidersResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)
	listArgs := &services.ListArguments{Page: int(page), Size: int64(size)}

	var items []CIProvider
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &items)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	out := make([]*pb.CIProvider, len(items))
	for i := range items {
		out[i] = ciProviderToProto(&items[i])
	}
	return &pb.ListCIProvidersResponse{
		Items:    out,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *ciProviderGRPCHandler) WatchCIProviders(_ *pb.WatchCIProvidersRequest, stream grpc.ServerStreamingServer[pb.WatchCIProvidersResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	slog.DebugContext(ctx, "WatchCIProviders subscriber connected", "subscriber_id", sub.ID)

	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			slog.DebugContext(ctx, "WatchCIProviders subscriber disconnected", "subscriber_id", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if evt.Source != ciProviderEventSource {
				continue
			}

			watchEvent := &pb.WatchCIProvidersResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}
			if evt.EventType != api.DeleteEventType {
				item, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					slog.WarnContext(ctx, "WatchCIProviders failed to load CIProvider", "resource_id", evt.SourceID, "error", svcErr)
					continue
				}
				watchEvent.CiProvider = ciProviderToProto(item)
			}
			if err := stream.Send(watchEvent); err != nil {
				slog.DebugContext(ctx, "WatchCIProviders send failed", "subscriber_id", sub.ID, "error", err)
				return err
			}
		}
	}
}
