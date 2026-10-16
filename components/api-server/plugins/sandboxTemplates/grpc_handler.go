package sandboxTemplates

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

type sandboxTemplateGRPCHandler struct {
	pb.UnimplementedSandboxTemplateServiceServer
	service    SandboxTemplateService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewSandboxTemplateGRPCHandler(svc SandboxTemplateService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.SandboxTemplateServiceServer {
	return &sandboxTemplateGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *sandboxTemplateGRPCHandler) GetSandboxTemplate(ctx context.Context, req *pb.GetSandboxTemplateRequest) (*pb.GetSandboxTemplateResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	item, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetSandboxTemplateResponse{SandboxTemplate: sandboxTemplateToProto(item)}, nil
}

func (h *sandboxTemplateGRPCHandler) CreateSandboxTemplate(ctx context.Context, req *pb.CreateSandboxTemplateRequest) (*pb.CreateSandboxTemplateResponse, error) {
	if err := grpcutil.ValidateStringField("name", req.Name, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("image", req.Image, true); err != nil {
		return nil, err
	}
	item := &SandboxTemplate{
		Name:         req.Name,
		Image:        req.Image,
		NamePrefix:   req.NamePrefix,
		Policy:       req.Policy,
		RepositoryId: req.RepositoryId,
	}
	created, svcErr := h.service.Create(ctx, item)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateSandboxTemplateResponse{SandboxTemplate: sandboxTemplateToProto(created)}, nil
}

func (h *sandboxTemplateGRPCHandler) UpdateSandboxTemplate(ctx context.Context, req *pb.UpdateSandboxTemplateRequest) (*pb.UpdateSandboxTemplateResponse, error) {
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
	if req.Image != nil {
		found.Image = *req.Image
	}
	if req.NamePrefix != nil {
		found.NamePrefix = req.NamePrefix
	}
	if req.Policy != nil {
		found.Policy = req.Policy
	}
	updated, svcErr := h.service.Replace(ctx, found)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateSandboxTemplateResponse{SandboxTemplate: sandboxTemplateToProto(updated)}, nil
}

func (h *sandboxTemplateGRPCHandler) DeleteSandboxTemplate(ctx context.Context, req *pb.DeleteSandboxTemplateRequest) (*pb.DeleteSandboxTemplateResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	if svcErr := h.service.Delete(ctx, req.Id); svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteSandboxTemplateResponse{}, nil
}

func (h *sandboxTemplateGRPCHandler) ListSandboxTemplates(ctx context.Context, req *pb.ListSandboxTemplatesRequest) (*pb.ListSandboxTemplatesResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)
	listArgs := &services.ListArguments{Page: int(page), Size: int64(size)}

	var items []SandboxTemplate
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &items)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	out := make([]*pb.SandboxTemplate, len(items))
	for i := range items {
		out[i] = sandboxTemplateToProto(&items[i])
	}
	return &pb.ListSandboxTemplatesResponse{
		Items:    out,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *sandboxTemplateGRPCHandler) WatchSandboxTemplates(_ *pb.WatchSandboxTemplatesRequest, stream grpc.ServerStreamingServer[pb.WatchSandboxTemplatesResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	glog.V(4).Infof("WatchSandboxTemplates: subscriber %s connected", sub.ID)

	// Flush the header once the subscription is live so a client can seed its
	// state with a LIST without a list-watch gap (see gateways WatchGateways).
	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			glog.V(4).Infof("WatchSandboxTemplates: subscriber %s disconnected", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if evt.Source != sandboxTemplateEventSource {
				continue
			}

			watchEvent := &pb.WatchSandboxTemplatesResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}
			// A delete event carries only the id: the row is soft-deleted and the
			// import reconciler needs just the id to prune. A create/update loads
			// and sends the current object.
			if evt.EventType != api.DeleteEventType {
				item, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					glog.Warningf("WatchSandboxTemplates: failed to load sandbox template %s: %v", evt.SourceID, svcErr)
					continue
				}
				watchEvent.SandboxTemplate = sandboxTemplateToProto(item)
			}
			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchSandboxTemplates: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}
