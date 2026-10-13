package agentRuntimes

import (
	"context"

	"github.com/golang/glog"
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

// GatewayClusterLookup resolves the managed cluster a gateway is assigned to
// (clusterID, found, err), including soft-deleted gateways: a gateway's cluster
// assignment is a fact about the row that deletion does not change, and an
// AgentRuntime that derives its placement from a since-deleted gateway must still
// resolve to the cluster that owned it. It aliases the shared seam the gateways
// plugin already exposes (roleBindings.GatewayClusterLookup), so the gateways
// service locator satisfies both consumers without an import cycle.
type GatewayClusterLookup = roleBindings.GatewayClusterLookup

type agentRuntimeGRPCHandler struct {
	pb.UnimplementedAgentRuntimeServiceServer
	service         AgentRuntimeService
	gatewayClusters GatewayClusterLookup
	brokerFunc      func() *pkgserver.EventBroker
}

func NewAgentRuntimeGRPCHandler(svc AgentRuntimeService, gatewayClusters GatewayClusterLookup, brokerFunc func() *pkgserver.EventBroker) pb.AgentRuntimeServiceServer {
	return &agentRuntimeGRPCHandler{service: svc, gatewayClusters: gatewayClusters, brokerFunc: brokerFunc}
}

// clusterScope filters AgentRuntimes to those whose effective cluster is one
// managed cluster. The effective cluster is the AgentRuntime's own cluster_id
// when set, else the cluster of its referenced gateway (derived placement,
// data-model.spec.md "AgentRuntime as a Top-Level Resource"). The event broker
// fans every AgentRuntime out to every subscriber, so for a registered control
// plane (which the gRPC RBAC caller binding forces to pass its own cluster_id)
// this is the boundary that keeps other clusters' runtimes off its stream. A nil
// *clusterScope means no filter.
type clusterScope struct {
	clusterID string
	lookup    GatewayClusterLookup
	// gatewayClusters caches gatewayID -> clusterID ("" when the gateway does not
	// exist) for the lifetime of one List call or one replay.
	gatewayClusters map[string]string
}

// newClusterScope validates the request's cluster_id and returns the scope, or
// nil when the request is unfiltered.
func (h *agentRuntimeGRPCHandler) newClusterScope(clusterID string) (*clusterScope, error) {
	if clusterID == "" {
		return nil, nil
	}
	if err := grpcutil.ValidateStringField("cluster_id", clusterID, false); err != nil {
		return nil, err
	}
	if h.gatewayClusters == nil {
		// Fail closed: without the gateway lookup a filtered request whose
		// AgentRuntime omits cluster_id cannot be scoped, and serving it
		// unfiltered would leak other clusters' runtimes.
		return nil, status.Error(codes.Unavailable, "gateway lookup unavailable; cannot scope agent runtimes to cluster_id")
	}
	return &clusterScope{clusterID: clusterID, lookup: h.gatewayClusters, gatewayClusters: map[string]string{}}, nil
}

// includes reports whether ar belongs to the scoped cluster by its effective
// cluster. An AgentRuntime with an explicit cluster_id is matched directly. One
// that omits cluster_id derives its placement from its gateway; a runtime whose
// gateway does not exist is excluded (found is false) so the caller can log it,
// and a lookup failure is returned so the caller fails the request rather than
// silently dropping a runtime the cluster owns.
func (c *clusterScope) includes(ctx context.Context, ar *AgentRuntime) (included bool, found bool, err *errors.ServiceError) {
	if ar == nil {
		return false, true, nil
	}
	if ar.ClusterId != "" {
		return ar.ClusterId == c.clusterID, true, nil
	}
	if ar.GatewayId == "" {
		// No explicit cluster and no gateway to derive one from: not attributable
		// to any cluster.
		return false, false, nil
	}
	gatewayID := ar.GatewayId
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

// ListAgentRuntimes returns the AgentRuntimes whose effective cluster matches the
// request's cluster_id. The control plane uses it to seed the watch on
// (re)connect. cluster_id is required for a registered control plane (the caller
// binding enforces it); an unscoped list is allowed only for callers that are not
// cluster-bound (platform admins over gRPC).
func (h *agentRuntimeGRPCHandler) ListAgentRuntimes(ctx context.Context, req *pb.ListAgentRuntimesRequest) (*pb.ListAgentRuntimesResponse, error) {
	scope, scopeErr := h.newClusterScope(req.GetClusterId())
	if scopeErr != nil {
		return nil, scopeErr
	}

	runtimes, svcErr := h.service.All(ctx)
	if svcErr != nil {
		return nil, status.Errorf(codes.Internal, "list agent runtimes: %v", svcErr)
	}

	resp := &pb.ListAgentRuntimesResponse{}
	for _, ar := range runtimes {
		if ar == nil {
			continue
		}
		if scope != nil {
			included, found, lookupErr := scope.includes(ctx, ar)
			if lookupErr != nil {
				return nil, status.Errorf(codes.Internal, "resolve effective cluster of agent runtime %s: %v", ar.ID, lookupErr)
			}
			if !found || !included {
				continue
			}
		}
		resp.Items = append(resp.Items, agentRuntimeToProto(ar))
	}
	return resp, nil
}

func (h *agentRuntimeGRPCHandler) WatchAgentRuntimes(req *pb.WatchAgentRuntimesRequest, stream grpc.ServerStreamingServer[pb.WatchAgentRuntimesResponse]) error {
	broker := h.brokerFunc()
	if broker == nil {
		return status.Error(codes.Unavailable, "event broker not available")
	}

	// clusterFilter, when set, scopes this stream to AgentRuntimes whose effective
	// cluster is that managed cluster (see clusterScope). The broker fans EVERY
	// AgentRuntime out to EVERY subscriber; for a caller whose JWT subject is a
	// registered ManagedCluster the gRPC RBAC caller binding has already required
	// cluster_id to equal that cluster's id before this handler runs, so the filter
	// is enforced, not cooperative, for control planes.
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
	glog.V(4).Infof("WatchAgentRuntimes: subscriber %s connected", sub.ID)

	// Flush the header after subscribing, then replay the current inventory as
	// updates. This closes the list-watch gap: the control plane awaits this header
	// before its seed LIST, so an AgentRuntime event that fires during the seed is
	// captured by the watch rather than lost.
	if err := stream.SendHeader(nil); err != nil {
		return status.Errorf(codes.Unavailable, "failed to send watch header: %v", err)
	}
	if err := h.replayAgentRuntimes(ctx, stream, scope); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			glog.V(4).Infof("WatchAgentRuntimes: subscriber %s disconnected", sub.ID)
			return nil
		case evt, ok := <-sub.Events:
			if !ok {
				return nil
			}

			if evt.Source != agentRuntimeEventSource {
				continue
			}

			var ar *AgentRuntime
			var svcErr *errors.ServiceError
			if evt.EventType == api.DeleteEventType {
				// The row is already soft-deleted on a delete event; load it unscoped
				// so the delete still reaches the cluster that owned the runtime.
				ar, svcErr = h.service.GetUnscoped(ctx, evt.SourceID)
			} else {
				ar, svcErr = h.service.Get(ctx, evt.SourceID)
			}
			if svcErr != nil {
				glog.Warningf("WatchAgentRuntimes: failed to load agent runtime %s: %v", evt.SourceID, svcErr)
				continue
			}

			if scope != nil {
				// Re-resolve per event: a gateway can move between clusters, so an
				// assignment cached from the replay or an earlier event could misroute
				// this one.
				clear(scope.gatewayClusters)
				included, found, lookupErr := scope.includes(ctx, ar)
				if lookupErr != nil {
					// Ending the stream makes the control plane reconnect, and the
					// replay on reconnect restores the runtime; dropping it here would
					// strand it until an unrelated reconnect.
					glog.Warningf("WatchAgentRuntimes: failed to resolve the effective cluster of agent runtime %s for subscriber %s: %v", ar.ID, sub.ID, lookupErr)
					return status.Errorf(codes.Unavailable, "resolve effective cluster for agent runtime %s: %v", ar.ID, lookupErr)
				}
				if !found {
					// Not attributable to any cluster, so it must not reach a scoped
					// subscriber.
					glog.Warningf("WatchAgentRuntimes: effective cluster of agent runtime %s unresolved; not delivering to cluster %s", ar.ID, clusterFilter)
					continue
				}
				if !included {
					continue
				}
			}

			watchEvent := &pb.WatchAgentRuntimesResponse{
				Type:         pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId:   ar.ID,
				AgentRuntime: agentRuntimeToProto(ar),
			}
			if err := stream.Send(watchEvent); err != nil {
				glog.V(4).Infof("WatchAgentRuntimes: send error for subscriber %s: %v", sub.ID, err)
				return err
			}
		}
	}
}

