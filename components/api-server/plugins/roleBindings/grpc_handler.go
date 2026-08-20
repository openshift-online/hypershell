package roleBindings

import (
	"context"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	pkgserver "github.com/openshift-online/rh-trex-ai/pkg/server"
	"github.com/openshift-online/rh-trex-ai/pkg/server/grpcutil"
)

type roleBindingGRPCHandler struct {
	pb.UnimplementedRoleBindingServiceServer
	service     RoleBindingService
	roleService roles.RoleService
	userService users.UserService
	brokerFunc  func() *pkgserver.EventBroker
}

func NewRoleBindingGRPCHandler(svc RoleBindingService, roleService roles.RoleService, userService users.UserService, brokerFunc func() *pkgserver.EventBroker) pb.RoleBindingServiceServer {
	return &roleBindingGRPCHandler{service: svc, roleService: roleService, userService: userService, brokerFunc: brokerFunc}
}

// ListRoleBindings returns the active role bindings for a user, optionally
// filtered to a single gateway. The control plane uses it on RoleBinding delete
// to recompute a user's effective Keycloak roles before revoking any, so a role
// still granted by another binding (e.g. openshell-user held via both an owner
// and a viewer binding) is not removed.
func (h *roleBindingGRPCHandler) ListRoleBindings(ctx context.Context, req *pb.ListRoleBindingsRequest) (*pb.ListRoleBindingsResponse, error) {
	userID := req.GetUserId()
	if userID == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	bindings, svcErr := h.service.FindByUserID(ctx, userID)
	if svcErr != nil {
		return nil, status.Errorf(codes.Internal, "list role bindings for user %s: %v", userID, svcErr)
	}

	resp := &pb.ListRoleBindingsResponse{}
	for _, rb := range bindings {
		if rb == nil {
			continue
		}
		if req.GatewayId != nil {
			if rb.GatewayID == nil || *rb.GatewayID != *req.GatewayId {
				continue
			}
		}

		roleName := ""
		if h.roleService != nil {
			role, roleErr := h.roleService.Get(ctx, rb.RoleID)
			if roleErr != nil {
				// The control plane unions role names across a user's surviving
				// bindings to decide which Keycloak roles to keep when a binding is
				// deleted. A binding returned with an unresolved (empty) role name
				// would silently drop out of that union and could cause a role still
				// granted by another binding to be revoked. Fail the whole list
				// rather than hand back an incomplete view, so revocation aborts and
				// retries once the role is resolvable again.
				return nil, status.Errorf(codes.Internal, "resolve role %s for binding %s: %v", rb.RoleID, rb.ID, roleErr)
			}
			roleName = role.Name
		}
		username := ""
		if rb.UserID != nil && h.userService != nil {
			if user, userErr := h.userService.Get(ctx, *rb.UserID); userErr == nil {
				username = user.Username
			}
		}
		resp.Items = append(resp.Items, roleBindingToProto(rb, roleName, username))
	}

	return resp, nil
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
				var unscopedErr *errors.ServiceError
				rb, unscopedErr = h.service.GetUnscoped(ctx, evt.SourceID)
				if unscopedErr != nil {
					glog.Warningf("WatchRoleBindings: failed to load deleted role binding %s: %v", evt.SourceID, unscopedErr)
					continue
				}
			} else {
				var svcErr *errors.ServiceError
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
				username := ""
				if rb.UserID != nil && h.userService != nil {
					user, userErr := h.userService.Get(ctx, *rb.UserID)
					if userErr == nil {
						username = user.Username
					}
				}
				watchEvent.RoleBinding = roleBindingToProto(rb, roleName, username)
			}

			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchRoleBindings: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}
