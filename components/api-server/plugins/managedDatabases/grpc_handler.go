package managedDatabases

import (
	"context"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
	"github.com/openshift-online/rh-trex-ai/pkg/server/grpcutil"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

type managedDatabaseGRPCHandler struct {
	pb.UnimplementedManagedDatabaseServiceServer
	service    ManagedDatabaseService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewManagedDatabaseGRPCHandler(svc ManagedDatabaseService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.ManagedDatabaseServiceServer {
	return &managedDatabaseGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *managedDatabaseGRPCHandler) GetManagedDatabase(ctx context.Context, req *pb.GetManagedDatabaseRequest) (*pb.GetManagedDatabaseResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	managedDatabase, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetManagedDatabaseResponse{ManagedDatabase: managedDatabaseToProto(managedDatabase)}, nil
}

func (h *managedDatabaseGRPCHandler) CreateManagedDatabase(ctx context.Context, req *pb.CreateManagedDatabaseRequest) (*pb.CreateManagedDatabaseResponse, error) {
	if err := grpcutil.ValidateStringField("name", req.Name, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("provider", req.Provider, true); err != nil {
		return nil, err
	}

	managedDatabase := &ManagedDatabase{
		Name:             req.Name,
		Provider:         req.Provider,
		Region:           req.Region,
		Engine:           req.Engine,
		EngineVersion:    req.EngineVersion,
		InstanceClass:    req.InstanceClass,
		ConnectionSecret: req.ConnectionSecret,
		Status:           req.Status,
	}
	result, svcErr := h.service.Create(ctx, managedDatabase)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateManagedDatabaseResponse{ManagedDatabase: managedDatabaseToProto(result)}, nil
}

func (h *managedDatabaseGRPCHandler) UpdateManagedDatabase(ctx context.Context, req *pb.UpdateManagedDatabaseRequest) (*pb.UpdateManagedDatabaseResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	if req.Name != nil {
		if err := grpcutil.ValidateStringField("name", *req.Name, false); err != nil {
			return nil, err
		}
	}
	if req.Provider != nil {
		if err := grpcutil.ValidateStringField("provider", *req.Provider, false); err != nil {
			return nil, err
		}
	}
	if req.Region != nil {
		if err := grpcutil.ValidateStringField("region", *req.Region, false); err != nil {
			return nil, err
		}
	}
	if req.Engine != nil {
		if err := grpcutil.ValidateStringField("engine", *req.Engine, false); err != nil {
			return nil, err
		}
	}
	if req.EngineVersion != nil {
		if err := grpcutil.ValidateStringField("engine_version", *req.EngineVersion, false); err != nil {
			return nil, err
		}
	}
	if req.InstanceClass != nil {
		if err := grpcutil.ValidateStringField("instance_class", *req.InstanceClass, false); err != nil {
			return nil, err
		}
	}
	if req.ConnectionSecret != nil {
		if err := grpcutil.ValidateStringField("connection_secret", *req.ConnectionSecret, false); err != nil {
			return nil, err
		}
	}
	if req.Status != nil {
		if err := grpcutil.ValidateStringField("status", *req.Status, false); err != nil {
			return nil, err
		}
	}

	managedDatabase, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if req.Name != nil {
		managedDatabase.Name = *req.Name
	}
	if req.Provider != nil {
		managedDatabase.Provider = *req.Provider
	}
	if req.Region != nil {
		managedDatabase.Region = req.Region
	}
	if req.Engine != nil {
		managedDatabase.Engine = req.Engine
	}
	if req.EngineVersion != nil {
		managedDatabase.EngineVersion = req.EngineVersion
	}
	if req.InstanceClass != nil {
		managedDatabase.InstanceClass = req.InstanceClass
	}
	if req.ConnectionSecret != nil {
		managedDatabase.ConnectionSecret = req.ConnectionSecret
	}
	if req.Status != nil {
		managedDatabase.Status = req.Status
	}
	result, svcErr := h.service.Replace(ctx, managedDatabase)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateManagedDatabaseResponse{ManagedDatabase: managedDatabaseToProto(result)}, nil
}

func (h *managedDatabaseGRPCHandler) DeleteManagedDatabase(ctx context.Context, req *pb.DeleteManagedDatabaseRequest) (*pb.DeleteManagedDatabaseResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	svcErr := h.service.Delete(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteManagedDatabaseResponse{}, nil
}

func (h *managedDatabaseGRPCHandler) ListManagedDatabases(ctx context.Context, req *pb.ListManagedDatabasesRequest) (*pb.ListManagedDatabasesResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)

	listArgs := &services.ListArguments{
		Page: int(page),
		Size: int64(size),
	}

	var managedDatabases []ManagedDatabase
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &managedDatabases)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	items := make([]*pb.ManagedDatabase, len(managedDatabases))
	for i, d := range managedDatabases {
		items[i] = managedDatabaseToProto(&d)
	}

	return &pb.ListManagedDatabasesResponse{
		Items:    items,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

const (
	managedDatabaseDeleteTombstoneHeader = "hypershell-managed-database-delete-tombstones"
	managedDatabaseReplayRequestHeader   = "hypershell-managed-database-replay"
	managedDatabaseReplayRequestValue    = "deleted-v1"
	managedDatabaseReplayPageSize        = 500
)

func sendManagedDatabaseWatchHeader(stream grpc.ServerStreamingServer[pb.WatchManagedDatabasesResponse]) error {
	if err := stream.SendHeader(metadata.Pairs(managedDatabaseDeleteTombstoneHeader, "v1")); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}
	return nil
}

func managedDatabaseReplayRequested(ctx context.Context) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	values := md.Get(managedDatabaseReplayRequestHeader)
	return len(values) == 1 && values[0] == managedDatabaseReplayRequestValue
}

func (h *managedDatabaseGRPCHandler) replayDeletedManagedDatabases(ctx context.Context, stream grpc.ServerStreamingServer[pb.WatchManagedDatabasesResponse]) error {
	if err := sendManagedDatabaseWatchHeader(stream); err != nil {
		return err
	}
	for offset := 0; ; offset += managedDatabaseReplayPageSize {
		deleted, svcErr := h.service.ListDeleted(ctx, offset, managedDatabaseReplayPageSize)
		if svcErr != nil {
			return status.Errorf(codes.Unavailable, "failed to load ManagedDatabase delete tombstones: %v", svcErr)
		}
		for i := range deleted {
			managedDatabase := &deleted[i]
			if err := stream.Send(&pb.WatchManagedDatabasesResponse{
				Type:            pb.EventType_EVENT_TYPE_DELETED,
				ResourceId:      managedDatabase.ID,
				ManagedDatabase: managedDatabaseToProto(managedDatabase),
			}); err != nil {
				return status.Errorf(codes.Unavailable, "replay ManagedDatabase delete tombstone %s: %v", managedDatabase.ID, err)
			}
		}
		if len(deleted) < managedDatabaseReplayPageSize {
			return nil
		}
	}
}

func (h *managedDatabaseGRPCHandler) WatchManagedDatabases(req *pb.WatchManagedDatabasesRequest, stream grpc.ServerStreamingServer[pb.WatchManagedDatabasesResponse]) error {
	ctx := stream.Context()
	if managedDatabaseReplayRequested(ctx) {
		return h.replayDeletedManagedDatabases(ctx, stream)
	}

	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	glog.V(4).Infof("WatchManagedDatabases: subscriber %s connected", sub.ID)

	// Flush the header immediately after subscribing. The control plane starts
	// draining this live stream before it requests paginated historical tombstones,
	// so replay can never block consumption of the broker's bounded event channel.
	if err := sendManagedDatabaseWatchHeader(stream); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			glog.V(4).Infof("WatchManagedDatabases: subscriber %s disconnected", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}

			if evt.Source != "ManagedDatabases" {
				continue
			}

			watchEvent := &pb.WatchManagedDatabasesResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}

			if evt.EventType == api.DeleteEventType {
				managedDatabase, svcErr := h.service.GetUnscoped(ctx, evt.SourceID)
				if svcErr != nil {
					return status.Errorf(codes.Unavailable, "load ManagedDatabase delete tombstone %s: %v", evt.SourceID, svcErr)
				}
				watchEvent.ManagedDatabase = managedDatabaseToProto(managedDatabase)
			} else {
				managedDatabase, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					glog.Warningf("WatchManagedDatabases: failed to load managedDatabase %s: %v", evt.SourceID, svcErr)
					continue
				}
				watchEvent.ManagedDatabase = managedDatabaseToProto(managedDatabase)
			}

			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchManagedDatabases: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}
