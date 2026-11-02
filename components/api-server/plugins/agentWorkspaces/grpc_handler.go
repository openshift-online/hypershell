package agentWorkspaces

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

type agentWorkspaceGRPCHandler struct {
	pb.UnimplementedAgentWorkspaceServiceServer
	service    AgentWorkspaceService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewAgentWorkspaceGRPCHandler(svc AgentWorkspaceService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.AgentWorkspaceServiceServer {
	return &agentWorkspaceGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *agentWorkspaceGRPCHandler) GetAgentWorkspace(ctx context.Context, req *pb.GetAgentWorkspaceRequest) (*pb.GetAgentWorkspaceResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	item, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetAgentWorkspaceResponse{AgentWorkspace: agentWorkspaceToProto(item)}, nil
}

func (h *agentWorkspaceGRPCHandler) CreateAgentWorkspace(ctx context.Context, req *pb.CreateAgentWorkspaceRequest) (*pb.CreateAgentWorkspaceResponse, error) {
	if err := grpcutil.ValidateStringField("agent_runtime_id", req.AgentRuntimeId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("gateway_id", req.GatewayId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("name", req.Name, true); err != nil {
		return nil, err
	}
	item := &AgentWorkspace{
		AgentRuntimeId: req.AgentRuntimeId,
		GatewayId:      req.GatewayId,
		Name:           req.Name,
		Status:         req.Status,
	}
	created, svcErr := h.service.Create(ctx, item)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateAgentWorkspaceResponse{AgentWorkspace: agentWorkspaceToProto(created)}, nil
}

func (h *agentWorkspaceGRPCHandler) UpdateAgentWorkspace(ctx context.Context, req *pb.UpdateAgentWorkspaceRequest) (*pb.UpdateAgentWorkspaceResponse, error) {
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
	if req.Status != nil {
		found.Status = req.Status
	}
	updated, svcErr := h.service.Replace(ctx, found)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateAgentWorkspaceResponse{AgentWorkspace: agentWorkspaceToProto(updated)}, nil
}

func (h *agentWorkspaceGRPCHandler) DeleteAgentWorkspace(ctx context.Context, req *pb.DeleteAgentWorkspaceRequest) (*pb.DeleteAgentWorkspaceResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	found, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if found == nil {
		return nil, status.Errorf(codes.NotFound, "agent workspace %s not found", req.Id)
	}
	if svcErr = h.service.Delete(ctx, req.Id); svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteAgentWorkspaceResponse{}, nil
}

func (h *agentWorkspaceGRPCHandler) ListAgentWorkspaces(ctx context.Context, req *pb.ListAgentWorkspacesRequest) (*pb.ListAgentWorkspacesResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)
	listArgs := &services.ListArguments{Page: int(page), Size: int64(size)}

	var items []AgentWorkspace
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &items)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	out := make([]*pb.AgentWorkspace, len(items))
	for i := range items {
		out[i] = agentWorkspaceToProto(&items[i])
	}
	return &pb.ListAgentWorkspacesResponse{
		Items:    out,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *agentWorkspaceGRPCHandler) WatchAgentWorkspaces(_ *pb.WatchAgentWorkspacesRequest, stream grpc.ServerStreamingServer[pb.WatchAgentWorkspacesResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	slog.DebugContext(ctx, "WatchAgentWorkspaces subscriber connected", "subscriber_id", sub.ID)

	// Flush the header once the subscription is live so a client can seed its
	// state with a LIST without a list-watch gap (see gateways WatchGateways).
	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			slog.DebugContext(ctx, "WatchAgentWorkspaces subscriber disconnected", "subscriber_id", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if evt.Source != agentWorkspaceEventSource {
				continue
			}

			watchEvent := &pb.WatchAgentWorkspacesResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}
			// A delete event carries only the id: the row is soft-deleted and a
			// consumer needs just the id to prune. A create/update loads and sends
			// the current object.
			if evt.EventType != api.DeleteEventType {
				item, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					slog.WarnContext(ctx, "WatchAgentWorkspaces failed to load agent workspace", "resource_id", evt.SourceID, "error", svcErr)
					continue
				}
				watchEvent.AgentWorkspace = agentWorkspaceToProto(item)
			}
			if err := stream.Send(watchEvent); err != nil {
				slog.DebugContext(ctx, "WatchAgentWorkspaces send failed", "subscriber_id", sub.ID, "error", err)
				return err
			}
		}
	}
}
