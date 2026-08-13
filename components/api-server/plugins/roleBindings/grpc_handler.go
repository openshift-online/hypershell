package roleBindings

import (
	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
	"github.com/openshift-online/rh-trex-ai/pkg/server/grpcutil"
)

type roleBindingGRPCHandler struct {
	pb.UnimplementedRoleBindingServiceServer
	service     RoleBindingService
	roleService roles.RoleService
	brokerFunc  func() *pkgserver.EventBroker
}

func NewRoleBindingGRPCHandler(svc RoleBindingService, roleService roles.RoleService, brokerFunc func() *pkgserver.EventBroker) pb.RoleBindingServiceServer {
	return &roleBindingGRPCHandler{service: svc, roleService: roleService, brokerFunc: brokerFunc}
}

func (h *roleBindingGRPCHandler) WatchRoleBindings(req *pb.WatchRoleBindingsRequest, stream grpc.ServerStreamingServer[pb.WatchRoleBindingsResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	glog.V(4).Infof("WatchRoleBindings: subscriber %s connected", sub.ID)

	for {
		select {
		case <-ctx.Done():
			glog.V(4).Infof("WatchRoleBindings: subscriber %s disconnected", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}

			if evt.Source != "RoleBindings" {
				continue
			}

			watchEvent := &pb.WatchRoleBindingsResponse{
				Type:       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId: evt.SourceID,
			}

			var rb *RoleBinding
			if evt.EventType == api.DeleteEventType {
				rb, _ = h.service.GetUnscoped(ctx, evt.SourceID)
			} else {
				var svcErr interface{ Error() string }
				rb, svcErr = h.service.Get(ctx, evt.SourceID)
				if svcErr != nil {
					glog.Warningf("WatchRoleBindings: failed to load role binding %s: %v", evt.SourceID, svcErr)
					continue
				}
			}

			if rb != nil {
				roleName := ""
				if h.roleService != nil {
					role, roleErr := h.roleService.Get(ctx, rb.RoleID)
					if roleErr == nil {
						roleName = role.Name
					}
				}
				watchEvent.RoleBinding = roleBindingToProto(rb, roleName)
			}

			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchRoleBindings: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}
