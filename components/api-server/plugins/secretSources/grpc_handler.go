package secretSources

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

type secretSourceGRPCHandler struct {
	pb.UnimplementedSecretSourceServiceServer
	service    SecretSourceService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewSecretSourceGRPCHandler(svc SecretSourceService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.SecretSourceServiceServer {
	return &secretSourceGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *secretSourceGRPCHandler) GetSecretSource(ctx context.Context, req *pb.GetSecretSourceRequest) (*pb.GetSecretSourceResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	item, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetSecretSourceResponse{SecretSource: secretSourceToProto(item)}, nil
}

func (h *secretSourceGRPCHandler) CreateSecretSource(ctx context.Context, req *pb.CreateSecretSourceRequest) (*pb.CreateSecretSourceResponse, error) {
	if err := grpcutil.ValidateStringField("name", req.Name, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("agent_runtime_id", req.AgentRuntimeId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("purpose", req.Purpose, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("backend", req.Backend, true); err != nil {
		return nil, err
	}
	// path is optional: a reference-only SecretSource (for example one declared in a
	// .hypershell import) MAY omit it, in which case the server derives the stable
	// convention location from the submission identity, matching the REST create path
	// (agent-secret-late-binding.spec.md "Reference-Only SecretSource Submission").
	path := req.Path
	if path == "" {
		path = deterministicSecretLocation(req.AgentRuntimeId, req.Name)
	}
	item := &SecretSource{
		Name:           req.Name,
		AgentRuntimeId: req.AgentRuntimeId,
		Purpose:        req.Purpose,
		Backend:        req.Backend,
		Path:           path,
		KeyMappings:    req.KeyMappings,
		RepositoryId:   pkgrbac.ImportOwnershipMark(ctx), // import ownership set server-side; a client-supplied repository_id is ignored
	}
	created, svcErr := h.service.Create(ctx, item)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateSecretSourceResponse{SecretSource: secretSourceToProto(created)}, nil
}

func (h *secretSourceGRPCHandler) UpdateSecretSource(ctx context.Context, req *pb.UpdateSecretSourceRequest) (*pb.UpdateSecretSourceResponse, error) {
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
	if req.Purpose != nil {
		found.Purpose = *req.Purpose
	}
	if req.Backend != nil {
		found.Backend = *req.Backend
	}
	if req.Path != nil {
		found.Path = *req.Path
	}
	if req.KeyMappings != nil {
		found.KeyMappings = req.KeyMappings
	}
	updated, svcErr := h.service.Replace(ctx, found)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateSecretSourceResponse{SecretSource: secretSourceToProto(updated)}, nil
}

func (h *secretSourceGRPCHandler) DeleteSecretSource(ctx context.Context, req *pb.DeleteSecretSourceRequest) (*pb.DeleteSecretSourceResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	found, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if found == nil {
		return nil, status.Errorf(codes.NotFound, "secret source %s not found", req.Id)
	}
	if err := pkgrbac.EnforceImportOwnership(ctx, found.RepositoryId); err != nil {
		return nil, err
	}
	if svcErr = h.service.Delete(ctx, req.Id); svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteSecretSourceResponse{}, nil
}

func (h *secretSourceGRPCHandler) ListSecretSources(ctx context.Context, req *pb.ListSecretSourcesRequest) (*pb.ListSecretSourcesResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)
	listArgs := &services.ListArguments{Page: int(page), Size: int64(size)}

	var items []SecretSource
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &items)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	out := make([]*pb.SecretSource, len(items))
	for i := range items {
		out[i] = secretSourceToProto(&items[i])
	}
	return &pb.ListSecretSourcesResponse{
		Items:    out,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *secretSourceGRPCHandler) WatchSecretSources(_ *pb.WatchSecretSourcesRequest, stream grpc.ServerStreamingServer[pb.WatchSecretSourcesResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	slog.DebugContext(ctx, "WatchSecretSources subscriber connected", "subscriber_id", sub.ID)

	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			slog.DebugContext(ctx, "WatchSecretSources subscriber disconnected", "subscriber_id", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if evt.Source != secretSourceEventSource {
				continue
			}

			watchEvent := &pb.WatchSecretSourcesResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}
			if evt.EventType != api.DeleteEventType {
				item, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					slog.WarnContext(ctx, "WatchSecretSources failed to load secret source", "resource_id", evt.SourceID, "error", svcErr)
					continue
				}
				watchEvent.SecretSource = secretSourceToProto(item)
			}
			if err := stream.Send(watchEvent); err != nil {
				slog.DebugContext(ctx, "WatchSecretSources send failed", "subscriber_id", sub.ID, "error", err)
				return err
			}
		}
	}
}
