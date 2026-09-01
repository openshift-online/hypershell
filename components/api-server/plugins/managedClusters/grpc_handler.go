package managedClusters

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

type managedClusterGRPCHandler struct {
	pb.UnimplementedManagedClusterServiceServer
	service    ManagedClusterService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
	profiles   ProfileChecker
}

func NewManagedClusterGRPCHandler(svc ManagedClusterService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker, profiles ProfileChecker) pb.ManagedClusterServiceServer {
	return &managedClusterGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc, profiles: profiles}
}

func (h *managedClusterGRPCHandler) GetManagedCluster(ctx context.Context, req *pb.GetManagedClusterRequest) (*pb.GetManagedClusterResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	managedCluster, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetManagedClusterResponse{ManagedCluster: managedClusterToProto(managedCluster)}, nil
}

func (h *managedClusterGRPCHandler) CreateManagedCluster(ctx context.Context, req *pb.CreateManagedClusterRequest) (*pb.CreateManagedClusterResponse, error) {
	if err := grpcutil.ValidateStringField("name", req.Name, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("provider", req.Provider, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("kubeconfig_secret", req.KubeconfigSecret, true); err != nil {
		return nil, err
	}

	if req.ProfileId != nil && *req.ProfileId != "" {
		exists, existsErr := h.profiles.ProfileExists(ctx, *req.ProfileId)
		if existsErr != nil {
			return nil, grpcutil.ServiceErrorToGRPC(existsErr)
		}
		if !exists {
			return nil, status.Errorf(codes.NotFound, "gateway profile %s does not exist", *req.ProfileId)
		}
	}

	managedCluster := &ManagedCluster{
		Name:             req.Name,
		Provider:         req.Provider,
		Region:           req.Region,
		KubeconfigSecret: req.KubeconfigSecret,
		Status:           req.Status,
		ApiServerUrl:     req.ApiServerUrl,
		ProfileId:        req.ProfileId,
		DatabaseId:       req.DatabaseId,
	}
	result, svcErr := h.service.Create(ctx, managedCluster)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateManagedClusterResponse{ManagedCluster: managedClusterToProto(result)}, nil
}

func (h *managedClusterGRPCHandler) UpdateManagedCluster(ctx context.Context, req *pb.UpdateManagedClusterRequest) (*pb.UpdateManagedClusterResponse, error) {
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
	if req.KubeconfigSecret != nil {
		if err := grpcutil.ValidateStringField("kubeconfig_secret", *req.KubeconfigSecret, false); err != nil {
			return nil, err
		}
	}
	if req.Status != nil {
		if err := grpcutil.ValidateStringField("status", *req.Status, false); err != nil {
			return nil, err
		}
	}
	if req.ApiServerUrl != nil {
		if err := grpcutil.ValidateStringField("api_server_url", *req.ApiServerUrl, false); err != nil {
			return nil, err
		}
	}

	managedCluster, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if req.Name != nil {
		managedCluster.Name = *req.Name
	}
	if req.Provider != nil {
		managedCluster.Provider = *req.Provider
	}
	if req.Region != nil {
		managedCluster.Region = req.Region
	}
	if req.KubeconfigSecret != nil {
		managedCluster.KubeconfigSecret = *req.KubeconfigSecret
	}
	if req.Status != nil {
		managedCluster.Status = req.Status
	}
	if req.ApiServerUrl != nil {
		managedCluster.ApiServerUrl = req.ApiServerUrl
	}
	if req.ProfileId != nil {
		if *req.ProfileId != "" {
			exists, existsErr := h.profiles.ProfileExists(ctx, *req.ProfileId)
			if existsErr != nil {
				return nil, grpcutil.ServiceErrorToGRPC(existsErr)
			}
			if !exists {
				return nil, status.Errorf(codes.NotFound, "gateway profile %s does not exist", *req.ProfileId)
			}
		}
		managedCluster.ProfileId = req.ProfileId
	}
	if req.DatabaseId != nil {
		managedCluster.DatabaseId = req.DatabaseId
	}
	result, svcErr := h.service.Replace(ctx, managedCluster)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateManagedClusterResponse{ManagedCluster: managedClusterToProto(result)}, nil
}

func (h *managedClusterGRPCHandler) DeleteManagedCluster(ctx context.Context, req *pb.DeleteManagedClusterRequest) (*pb.DeleteManagedClusterResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	svcErr := h.service.Delete(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteManagedClusterResponse{}, nil
}

func (h *managedClusterGRPCHandler) ListManagedClusters(ctx context.Context, req *pb.ListManagedClustersRequest) (*pb.ListManagedClustersResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)

	listArgs := &services.ListArguments{
		Page: int(page),
		Size: int64(size),
	}

	var managedClusters []ManagedCluster
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &managedClusters)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	items := make([]*pb.ManagedCluster, len(managedClusters))
	for i, d := range managedClusters {
		items[i] = managedClusterToProto(&d)
	}

	return &pb.ListManagedClustersResponse{
		Items:    items,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *managedClusterGRPCHandler) WatchManagedClusters(req *pb.WatchManagedClustersRequest, stream grpc.ServerStreamingServer[pb.WatchManagedClustersResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	glog.V(4).Infof("WatchManagedClusters: subscriber %s connected", sub.ID)

	for {
		select {
		case <-ctx.Done():
			glog.V(4).Infof("WatchManagedClusters: subscriber %s disconnected", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}

			if evt.Source != "ManagedClusters" {
				continue
			}

			watchEvent := &pb.WatchManagedClustersResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}

			if evt.EventType != api.DeleteEventType {
				managedCluster, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					glog.Warningf("WatchManagedClusters: failed to load managedCluster %s: %v", evt.SourceID, svcErr)
					continue
				}
				watchEvent.ManagedCluster = managedClusterToProto(managedCluster)
			}

			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchManagedClusters: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}
