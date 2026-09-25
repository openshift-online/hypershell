package roleBindings

import (
	"context"
	"time"

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

// GatewayClusterLookup resolves the managed cluster a gateway is assigned to.
// found is false when no gateway row exists. It includes soft-deleted gateways:
// a gateway's cluster assignment is a fact about the row that deletion does not
// change, and a binding DELETE that follows its gateway's deletion must still
// reach the cluster that owns the gateway.
type GatewayClusterLookup func(ctx context.Context, gatewayID string) (clusterID string, found bool, err *errors.ServiceError)

// GatewayClusterLookupSource is implemented by the gateways plugin's service
// locator. plugins/gateways already imports this package (gateway owner
// bindings), so this package resolves the lookup through the service registry by
// interface rather than a typed gateways.Service accessor, which would be an
// import cycle.
type GatewayClusterLookupSource interface {
	GatewayClusterLookup() GatewayClusterLookup
}

type roleBindingGRPCHandler struct {
	pb.UnimplementedRoleBindingServiceServer
	service         RoleBindingService
	roleService     roles.RoleService
	userService     users.UserService
	gatewayClusters GatewayClusterLookup
	brokerFunc      func() *pkgserver.EventBroker
}

func NewRoleBindingGRPCHandler(svc RoleBindingService, roleService roles.RoleService, userService users.UserService, gatewayClusters GatewayClusterLookup, brokerFunc func() *pkgserver.EventBroker) pb.RoleBindingServiceServer {
	return &roleBindingGRPCHandler{service: svc, roleService: roleService, userService: userService, gatewayClusters: gatewayClusters, brokerFunc: brokerFunc}
}

// clusterScope filters role bindings to those whose gateway is assigned to one
// managed cluster. The event broker fans every RoleBinding event out to every
// subscriber, so for a registered control plane (which the gRPC RBAC interceptor
// forces to pass its own cluster_id, see pkg/rbac cluster caller binding) this is
// the boundary that keeps other clusters' bindings, and the user identities they
// carry, off its stream. A nil *clusterScope means no filter.
type clusterScope struct {
	clusterID string
	lookup    GatewayClusterLookup
	// gatewayClusters caches gatewayID -> clusterID ("" when the gateway does
	// not exist) for the lifetime of one List call or one replay.
	gatewayClusters map[string]string
}

// newClusterScope validates the request's cluster_id and returns the scope, or
// nil when the request is unfiltered.
func (h *roleBindingGRPCHandler) newClusterScope(clusterID string) (*clusterScope, error) {
	if clusterID == "" {
		return nil, nil
	}
	if err := grpcutil.ValidateStringField("cluster_id", clusterID, false); err != nil {
		return nil, err
	}
	if h.gatewayClusters == nil {
		// Fail closed: without the gateway lookup a filtered request cannot be
		// scoped, and serving it unfiltered would leak other clusters' bindings.
		return nil, status.Error(codes.Unavailable, "gateway lookup unavailable; cannot scope role bindings to cluster_id")
	}
	return &clusterScope{clusterID: clusterID, lookup: h.gatewayClusters, gatewayClusters: map[string]string{}}, nil
}

// includes reports whether rb belongs to the scoped cluster. Global bindings (no
// gateway) never do. A binding whose gateway does not exist is excluded (found
// is false) so the caller can log it; a lookup failure is returned so the caller
// fails the request instead of silently dropping a binding the cluster owns.
func (c *clusterScope) includes(ctx context.Context, rb *RoleBinding) (included bool, found bool, err *errors.ServiceError) {
	if rb == nil || rb.GatewayID == nil || *rb.GatewayID == "" {
		return false, true, nil
	}
	gatewayID := *rb.GatewayID
	clusterID, cached := c.gatewayClusters[gatewayID]
	if !cached {
		var exists bool
		var svcErr *errors.ServiceError
		clusterID, exists, svcErr = c.lookup(ctx, gatewayID)
		if svcErr != nil {
			return false, false, svcErr
		}
		if !exists {
			clusterID = ""
		}
		c.gatewayClusters[gatewayID] = clusterID
	}
	if clusterID == "" {
		return false, false, nil
	}
	return clusterID == c.clusterID, true, nil
}

// ListRoleBindings returns the active role bindings for a user, optionally
// filtered to a single gateway. The control plane uses it on RoleBinding delete
// to recompute a user's effective Keycloak roles before revoking any, so a role
// still granted by another binding (e.g. openshell-user held via both an owner
// and a viewer binding) is not removed.
//
// cluster_id, when set, excludes bindings whose gateway is not assigned to that
// managed cluster and all global bindings, with the same semantics as the
// WatchRoleBindings filter.
func (h *roleBindingGRPCHandler) ListRoleBindings(ctx context.Context, req *pb.ListRoleBindingsRequest) (*pb.ListRoleBindingsResponse, error) {
	userID := req.GetUserId()
	if userID == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	scope, scopeErr := h.newClusterScope(req.GetClusterId())
	if scopeErr != nil {
		return nil, scopeErr
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
		if scope != nil {
			included, _, lookupErr := scope.includes(ctx, rb)
			if lookupErr != nil {
				// Same reasoning as an unresolved role below: dropping a binding
				// the cluster owns would understate the user's surviving roles.
				return nil, status.Errorf(codes.Internal, "resolve cluster of gateway %s for binding %s: %v", *rb.GatewayID, rb.ID, lookupErr)
			}
			if !included {
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

	// clusterFilter, when set, scopes this stream to bindings whose gateway is
	// assigned to that managed cluster (see clusterScope). The broker fans EVERY
	// binding out to EVERY subscriber; for a caller whose JWT subject is a
	// registered ManagedCluster the gRPC RBAC interceptor has already required
	// cluster_id to equal that cluster's id before this handler runs, so the
	// filter is enforced, not cooperative, for control planes.
	clusterFilter := req.GetClusterId()
	scope, scopeErr := h.newClusterScope(clusterFilter)
	if scopeErr != nil {
		return scopeErr
	}

	ctx := stream.Context()
	sub, err := broker.Subscribe(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to subscribe: %v", err)
	}
	glog.V(4).Infof("WatchRoleBindings: subscriber %s connected", sub.ID)

	// Send the header after the subscription starts. Then send each active
	// gateway binding as an update. This restores bindings that the control
	// plane missed while the stream was not connected. The subscription keeps
	// new events in its queue until this function sends the current bindings.
	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}
	if err := h.replayActiveRoleBindings(ctx, stream, scope); err != nil {
		return err
	}

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

			var rb *RoleBinding
			if evt.EventType == api.DeleteEventType {
				// Already-gone on delete is terminal; retrying NotFound
				// stalls the watch send loop for a row that will not return.
				var unscopedErr *errors.ServiceError
				rb, unscopedErr = h.service.GetUnscoped(ctx, evt.SourceID)
				if unscopedErr != nil {
					glog.Warningf("WatchRoleBindings: failed to load deleted role binding %s: %v", evt.SourceID, unscopedErr)
					continue
				}
			} else {
				var svcErr *errors.ServiceError
				rb, svcErr = loadRoleBindingWithRetry(ctx, func() (*RoleBinding, *errors.ServiceError) {
					return h.service.Get(ctx, evt.SourceID)
				})
				if svcErr != nil {
					glog.Warningf("WatchRoleBindings: failed to load role binding %s: %v", evt.SourceID, svcErr)
					continue
				}
			}

			if scope != nil {
				// Re-resolve per event: a gateway can move between clusters, so
				// an assignment cached from the replay or an earlier event could
				// misroute this one.
				clear(scope.gatewayClusters)
				included, found, lookupErr := scope.includes(ctx, rb)
				if lookupErr != nil {
					// Ending the stream makes the control plane reconnect, and the
					// replay on reconnect restores the binding; dropping it here
					// would strand it until an unrelated reconnect.
					glog.Warningf("WatchRoleBindings: failed to resolve the gateway cluster of role binding %s for subscriber %s: %v", rb.ID, sub.ID, lookupErr)
					return status.Errorf(codes.Unavailable, "resolve gateway cluster for role binding %s: %v", rb.ID, lookupErr)
				}
				if !found {
					// Not attributable to any cluster, so it must not reach a
					// scoped subscriber.
					glog.Warningf("WatchRoleBindings: gateway %s of role binding %s not found; not delivering to cluster %s", *rb.GatewayID, rb.ID, clusterFilter)
					continue
				}
				if !included {
					continue
				}
			}

			watchEvent := h.roleBindingWatchEvent(ctx, pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)), rb)

			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchRoleBindings: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}

// replayActiveRoleBindings sends every active gateway binding as an update.
// scope, when non-nil, limits the replay to bindings whose gateway is assigned
// to the watched cluster.
func (h *roleBindingGRPCHandler) replayActiveRoleBindings(ctx context.Context, stream grpc.ServerStreamingServer[pb.WatchRoleBindingsResponse], scope *clusterScope) error {
	bindings, svcErr := h.service.All(ctx)
	if svcErr != nil {
		return status.Errorf(codes.Unavailable, "failed to load active role bindings: %v", svcErr)
	}

	replayed := 0
	for _, rb := range bindings {
		// Global bindings do not map to gateway client roles.
		if rb == nil || rb.GatewayID == nil || *rb.GatewayID == "" {
			continue
		}
		if scope != nil {
			included, found, lookupErr := scope.includes(ctx, rb)
			if lookupErr != nil {
				return status.Errorf(codes.Unavailable, "failed to resolve gateway cluster for role binding %s: %v", rb.ID, lookupErr)
			}
			if !found {
				glog.Warningf("WatchRoleBindings: gateway %s of role binding %s not found; not replaying to cluster %s", *rb.GatewayID, rb.ID, scope.clusterID)
				continue
			}
			if !included {
				continue
			}
		}
		if err := stream.Send(h.roleBindingWatchEvent(ctx, pb.EventType_EVENT_TYPE_UPDATED, rb)); err != nil {
			return status.Errorf(codes.Unavailable, "failed to replay role binding %s: %v", rb.ID, err)
		}
		replayed++
	}
	glog.V(4).Infof("WatchRoleBindings: replayed %d active role bindings", replayed)
	return nil
}

const roleBindingLoadAttempts = 5
const roleBindingLoadRetry = 100 * time.Millisecond

// loadRoleBindingWithRetry re-reads a RoleBinding after the events table
// notifies watchers. CreateOwnerBinding writes the row and then the event in
// separate statements, so a subscriber can observe the event before Get sees
// the commit. Dropping that event strands Keycloak role assignment until the
// watch reconnects.
func loadRoleBindingWithRetry(ctx context.Context, load func() (*RoleBinding, *errors.ServiceError)) (*RoleBinding, *errors.ServiceError) {
	var lastErr *errors.ServiceError
	for attempt := 0; attempt < roleBindingLoadAttempts; attempt++ {
		rb, err := load()
		if err == nil {
			return rb, nil
		}
		lastErr = err
		// The event-before-commit race is NotFound. Other codes are terminal.
		if !err.Is404() || attempt == roleBindingLoadAttempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, lastErr
		case <-time.After(roleBindingLoadRetry):
		}
	}
	return nil, lastErr
}

func (h *roleBindingGRPCHandler) roleBindingWatchEvent(ctx context.Context, eventType pb.EventType, rb *RoleBinding) *pb.WatchRoleBindingsResponse {
	watchEvent := &pb.WatchRoleBindingsResponse{Type: eventType}
	if rb == nil {
		return watchEvent
	}

	watchEvent.ResourceId = rb.ID
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
	return watchEvent
}
