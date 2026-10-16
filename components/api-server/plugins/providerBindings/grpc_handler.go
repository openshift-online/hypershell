package providerBindings

import (
	"context"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server/grpcutil"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type providerBindingGRPCHandler struct {
	pb.UnimplementedProviderBindingServiceServer
	service    ProviderBindingService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewProviderBindingGRPCHandler(svc ProviderBindingService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.ProviderBindingServiceServer {
	return &providerBindingGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *providerBindingGRPCHandler) GetProviderBinding(ctx context.Context, req *pb.GetProviderBindingRequest) (*pb.GetProviderBindingResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	item, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetProviderBindingResponse{ProviderBinding: providerBindingToProto(item)}, nil
}

func (h *providerBindingGRPCHandler) CreateProviderBinding(ctx context.Context, req *pb.CreateProviderBindingRequest) (*pb.CreateProviderBindingResponse, error) {
	if err := grpcutil.ValidateStringField("name", req.Name, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("workspace_id", req.WorkspaceId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("provider_spec_id", req.ProviderSpecId, true); err != nil {
		return nil, err
	}
	item := &ProviderBinding{
		Name:            req.Name,
		WorkspaceId:     req.WorkspaceId,
		ProviderSpecId:  req.ProviderSpecId,
		RefreshStrategy: req.RefreshStrategy,
		RepositoryId:    req.RepositoryId,
	}
	if req.SecretSourceId != nil {
		item.SecretSourceId = *req.SecretSourceId
	}
	created, svcErr := h.service.Create(ctx, item)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateProviderBindingResponse{ProviderBinding: providerBindingToProto(created)}, nil
}

func (h *providerBindingGRPCHandler) UpdateProviderBinding(ctx context.Context, req *pb.UpdateProviderBindingRequest) (*pb.UpdateProviderBindingResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	found, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if req.Name != nil {
		found.Name = *req.Name
	}
	if req.SecretSourceId != nil {
		found.SecretSourceId = *req.SecretSourceId
	}
	if req.RefreshStrategy != nil {
		found.RefreshStrategy = req.RefreshStrategy
	}
	updated, svcErr := h.service.Replace(ctx, found)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateProviderBindingResponse{ProviderBinding: providerBindingToProto(updated)}, nil
}

func (h *providerBindingGRPCHandler) DeleteProviderBinding(ctx context.Context, req *pb.DeleteProviderBindingRequest) (*pb.DeleteProviderBindingResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	if svcErr := h.service.Delete(ctx, req.Id); svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteProviderBindingResponse{}, nil
}

func (h *providerBindingGRPCHandler) ListProviderBindings(ctx context.Context, req *pb.ListProviderBindingsRequest) (*pb.ListProviderBindingsResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)
	listArgs := &services.ListArguments{Page: int(page), Size: int64(size)}

	var items []ProviderBinding
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &items)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	out := make([]*pb.ProviderBinding, len(items))
	for i := range items {
		out[i] = providerBindingToProto(&items[i])
	}
	return &pb.ListProviderBindingsResponse{
		Items:    out,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *providerBindingGRPCHandler) WatchProviderBindings(_ *pb.WatchProviderBindingsRequest, stream grpc.ServerStreamingServer[pb.WatchProviderBindingsResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	glog.V(4).Infof("WatchProviderBindings: subscriber %s connected", sub.ID)

	// Flush the header once the subscription is live so a client can seed its
	// state with a LIST without a list-watch gap (see gateways WatchGateways).
	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			glog.V(4).Infof("WatchProviderBindings: subscriber %s disconnected", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if evt.Source != providerBindingEventSource {
				continue
			}

			watchEvent := &pb.WatchProviderBindingsResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}
			// A delete event carries only the id: the row is soft-deleted and the
			// import reconciler needs just the id to prune. A create/update loads
			// and sends the current object.
			if evt.EventType != api.DeleteEventType {
				item, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					glog.Warningf("WatchProviderBindings: failed to load provider binding %s: %v", evt.SourceID, svcErr)
					continue
				}
				watchEvent.ProviderBinding = providerBindingToProto(item)
			}
			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchProviderBindings: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}
