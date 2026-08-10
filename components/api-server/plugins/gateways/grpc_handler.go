package gateways

import (
	"context"
	"encoding/json"

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

type gatewayGRPCHandler struct {
	pb.UnimplementedGatewayServiceServer
	service    GatewayService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewGatewayGRPCHandler(svc GatewayService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.GatewayServiceServer {
	return &gatewayGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *gatewayGRPCHandler) GetGateway(ctx context.Context, req *pb.GetGatewayRequest) (*pb.GetGatewayResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	gateway, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetGatewayResponse{Gateway: gatewayToProto(gateway)}, nil
}

func (h *gatewayGRPCHandler) CreateGateway(ctx context.Context, req *pb.CreateGatewayRequest) (*pb.CreateGatewayResponse, error) {
	if err := grpcutil.ValidateStringField("name", req.Name, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("fleet_id", req.FleetId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("cluster_id", req.ClusterId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("release_id", req.ReleaseId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("database_id", req.DatabaseId, true); err != nil {
		return nil, err
	}
	var serverDnsNamesJSON *string
	if len(req.ServerDnsNames) > 0 {
		data, _ := json.Marshal(req.ServerDnsNames)
		s := string(data)
		serverDnsNamesJSON = &s
	}

	gateway := &Gateway{
		Name:           req.Name,
		FleetId:        req.FleetId,
		ClusterId:      req.ClusterId,
		ReleaseId:      req.ReleaseId,
		DatabaseId:     req.DatabaseId,
		ExternalDns:    req.ExternalDns,
		TlsMode:        req.TlsMode,
		ServiceType:    req.ServiceType,
		Status:         req.Status,
		Phase:          req.Phase,
		Image:          req.Image,
		ServerDnsNames: serverDnsNamesJSON,
		Oidc:           req.Oidc,
		Route:          req.Route,
		DatabaseConfig: req.DatabaseConfig,
	}
	result, svcErr := h.service.Create(ctx, gateway)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateGatewayResponse{Gateway: gatewayToProto(result)}, nil
}

func (h *gatewayGRPCHandler) UpdateGateway(ctx context.Context, req *pb.UpdateGatewayRequest) (*pb.UpdateGatewayResponse, error) {
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
	if req.ClusterId != nil {
		if err := grpcutil.ValidateStringField("cluster_id", *req.ClusterId, false); err != nil {
			return nil, err
		}
	}
	if req.ReleaseId != nil {
		if err := grpcutil.ValidateStringField("release_id", *req.ReleaseId, false); err != nil {
			return nil, err
		}
	}
	if req.DatabaseId != nil {
		if err := grpcutil.ValidateStringField("database_id", *req.DatabaseId, false); err != nil {
			return nil, err
		}
	}
	if req.ExternalDns != nil {
		if err := grpcutil.ValidateStringField("external_dns", *req.ExternalDns, false); err != nil {
			return nil, err
		}
	}
	if req.TlsMode != nil {
		if err := grpcutil.ValidateStringField("tls_mode", *req.TlsMode, false); err != nil {
			return nil, err
		}
	}
	if req.ServiceType != nil {
		if err := grpcutil.ValidateStringField("service_type", *req.ServiceType, false); err != nil {
			return nil, err
		}
	}
	if req.Status != nil {
		if err := grpcutil.ValidateStringField("status", *req.Status, false); err != nil {
			return nil, err
		}
	}
	if req.Phase != nil {
		if err := grpcutil.ValidateStringField("phase", *req.Phase, false); err != nil {
			return nil, err
		}
	}

	gateway, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if req.Name != nil {
		gateway.Name = *req.Name
	}
	if req.FleetId != nil {
		gateway.FleetId = *req.FleetId
	}
	if req.ClusterId != nil {
		gateway.ClusterId = *req.ClusterId
	}
	if req.ReleaseId != nil {
		gateway.ReleaseId = *req.ReleaseId
	}
	if req.DatabaseId != nil {
		gateway.DatabaseId = *req.DatabaseId
	}
	if req.ExternalDns != nil {
		gateway.ExternalDns = req.ExternalDns
	}
	if req.TlsMode != nil {
		gateway.TlsMode = req.TlsMode
	}
	if req.ServiceType != nil {
		gateway.ServiceType = req.ServiceType
	}
	if req.Status != nil {
		gateway.Status = req.Status
	}
	if req.Phase != nil {
		gateway.Phase = req.Phase
	}
	if req.Image != nil {
		gateway.Image = req.Image
	}
	if len(req.ServerDnsNames) > 0 {
		data, _ := json.Marshal(req.ServerDnsNames)
		s := string(data)
		gateway.ServerDnsNames = &s
	}
	if req.RouteAddress != nil {
		gateway.RouteAddress = req.RouteAddress
	}
	if req.Oidc != nil {
		gateway.Oidc = req.Oidc
	}
	if req.Route != nil {
		gateway.Route = req.Route
	}
	if req.DatabaseConfig != nil {
		gateway.DatabaseConfig = req.DatabaseConfig
	}
	result, svcErr := h.service.Replace(ctx, gateway)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateGatewayResponse{Gateway: gatewayToProto(result)}, nil
}

func (h *gatewayGRPCHandler) DeleteGateway(ctx context.Context, req *pb.DeleteGatewayRequest) (*pb.DeleteGatewayResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	svcErr := h.service.Delete(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteGatewayResponse{}, nil
}

func (h *gatewayGRPCHandler) ListGateways(ctx context.Context, req *pb.ListGatewaysRequest) (*pb.ListGatewaysResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)

	listArgs := &services.ListArguments{
		Page: int(page),
		Size: int64(size),
	}

	var gateways []Gateway
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &gateways)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	items := make([]*pb.Gateway, len(gateways))
	for i, d := range gateways {
		items[i] = gatewayToProto(&d)
	}

	return &pb.ListGatewaysResponse{
		Items:    items,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *gatewayGRPCHandler) WatchGateways(req *pb.WatchGatewaysRequest, stream grpc.ServerStreamingServer[pb.WatchGatewaysResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	glog.V(4).Infof("WatchGateways: subscriber %s connected", sub.ID)

	for {
		select {
		case <-ctx.Done():
			glog.V(4).Infof("WatchGateways: subscriber %s disconnected", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}

			if evt.Source != "Gateways" {
				continue
			}

			watchEvent := &pb.WatchGatewaysResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}

			if evt.EventType == api.DeleteEventType {
				gateway, svcErr := h.service.GetUnscoped(ctx, evt.SourceID)
				if svcErr != nil {
					glog.Warningf("WatchGateways: failed to load soft-deleted gateway %s: %v", evt.SourceID, svcErr)
				} else {
					watchEvent.Gateway = gatewayToProto(gateway)
				}
			} else {
				gateway, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					glog.Warningf("WatchGateways: failed to load gateway %s: %v", evt.SourceID, svcErr)
					continue
				}
				watchEvent.Gateway = gatewayToProto(gateway)
			}

			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchGateways: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}
