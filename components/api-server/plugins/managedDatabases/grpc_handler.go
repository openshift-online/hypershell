package managedDatabases

import (
	"context"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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
	if err := grpcutil.ValidateStringField("fleet_id", req.FleetId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("provider", req.Provider, true); err != nil {
		return nil, err
	}

	managedDatabase := &ManagedDatabase{
		Name:             req.Name,
		FleetId:          req.FleetId,
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
	if req.FleetId != nil {
		if err := grpcutil.ValidateStringField("fleet_id", *req.FleetId, false); err != nil {
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
	if req.FleetId != nil {
		managedDatabase.FleetId = *req.FleetId
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

func (h *managedDatabaseGRPCHandler) WatchManagedDatabases(req *pb.WatchManagedDatabasesRequest, stream grpc.ServerStreamingServer[pb.WatchManagedDatabasesResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	glog.V(4).Infof("WatchManagedDatabases: subscriber %s connected", sub.ID)

	// Flush the stream header now that the subscription is live. This turns opening
	// the stream into a real subscription handshake: a client that blocks on the
	// response header (the control-plane watcher does, to seed without a list-watch
	// gap) knows no event can be missed once Header() returns. Sending an empty
	// header is a no-op for clients that ignore it.
	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
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

			if evt.EventType != api.DeleteEventType {
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
