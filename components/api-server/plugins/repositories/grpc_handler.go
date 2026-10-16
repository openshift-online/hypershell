package repositories

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

type repositoryGRPCHandler struct {
	pb.UnimplementedRepositoryServiceServer
	service    RepositoryService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewRepositoryGRPCHandler(svc RepositoryService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.RepositoryServiceServer {
	return &repositoryGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *repositoryGRPCHandler) GetRepository(ctx context.Context, req *pb.GetRepositoryRequest) (*pb.GetRepositoryResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	item, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetRepositoryResponse{Repository: repositoryToProto(item)}, nil
}

func (h *repositoryGRPCHandler) CreateRepository(ctx context.Context, req *pb.CreateRepositoryRequest) (*pb.CreateRepositoryResponse, error) {
	if err := grpcutil.ValidateStringField("name", req.Name, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("url", req.Url, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("provider", req.Provider, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("secret_source_id", req.SecretSourceId, true); err != nil {
		return nil, err
	}
	item := &Repository{
		Name:           req.Name,
		Url:            req.Url,
		Provider:       req.Provider,
		SecretSourceId: req.SecretSourceId,
		DefaultBranch:  req.DefaultBranch,
		ImportPath:     req.ImportPath,
	}
	created, svcErr := h.service.Create(ctx, item)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateRepositoryResponse{Repository: repositoryToProto(created)}, nil
}

func (h *repositoryGRPCHandler) UpdateRepository(ctx context.Context, req *pb.UpdateRepositoryRequest) (*pb.UpdateRepositoryResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	found, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if req.SecretSourceId != nil {
		found.SecretSourceId = *req.SecretSourceId
	}
	if req.DefaultBranch != nil {
		found.DefaultBranch = req.DefaultBranch
	}
	if req.ImportPath != nil {
		found.ImportPath = req.ImportPath
	}
	if req.Status != nil {
		found.Status = req.Status
	}
	updated, svcErr := h.service.Replace(ctx, found)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateRepositoryResponse{Repository: repositoryToProto(updated)}, nil
}

func (h *repositoryGRPCHandler) DeleteRepository(ctx context.Context, req *pb.DeleteRepositoryRequest) (*pb.DeleteRepositoryResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	if svcErr := h.service.Delete(ctx, req.Id); svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteRepositoryResponse{}, nil
}

func (h *repositoryGRPCHandler) ListRepositories(ctx context.Context, req *pb.ListRepositoriesRequest) (*pb.ListRepositoriesResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)
	listArgs := &services.ListArguments{Page: int(page), Size: int64(size)}

	var items []Repository
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &items)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	out := make([]*pb.Repository, len(items))
	for i := range items {
		out[i] = repositoryToProto(&items[i])
	}
	return &pb.ListRepositoriesResponse{
		Items:    out,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *repositoryGRPCHandler) WatchRepositories(_ *pb.WatchRepositoriesRequest, stream grpc.ServerStreamingServer[pb.WatchRepositoriesResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	glog.V(4).Infof("WatchRepositories: subscriber %s connected", sub.ID)

	// Flush the header once the subscription is live so a client can seed its
	// state with a LIST without a list-watch gap (see gateways WatchGateways).
	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			glog.V(4).Infof("WatchRepositories: subscriber %s disconnected", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if evt.Source != repositoryEventSource {
				continue
			}

			watchEvent := &pb.WatchRepositoriesResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}
			// A delete event carries only the id: the row is soft-deleted and the
			// import reconciler needs just the id to prune. A create/update loads
			// and sends the current object.
			if evt.EventType != api.DeleteEventType {
				item, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					glog.Warningf("WatchRepositories: failed to load repository %s: %v", evt.SourceID, svcErr)
					continue
				}
				watchEvent.Repository = repositoryToProto(item)
			}
			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchRepositories: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}