// replayAgentRuntimes sends every current AgentRuntime as an update. scope, when
// non-nil, limits the replay to runtimes whose effective cluster is the watched
// cluster. It is the server side of seed-on-reconnect: the stream otherwise sends
// only future events.
func (h *agentRuntimeGRPCHandler) replayAgentRuntimes(ctx context.Context, stream grpc.ServerStreamingServer[pb.WatchAgentRuntimesResponse], scope *clusterScope) error {
	runtimes, svcErr := h.service.All(ctx)
	if svcErr != nil {
		return status.Errorf(codes.Unavailable, "failed to load agent runtimes: %v", svcErr)
	}

	replayed := 0
	for _, ar := range runtimes {
		if ar == nil {
			continue
		}
		if scope != nil {
			included, found, lookupErr := scope.includes(ctx, ar)
			if lookupErr != nil {
				return status.Errorf(codes.Unavailable, "failed to resolve effective cluster for agent runtime %s: %v", ar.ID, lookupErr)
			}
			if !found || !included {
				continue
			}
		}
		if err := stream.Send(&pb.WatchAgentRuntimesResponse{
			Type:         pb.EventType_EVENT_TYPE_UPDATED,
			ResourceId:   ar.ID,
			AgentRuntime: agentRuntimeToProto(ar),
		}); err != nil {
			return status.Errorf(codes.Unavailable, "failed to replay agent runtime %s: %v", ar.ID, err)
		}
		replayed++
	}
	glog.V(4).Infof("WatchAgentRuntimes: replayed %d agent runtimes", replayed)
	return nil
}
