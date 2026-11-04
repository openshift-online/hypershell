package ciTrusts

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
	// Imported CITrust tenancy (SEC-RBAC-25): on the declarative-import path the
	// trust may be authored ONLY on a gateway owned by the SAME reconciled
	// Repository. With no import context this is a no-op and the human HTTP path
	// (SEC-RBAC-22) is unchanged.
	if err := h.enforceImportGatewayOwnership(ctx, req.GatewayId); err != nil {
		return nil, err
	}
	item := &CITrust{
		Name:                req.GetName(),
		CIProviderID:        req.CiProviderId,
		GatewayID:           req.GatewayId,
		SubjectRepository:   req.SubjectRepository,
		SubjectProjectPath:  req.SubjectProjectPath,
		SubjectRef:          req.SubjectRef,
		SubjectEnvironment:  req.SubjectEnvironment,
		SubjectRefProtected: req.SubjectRefProtected,
		Role:                req.Role,
		// Import ownership is set server-side from trusted metadata; a client-supplied
		// repository_id is ignored so ownership cannot be forged (SEC-RBAC-25).
		RepositoryID: pkgrbac.ImportOwnershipMark(ctx),
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
	// On the import path a reconciler may update ONLY a CITrust its own Repository
	// owns; a trust owned by another Repository or created directly over REST is
	// refused (SEC-RBAC-25). With no import context this is a no-op.
	if err := pkgrbac.EnforceImportOwnership(ctx, found.RepositoryID); err != nil {
		return nil, err
	}
	if pkgrbac.ImportRepositoryIDFromContext(ctx) != "" {
		// Declarative import is DESIRED STATE: the mutable subject predicate and role
		// are REPLACED wholesale with the manifest's values, so a subject field the
		// manifest omits (nil) is CLEARED rather than left stale (FINDING 1). This is
		// deliberately different from the human sparse-PATCH path below, where nil means
		// "unchanged" (SEC-RBAC-22). Status fields are platform-owned (written by the
		// CITrust reconciler's token-exchange path), so an import never touches them and
		// they stay out of the replace set.
		found.SubjectRepository = req.SubjectRepository
		found.SubjectProjectPath = req.SubjectProjectPath
		found.SubjectRef = req.SubjectRef
		found.SubjectEnvironment = req.SubjectEnvironment
		found.SubjectRefProtected = req.SubjectRefProtected
		if req.Role != nil {
			found.Role = *req.Role
		}
	} else {
		// Human HTTP partial-PATCH (SEC-RBAC-22): nil means "unchanged", so a caller
		// can update one field without clearing the others.
		if req.SubjectRepository != nil {
			found.SubjectRepository = req.SubjectRepository
		}
		if req.SubjectProjectPath != nil {
			found.SubjectProjectPath = req.SubjectProjectPath
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
	found, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	// On the import path a reconciler may prune ONLY a CITrust its own Repository
	// owns, so one Repository's import never deletes another Repository's or a
	// REST/hsctl-created trust (SEC-RBAC-25). With no import context this is a no-op.
	if err := pkgrbac.EnforceImportOwnership(ctx, found.RepositoryID); err != nil {
		return nil, err
	}
	if svcErr := h.service.Delete(ctx, req.Id); svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteCITrustResponse{}, nil
}

// enforceImportGatewayOwnership implements the imported CITrust tenancy gate
// (SEC-RBAC-25): when the write carries a reconciled-Repository import binding, the
// referenced gateway MUST be owned by that SAME Repository (same repository_id
// ownership mark). A gateway owned by another Repository, by a human, or created
// directly over REST (nil ownership), or a missing gateway, is refused. With no
// import context the check is skipped so the human HTTP tenancy path (SEC-RBAC-22)
// is unchanged.
func (h *ciTrustGRPCHandler) enforceImportGatewayOwnership(ctx context.Context, gatewayID string) error {
	importRepositoryID := pkgrbac.ImportRepositoryIDFromContext(ctx)
	if importRepositoryID == "" {
		return nil
	}
	ownerRepositoryID, found, svcErr := h.service.GatewayRepositoryID(ctx, gatewayID)
	if svcErr != nil {
		return grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if !found || ownerRepositoryID == nil || *ownerRepositoryID != importRepositoryID {
		return status.Error(codes.PermissionDenied, "a repository import may author a CITrust only on a gateway it owns")
	}
	return nil
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
