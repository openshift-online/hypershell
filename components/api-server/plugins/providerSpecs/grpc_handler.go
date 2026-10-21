package providerSpecs

import (
	"context"
	"log/slog"

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

type providerSpecGRPCHandler struct {
	pb.UnimplementedProviderSpecServiceServer
	service    ProviderSpecService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewProviderSpecGRPCHandler(svc ProviderSpecService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.ProviderSpecServiceServer {
	return &providerSpecGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *providerSpecGRPCHandler) GetProviderSpec(ctx context.Context, req *pb.GetProviderSpecRequest) (*pb.GetProviderSpecResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	item, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetProviderSpecResponse{ProviderSpec: providerSpecToProto(item)}, nil
}

func (h *providerSpecGRPCHandler) CreateProviderSpec(ctx context.Context, req *pb.CreateProviderSpecRequest) (*pb.CreateProviderSpecResponse, error) {
	if err := grpcutil.ValidateStringField("name", req.Name, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("category", req.Category, true); err != nil {
		return nil, err
	}
	item := &ProviderSpec{
		Name:         req.Name,
		Category:     req.Category,
		Capability:   req.Capability,
		Profile:      req.Profile,
		RepositoryId: pkgrbac.ImportOwnershipMark(ctx), // import ownership set server-side; a client-supplied repository_id is ignored
	}
	created, svcErr := h.service.Create(ctx, item)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateProviderSpecResponse{ProviderSpec: providerSpecToProto(created)}, nil
}

func (h *providerSpecGRPCHandler) UpdateProviderSpec(ctx context.Context, req *pb.UpdateProviderSpecRequest) (*pb.UpdateProviderSpecResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	found, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if err := pkgrbac.EnforceImportOwnership(ctx, found.RepositoryId); err != nil {
		return nil, err
	}
	if req.Name != nil {
		found.Name = *req.Name
	}
	if req.Capability != nil {
		found.Capability = req.Capability
	}
	if req.Profile != nil {
		found.Profile = req.Profile
	}
	updated, svcErr := h.service.Replace(ctx, found)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateProviderSpecResponse{ProviderSpec: providerSpecToProto(updated)}, nil
}

func (h *providerSpecGRPCHandler) DeleteProviderSpec(ctx context.Context, req *pb.DeleteProviderSpecRequest) (*pb.DeleteProviderSpecResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	found, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if found == nil {
		return nil, status.Errorf(codes.NotFound, "provider spec %s not found", req.Id)
	}
	if err := pkgrbac.EnforceImportOwnership(ctx, found.RepositoryId); err != nil {
		return nil, err
	}
	if svcErr = h.service.Delete(ctx, req.Id); svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteProviderSpecResponse{}, nil
}

func (h *providerSpecGRPCHandler) ListProviderSpecs(ctx context.Context, req *pb.ListProviderSpecsRequest) (*pb.ListProviderSpecsResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)
	listArgs := &services.ListArguments{Page: int(page), Size: int64(size)}

	var items []ProviderSpec
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &items)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	out := make([]*pb.ProviderSpec, len(items))
	for i := range items {
		out[i] = providerSpecToProto(&items[i])
	}
	return &pb.ListProviderSpecsResponse{
		Items:    out,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *providerSpecGRPCHandler) WatchProviderSpecs(_ *pb.WatchProviderSpecsRequest, stream grpc.ServerStreamingServer[pb.WatchProviderSpecsResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	slog.DebugContext(ctx, "WatchProviderSpecs subscriber connected", "subscriber_id", sub.ID)

	// Flush the header once the subscription is live so a client can seed its
	// state with a LIST without a list-watch gap (see gateways WatchGateways).
	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			slog.DebugContext(ctx, "WatchProviderSpecs subscriber disconnected", "subscriber_id", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if evt.Source != providerSpecEventSource {
				continue
			}

			watchEvent := &pb.WatchProviderSpecsResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}
			// A delete event carries only the id: the row is soft-deleted and the
			// import reconciler needs just the id to prune. A create/update loads
			// and sends the current object.
			if evt.EventType != api.DeleteEventType {
				item, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					slog.WarnContext(ctx, "WatchProviderSpecs failed to load provider spec", "resource_id", evt.SourceID, "error", svcErr)
					continue
				}
				watchEvent.ProviderSpec = providerSpecToProto(item)
			}
			if err := stream.Send(watchEvent); err != nil {
				slog.DebugContext(ctx, "WatchProviderSpecs send failed", "subscriber_id", sub.ID, "error", err)
				return err
			}
		}
	}
}
