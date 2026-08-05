package fleets

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

type fleetGRPCHandler struct {
	pb.UnimplementedFleetServiceServer
	service    FleetService
	generic    services.GenericService
	brokerFunc func() *pkgserver.EventBroker
}

func NewFleetGRPCHandler(svc FleetService, generic services.GenericService, brokerFunc func() *pkgserver.EventBroker) pb.FleetServiceServer {
	return &fleetGRPCHandler{service: svc, generic: generic, brokerFunc: brokerFunc}
}

func (h *fleetGRPCHandler) GetFleet(ctx context.Context, req *pb.GetFleetRequest) (*pb.GetFleetResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	fleet, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.GetFleetResponse{Fleet: fleetToProto(fleet)}, nil
}

func (h *fleetGRPCHandler) CreateFleet(ctx context.Context, req *pb.CreateFleetRequest) (*pb.CreateFleetResponse, error) {
	if err := grpcutil.ValidateStringField("name", req.Name, true); err != nil {
		return nil, err
	}

	fleet := &Fleet{
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
	}
	result, svcErr := h.service.Create(ctx, fleet)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.CreateFleetResponse{Fleet: fleetToProto(result)}, nil
}

func (h *fleetGRPCHandler) UpdateFleet(ctx context.Context, req *pb.UpdateFleetRequest) (*pb.UpdateFleetResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}
	if req.Name != nil {
		if err := grpcutil.ValidateStringField("name", *req.Name, false); err != nil {
			return nil, err
		}
	}
	if req.Description != nil {
		if err := grpcutil.ValidateStringField("description", *req.Description, false); err != nil {
			return nil, err
		}
	}
	if req.Status != nil {
		if err := grpcutil.ValidateStringField("status", *req.Status, false); err != nil {
			return nil, err
		}
	}

	fleet, svcErr := h.service.Get(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if req.Name != nil {
		fleet.Name = *req.Name
	}
	if req.Description != nil {
		fleet.Description = req.Description
	}
	if req.Status != nil {
		fleet.Status = req.Status
	}
	result, svcErr := h.service.Replace(ctx, fleet)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.UpdateFleetResponse{Fleet: fleetToProto(result)}, nil
}

func (h *fleetGRPCHandler) DeleteFleet(ctx context.Context, req *pb.DeleteFleetRequest) (*pb.DeleteFleetResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.Id); err != nil {
		return nil, err
	}

	svcErr := h.service.Delete(ctx, req.Id)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteFleetResponse{}, nil
}

func (h *fleetGRPCHandler) ListFleets(ctx context.Context, req *pb.ListFleetsRequest) (*pb.ListFleetsResponse, error) {
	page, size := grpcutil.NormalizePagination(req.Page, req.Size)

	listArgs := &services.ListArguments{
		Page: int(page),
		Size: int64(size),
	}

	var fleets []Fleet
	paging, svcErr := h.generic.List(ctx, "id", listArgs, &fleets)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}

	items := make([]*pb.Fleet, len(fleets))
	for i, d := range fleets {
		items[i] = fleetToProto(&d)
	}

	return &pb.ListFleetsResponse{
		Items:    items,
		Metadata: &pb.ListMeta{Page: page, Size: size, Total: int32(paging.Total)},
	}, nil
}

func (h *fleetGRPCHandler) WatchFleets(req *pb.WatchFleetsRequest, stream grpc.ServerStreamingServer[pb.WatchFleetsResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	glog.V(4).Infof("WatchFleets: subscriber %s connected", sub.ID)

	for {
		select {
		case <-ctx.Done():
			glog.V(4).Infof("WatchFleets: subscriber %s disconnected", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}

			if evt.Source != "Fleets" {
				continue
			}

			watchEvent := &pb.WatchFleetsResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}

			if evt.EventType != api.DeleteEventType {
				fleet, svcErr := h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					glog.Warningf("WatchFleets: failed to load fleet %s: %v", evt.SourceID, svcErr)
					continue
				}
				watchEvent.Fleet = fleetToProto(fleet)
			}

			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchFleets: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}
