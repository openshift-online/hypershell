package inferenceRoutes

import (
	"context"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	pkgrbac "github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server/grpcutil"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type inferenceRouteGRPCHandler struct {
	pb.UnimplementedInferenceRouteServiceServer
	service    InferenceRouteService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewInferenceRouteGRPCHandler(svc InferenceRouteService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.InferenceRouteServiceServer {
	return &inferenceRouteGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *inferenceRouteGRPCHandler) GetInferenceRoute(ctx context.Context, req *pb.GetInferenceRouteRequest) (*pb.GetInferenceRouteResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	item, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetInferenceRouteResponse{InferenceRoute: inferenceRouteToProto(item)}, nil
}

func (h *inferenceRouteGRPCHandler) CreateInferenceRoute(ctx context.Context, req *pb.CreateInferenceRouteRequest) (*pb.CreateInferenceRouteResponse, error) {
	if err := grpcutil.ValidateStringField("workspace_id", req.WorkspaceId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("provider_binding_id", req.ProviderBindingId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("model", req.Model, true); err != nil {
		return nil, err
	}
	item := &InferenceRoute{
		WorkspaceId:       req.WorkspaceId,
		ProviderBindingId: req.ProviderBindingId,
		Model:             req.Model,
		RepositoryId:      pkgrbac.ImportOwnershipMark(ctx), // import ownership set server-side; a client-supplied repository_id is ignored
	}
	created, svcErr := h.service.Create(ctx, item)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateInferenceRouteResponse{InferenceRoute: inferenceRouteToProto(created)}, nil
}

func (h *inferenceRouteGRPCHandler) DeleteInferenceRoute(ctx context.Context, req *pb.DeleteInferenceRouteRequest) (*pb.DeleteInferenceRouteResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	found, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if found == nil {
		return nil, status.Errorf(codes.NotFound, "inference route %s not found", req.Id)
	}
	if err := pkgrbac.EnforceImportOwnership(ctx, found.RepositoryId); err != nil {
		return nil, err
	}
	if svcErr = h.service.Delete(ctx, req.Id); svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteInferenceRouteResponse{}, nil
}

func (h *inferenceRouteGRPCHandler) ListInferenceRoutes(ctx context.Context, req *pb.ListInferenceRoutesRequest) (*pb.ListInferenceRoutesResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)
	listArgs := &services.ListArguments{Page: int(page), Size: int64(size)}

	var items []InferenceRoute
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &items)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	out := make([]*pb.InferenceRoute, len(items))
	for i := range items {
		out[i] = inferenceRouteToProto(&items[i])
	}
	return &pb.ListInferenceRoutesResponse{
		Items:    out,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *inferenceRouteGRPCHandler) WatchInferenceRoutes(_ *pb.WatchInferenceRoutesRequest, stream grpc.ServerStreamingServer[pb.WatchInferenceRoutesResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	glog.V(4).Infof("WatchInferenceRoutes: subscriber %s connected", sub.ID)

	// Flush the header once the subscription is live so a client can seed its
	// state with a LIST without a list-watch gap (see gateways WatchGateways).
	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			glog.V(4).Infof("WatchInferenceRoutes: subscriber %s disconnected", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if evt.Source != inferenceRouteEventSource {
				continue
			}

			watchEvent := &pb.WatchInferenceRoutesResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}
			// A delete event carries only the id: the row is soft-deleted and the
			// import reconciler needs just the id to prune. A create loads and
			// sends the current object.
			if evt.EventType != api.DeleteEventType {
				item, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					glog.Warningf("WatchInferenceRoutes: failed to load inference route %s: %v", evt.SourceID, svcErr)
					continue
				}
				watchEvent.InferenceRoute = inferenceRouteToProto(item)
			}
			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchInferenceRoutes: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}
