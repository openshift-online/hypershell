package gatewayNetworks

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

type gatewayNetworkGRPCHandler struct {
	pb.UnimplementedGatewayNetworkServiceServer
	service    GatewayNetworkService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewGatewayNetworkGRPCHandler(svc GatewayNetworkService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.GatewayNetworkServiceServer {
	return &gatewayNetworkGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *gatewayNetworkGRPCHandler) GetGatewayNetwork(ctx context.Context, req *pb.GetGatewayNetworkRequest) (*pb.GetGatewayNetworkResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	gatewayNetwork, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetGatewayNetworkResponse{GatewayNetwork: gatewayNetworkToProto(gatewayNetwork)}, nil
}

func (h *gatewayNetworkGRPCHandler) CreateGatewayNetwork(ctx context.Context, req *pb.CreateGatewayNetworkRequest) (*pb.CreateGatewayNetworkResponse, error) {
	if err := grpcutil.ValidateStringField("name", req.Name, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("fleet_id", req.FleetId, true); err != nil {
		return nil, err
	}

	gatewayNetwork := &GatewayNetwork{
		Name:         req.Name,
		FleetId:      req.FleetId,
		Topology:     req.Topology,
		TunnelMode:   req.TunnelMode,
		HubGatewayId: req.HubGatewayId,
		Status:       req.Status,
	}
	result, svcErr := h.service.Create(ctx, gatewayNetwork)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateGatewayNetworkResponse{GatewayNetwork: gatewayNetworkToProto(result)}, nil
}

func (h *gatewayNetworkGRPCHandler) UpdateGatewayNetwork(ctx context.Context, req *pb.UpdateGatewayNetworkRequest) (*pb.UpdateGatewayNetworkResponse, error) {
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
	if req.Topology != nil {
		if err := grpcutil.ValidateStringField("topology", *req.Topology, false); err != nil {
			return nil, err
		}
	}
	if req.TunnelMode != nil {
		if err := grpcutil.ValidateStringField("tunnel_mode", *req.TunnelMode, false); err != nil {
			return nil, err
		}
	}
	if req.HubGatewayId != nil {
		if err := grpcutil.ValidateStringField("hub_gateway_id", *req.HubGatewayId, false); err != nil {
			return nil, err
		}
	}
	if req.Status != nil {
		if err := grpcutil.ValidateStringField("status", *req.Status, false); err != nil {
			return nil, err
		}
	}

	gatewayNetwork, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if req.Name != nil {
		gatewayNetwork.Name = *req.Name
	}
	if req.FleetId != nil {
		gatewayNetwork.FleetId = *req.FleetId
	}
	if req.Topology != nil {
		gatewayNetwork.Topology = req.Topology
	}
	if req.TunnelMode != nil {
		gatewayNetwork.TunnelMode = req.TunnelMode
	}
	if req.HubGatewayId != nil {
		gatewayNetwork.HubGatewayId = req.HubGatewayId
	}
	if req.Status != nil {
		gatewayNetwork.Status = req.Status
	}
	result, svcErr := h.service.Replace(ctx, gatewayNetwork)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateGatewayNetworkResponse{GatewayNetwork: gatewayNetworkToProto(result)}, nil
}

func (h *gatewayNetworkGRPCHandler) DeleteGatewayNetwork(ctx context.Context, req *pb.DeleteGatewayNetworkRequest) (*pb.DeleteGatewayNetworkResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	svcErr := h.service.Delete(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteGatewayNetworkResponse{}, nil
}

func (h *gatewayNetworkGRPCHandler) ListGatewayNetworks(ctx context.Context, req *pb.ListGatewayNetworksRequest) (*pb.ListGatewayNetworksResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)

	listArgs := &services.ListArguments{
		Page: int(page),
		Size: int64(size),
	}

	var gatewayNetworks []GatewayNetwork
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &gatewayNetworks)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	items := make([]*pb.GatewayNetwork, len(gatewayNetworks))
	for i, d := range gatewayNetworks {
		items[i] = gatewayNetworkToProto(&d)
	}

	return &pb.ListGatewayNetworksResponse{
		Items:    items,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *gatewayNetworkGRPCHandler) WatchGatewayNetworks(req *pb.WatchGatewayNetworksRequest, stream grpc.ServerStreamingServer[pb.WatchGatewayNetworksResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	glog.V(4).Infof("WatchGatewayNetworks: subscriber %s connected", sub.ID)

	for {
		select {
		case <-ctx.Done():
			glog.V(4).Infof("WatchGatewayNetworks: subscriber %s disconnected", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}

			if evt.Source != "GatewayNetworks" {
				continue
			}

			watchEvent := &pb.WatchGatewayNetworksResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}

			if evt.EventType != api.DeleteEventType {
				gatewayNetwork, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					glog.Warningf("WatchGatewayNetworks: failed to load gatewayNetwork %s: %v", evt.SourceID, svcErr)
					continue
				}
				watchEvent.GatewayNetwork = gatewayNetworkToProto(gatewayNetwork)
			}

			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchGatewayNetworks: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}
