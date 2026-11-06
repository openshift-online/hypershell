package defaultWorkspaceMemberships

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	pkgserver "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server/grpcutil"
)

type grpcHandler struct {
	pb.UnimplementedDefaultWorkspaceMembershipServiceServer
	service         MembershipService
	gatewayClusters roleBindings.GatewayClusterLookup
	brokerFunc      func() *pkgserver.EventBroker
}

func NewGRPCHandler(svc MembershipService, gatewayClusters roleBindings.GatewayClusterLookup, brokerFunc func() *pkgserver.EventBroker) pb.DefaultWorkspaceMembershipServiceServer {
	return &grpcHandler{service: svc, gatewayClusters: gatewayClusters, brokerFunc: brokerFunc}
}

// clusterScope filters memberships to those whose gateway is assigned to one
// managed cluster. The event broker fans every event out to every subscriber,
// so for a registered control plane (forced to pass its own cluster_id, see
// pkg/rbac cluster caller binding) this is the boundary that keeps other
// clusters' gateways' memberships, and the subjects they carry, off its
// stream. A nil *clusterScope means no filter. Mirrors
// roleBindings.clusterScope.
type clusterScope struct {
	clusterID       string
	lookup          roleBindings.GatewayClusterLookup
	gatewayClusters map[string]string
}

func (h *grpcHandler) newClusterScope(clusterID string) (*clusterScope, error) {
	if clusterID == "" {
		return nil, nil
	}
	if err := grpcutil.ValidateStringField("cluster_id", clusterID, false); err != nil {
		return nil, err
	}
	if h.gatewayClusters == nil {
		return nil, status.Error(codes.Unavailable, "gateway lookup unavailable; cannot scope default workspace memberships to cluster_id")
	}
	return &clusterScope{clusterID: clusterID, lookup: h.gatewayClusters, gatewayClusters: map[string]string{}}, nil
}

func (c *clusterScope) includes(ctx context.Context, m *DefaultWorkspaceMembership) (included bool, found bool, err *errors.ServiceError) {
	if m == nil || m.GatewayID == "" {
		return false, true, nil
	}
	clusterID, cached := c.gatewayClusters[m.GatewayID]
	if !cached {
		var exists bool
		var svcErr *errors.ServiceError
		clusterID, exists, svcErr = c.lookup(ctx, m.GatewayID)
		if svcErr != nil {
			return false, false, svcErr
		}
		if !exists {
			clusterID = ""
		}
		c.gatewayClusters[m.GatewayID] = clusterID
	}
	if clusterID == "" {
		return false, false, nil
	}
	return clusterID == c.clusterID, true, nil
}

func (h *grpcHandler) ListDefaultWorkspaceMemberships(ctx context.Context, req *pb.ListDefaultWorkspaceMembershipsRequest) (*pb.ListDefaultWorkspaceMembershipsResponse, error) {
	scope, scopeErr := h.newClusterScope(req.GetClusterId())
	if scopeErr != nil {
		return nil, scopeErr
	}

	var items DefaultWorkspaceMembershipList
	var svcErr *errors.ServiceError
	if req.GatewayId != nil {
		items, svcErr = h.service.AllForGateway(ctx, req.GetGatewayId())
	} else {
		items, svcErr = h.service.All(ctx)
	}
	if svcErr != nil {
		return nil, status.Errorf(codes.Internal, "list default workspace memberships: %v", svcErr)
	}

	resp := &pb.ListDefaultWorkspaceMembershipsResponse{}
	for _, m := range items {
		if m == nil {
			continue
		}
		if scope != nil {
			included, _, lookupErr := scope.includes(ctx, m)
			if lookupErr != nil {
				return nil, status.Errorf(codes.Internal, "resolve cluster of gateway %s for default workspace membership %s: %v", m.GatewayID, m.ID, lookupErr)
			}
			if !included {
				continue
			}
		}
		resp.Items = append(resp.Items, toProto(m))
	}
	return resp, nil
}

