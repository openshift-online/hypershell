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
	if err := grpcutil.ValidateStringField("cluster_id", req.ClusterId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("release_id", req.ReleaseId, true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("database_id", req.DatabaseId, false); err != nil {
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
	}
	if req.ProfileId != nil {
		gateway.ProfileId = *req.ProfileId
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
	if req.ClusterId != nil {
		gateway.ClusterId = *req.ClusterId
	}
	if req.ReleaseId != nil {
		gateway.ReleaseId = *req.ReleaseId
	}
	// database_id is server-owned placement state. Ignore values supplied by
	// callers; gateway creation business logic is the only assignment path.
	if req.ProfileId != nil && *req.ProfileId != "" {
		exists, existsErr := h.service.ProfileExists(ctx, *req.ProfileId)
		if existsErr != nil {
			return nil, grpcutil.ServiceErrorToGRPC(existsErr)
		}
		if !exists {
			return nil, status.Errorf(codes.NotFound, "gateway profile %s does not exist", *req.ProfileId)
		}
		gateway.ProfileId = *req.ProfileId
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
	if req.ConsoleAddress != nil {
		gateway.ConsoleAddress = req.ConsoleAddress
	}
	if req.Oidc != nil {
		gateway.Oidc = req.Oidc
	}
	if req.Route != nil {
		gateway.Route = req.Route
	}
	// active_sandbox_count is deliberately not settable here: it is
	// control-plane owned and mutated only via AdjustActiveSandboxCount /
	// SetActiveSandboxCount so this whole-row replace cannot clobber it.
	result, svcErr := h.service.Replace(ctx, gateway)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateGatewayResponse{Gateway: gatewayToProto(result)}, nil
}

// AdjustActiveSandboxCount applies a relative delta to the active_sandbox_count
// of the gateway backing the given namespace. The adjustment is atomic and
// floored at zero. A namespace with no live gateway is a no-op (count 0).
func (h *gatewayGRPCHandler) AdjustActiveSandboxCount(ctx context.Context, req *pb.AdjustActiveSandboxCountRequest) (*pb.AdjustActiveSandboxCountResponse, error) {
	if err := grpcutil.ValidateStringField("namespace", req.Namespace, true); err != nil {
		return nil, err
	}
	count, svcErr := h.service.AdjustActiveSandboxCount(ctx, req.Namespace, int(req.Delta))
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.AdjustActiveSandboxCountResponse{ActiveSandboxCount: int32(count)}, nil
}

// SetActiveSandboxCount sets the active_sandbox_count of the gateway backing the
// given namespace to an absolute observed value (self-heal). The value is
// floored at zero. A namespace with no live gateway is a no-op (count 0).
func (h *gatewayGRPCHandler) SetActiveSandboxCount(ctx context.Context, req *pb.SetActiveSandboxCountRequest) (*pb.SetActiveSandboxCountResponse, error) {
	if err := grpcutil.ValidateStringField("namespace", req.Namespace, true); err != nil {
		return nil, err
	}
	count, svcErr := h.service.SetActiveSandboxCount(ctx, req.Namespace, int(req.Count))
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.SetActiveSandboxCountResponse{ActiveSandboxCount: int32(count)}, nil
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

	// Flush the stream header now that the subscription is live. This turns opening
	// the stream into a real subscription handshake: a client that blocks on the
	// response header (the control-plane watcher does) knows no event can be missed
	// once Header() returns, and can then safely LIST to seed its state without a
	// list-watch gap. Sending an empty header is a no-op for clients that ignore it.
	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}

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