func (h *grpcHandler) WatchDefaultWorkspaceMemberships(req *pb.WatchDefaultWorkspaceMembershipsRequest, stream grpc.ServerStreamingServer[pb.WatchDefaultWorkspaceMembershipsResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

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
	slog.DebugContext(ctx, "WatchDefaultWorkspaceMemberships subscriber connected", "subscriber_id", sub.ID)

	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}
	if err := h.replayActive(ctx, stream, scope); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			slog.DebugContext(ctx, "WatchDefaultWorkspaceMemberships subscriber disconnected", "subscriber_id", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}
			if evt.Source != eventSource {
				continue
			}

			var m *DefaultWorkspaceMembership
			if evt.EventType == api.DeleteEventType {
				// GetUnscoped (not Get): the row's gateway_id and subject are
				// still needed -- by the cluster-scope filter below, and by the
				// control plane, which keys its reconcile queue on gateway_id
				// (not this row's own id) and must know which gateway's full
				// membership set to re-converge after a delete.
				var unscopedErr *errors.ServiceError
				m, unscopedErr = h.service.GetUnscoped(ctx, evt.SourceID)
				if unscopedErr != nil {
					slog.WarnContext(ctx, "WatchDefaultWorkspaceMemberships failed to load deleted default workspace membership", "resource_id", evt.SourceID, "error", unscopedErr)
					continue
				}
			} else {
				var svcErr *errors.ServiceError
				m, svcErr = loadWithRetry(ctx, func() (*DefaultWorkspaceMembership, *errors.ServiceError) {
					return h.service.Get(ctx, evt.SourceID)
				})
				if svcErr != nil {
					slog.WarnContext(ctx, "WatchDefaultWorkspaceMemberships failed to load membership", "resource_id", evt.SourceID, "error", svcErr)
					continue
				}
			}

			if scope != nil {
				clear(scope.gatewayClusters)
				included, found, lookupErr := scope.includes(ctx, m)
				if lookupErr != nil {
					slog.WarnContext(ctx, "WatchDefaultWorkspaceMemberships failed to resolve the gateway cluster", "membership_id", m.ID, "subscriber_id", sub.ID, "error", lookupErr)
					return status.Errorf(codes.Unavailable, "resolve gateway cluster for default workspace membership %s: %v", m.ID, lookupErr)
				}
				if !found || !included {
					continue
				}
			}

			watchEvent := &pb.WatchDefaultWorkspaceMembershipsResponse{
				Type:                       pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId:                 m.ID,
				DefaultWorkspaceMembership: toProto(m),
			}
			if err := stream.Send(watchEvent); err != nil {
				slog.DebugContext(ctx, "WatchDefaultWorkspaceMemberships send failed", "subscriber_id", sub.ID, "error", err)
				return err
			}
		}
	}
}

func (h *grpcHandler) replayActive(ctx context.Context, stream grpc.ServerStreamingServer[pb.WatchDefaultWorkspaceMembershipsResponse], scope *clusterScope) error {
	items, svcErr := h.service.All(ctx)
	if svcErr != nil {
		return status.Errorf(codes.Unavailable, "failed to load active default workspace memberships: %v", svcErr)
	}

	replayed := 0
	for _, m := range items {
		if m == nil {
			continue
		}
		if scope != nil {
			included, found, lookupErr := scope.includes(ctx, m)
			if lookupErr != nil {
				return status.Errorf(codes.Unavailable, "failed to resolve gateway cluster for default workspace membership %s: %v", m.ID, lookupErr)
			}
			if !found || !included {
				continue
			}
		}
		watchEvent := &pb.WatchDefaultWorkspaceMembershipsResponse{
			Type:                       pb.EventType_EVENT_TYPE_UPDATED,
			ResourceId:                 m.ID,
			DefaultWorkspaceMembership: toProto(m),
		}
		if err := stream.Send(watchEvent); err != nil {
			return status.Errorf(codes.Unavailable, "failed to replay default workspace membership %s: %v", m.ID, err)
		}
		replayed++
	}
	slog.DebugContext(ctx, "WatchDefaultWorkspaceMemberships replayed active memberships", "count", replayed)
	return nil
}

const loadAttempts = 5
const loadRetry = 100 * time.Millisecond

// loadWithRetry re-reads a membership after the events table notifies watchers,
// mirroring roleBindings.loadRoleBindingWithRetry: the service writes the row and
// then the event in separate statements, so a subscriber can observe the event
// before Get sees the commit.
func loadWithRetry(ctx context.Context, load func() (*DefaultWorkspaceMembership, *errors.ServiceError)) (*DefaultWorkspaceMembership, *errors.ServiceError) {
	var lastErr *errors.ServiceError
	for attempt := 0; attempt < loadAttempts; attempt++ {
		m, err := load()
		if err == nil {
			return m, nil
		}
		lastErr = err
		if !err.Is404() || attempt == loadAttempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, lastErr
		case <-time.After(loadRetry):
		}
	}
	return nil, lastErr
}

func (h *grpcHandler) UpdateDefaultWorkspaceMembershipStatus(ctx context.Context, req *pb.UpdateDefaultWorkspaceMembershipStatusRequest) (*pb.UpdateDefaultWorkspaceMembershipStatusResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	// The gRPC authorization layer (grpc_authorization.go) only checks that
	// req.GatewayId belongs to the caller's registered cluster; it has no way to
	// verify that req.GatewayId is actually the gateway that owns the row named
	// by req.Id. Without this check here, a registered control plane could
	// supply its own gateway_id alongside the id of a membership row belonging
	// to a gateway on a different cluster and overwrite that row's status.
	existing, getErr := h.service.Get(ctx, req.GetId())
	if getErr != nil {
		if getErr.Is404() {
			return nil, status.Errorf(codes.NotFound, "default workspace membership %s not found", req.GetId())
		}
		return nil, status.Errorf(codes.Internal, "get default workspace membership %s: %v", req.GetId(), getErr)
	}
	if existing.GatewayID != req.GetGatewayId() {
		return nil, status.Errorf(codes.PermissionDenied, "default workspace membership %s does not belong to gateway %s", req.GetId(), req.GetGatewayId())
	}
	item, svcErr := h.service.UpdateStatus(ctx, req.GetId(), req.GetReconciliationStatus(), req.LastReconcileError)
	if svcErr != nil {
		if svcErr.Is404() {
			return nil, status.Errorf(codes.NotFound, "default workspace membership %s not found", req.GetId())
		}
		return nil, status.Errorf(codes.Internal, "update default workspace membership status: %v", svcErr)
	}
	return &pb.UpdateDefaultWorkspaceMembershipStatusResponse{Item: toProto(item)}, nil
}
