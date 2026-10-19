package agentRuntimes

import (
	"context"

	"github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	pkgrbac "github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
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
	secretSources   SecretSourceLookup
	workspaces      WorkspaceLookup
	brokerFunc      func() *pkgserver.EventBroker
}

func NewAgentRuntimeGRPCHandler(svc AgentRuntimeService, gatewayClusters GatewayClusterLookup, secretSources SecretSourceLookup, workspaces WorkspaceLookup, brokerFunc func() *pkgserver.EventBroker) pb.AgentRuntimeServiceServer {
	return &agentRuntimeGRPCHandler{service: svc, gatewayClusters: gatewayClusters, secretSources: secretSources, workspaces: workspaces, brokerFunc: brokerFunc}
}

// resolveWorkspace returns the gateway-side workspace view owned by the runtime for
// embedding in its gRPC payload. When the lookup seam is unavailable (the
// agentWorkspaces plugin is absent) or the runtime has no workspace yet it returns
// nil so the stream still delivers the runtime; the reconciler then provisions
// nothing gateway-side for it rather than the watch silently breaking.
func (h *agentRuntimeGRPCHandler) resolveWorkspace(ctx context.Context, ar *AgentRuntime) (*WorkspaceView, error) {
	if h.workspaces == nil || ar == nil {
		return nil, nil
	}
	ws, svcErr := h.workspaces(ctx, ar.ID)
	if svcErr != nil {
		return nil, status.Errorf(codes.Internal, "resolve workspace for agent runtime %s: %v", ar.ID, svcErr)
	}
	return ws, nil
}

// resolveSecretSources returns the SecretSources owned by the runtime for
// embedding in its gRPC payload. When the lookup seam is unavailable (the
// secretSources plugin is absent) it returns nil so the stream still delivers the
// runtime; the reconciler then provisions no ExternalSecrets for it rather than the
// watch silently breaking.
func (h *agentRuntimeGRPCHandler) resolveSecretSources(ctx context.Context, ar *AgentRuntime) ([]SecretSourceView, error) {
	if h.secretSources == nil || ar == nil {
		return nil, nil
	}
	sources, svcErr := h.secretSources(ctx, ar.ID)
	if svcErr != nil {
		return nil, status.Errorf(codes.Internal, "resolve secret sources for agent runtime %s: %v", ar.ID, svcErr)
	}
	return sources, nil
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

	// repositoryFilter, when set, restricts the result to the AgentRuntimes owned by
	// that Repository. It is the declarative import reconciler's Repository-scoped
	// read; the cluster caller binding exemption that lets this read span clusters is
	// decided in pkg/rbac (import-bound + repository_id filter), not here.
	repositoryFilter := req.GetRepositoryId()

	resp := &pb.ListAgentRuntimesResponse{}
	for _, ar := range runtimes {
		if ar == nil {
			continue
		}
		if repositoryFilter != "" && (ar.RepositoryId == nil || *ar.RepositoryId != repositoryFilter) {
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
		sources, srcErr := h.resolveSecretSources(ctx, ar)
		if srcErr != nil {
			return nil, srcErr
		}
		ws, wsErr := h.resolveWorkspace(ctx, ar)
		if wsErr != nil {
			return nil, wsErr
		}
		resp.Items = append(resp.Items, agentRuntimeToProto(ar, sources, ws))
	}
	return resp, nil
}

// UpdateAgentRuntimeStatus is the control-plane status write-back path. The
// control plane owns the AgentRuntime status lifecycle, so this persists the new
// status and status_message and republishes the runtime (firing a watch event) so
// the value is observable by the console and CLI. See data-model.spec.md
// "AgentRuntime Status Lifecycle".
func (h *agentRuntimeGRPCHandler) UpdateAgentRuntimeStatus(ctx context.Context, req *pb.UpdateAgentRuntimeStatusRequest) (*pb.AgentRuntime, error) {
	if err := grpcutil.ValidateStringField("id", req.GetId(), true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("status", req.GetStatus(), true); err != nil {
		return nil, err
	}
	ar, svcErr := h.service.UpdateStatus(ctx, req.GetId(), req.GetStatus(), req.StatusMessage)
	if svcErr != nil {
		if svcErr.Is404() {
			return nil, status.Errorf(codes.NotFound, "agent runtime %s not found", req.GetId())
		}
		return nil, status.Errorf(codes.Internal, "update agent runtime %s status: %v", req.GetId(), svcErr)
	}
	sources, srcErr := h.resolveSecretSources(ctx, ar)
	if srcErr != nil {
		return nil, srcErr
	}
	ws, wsErr := h.resolveWorkspace(ctx, ar)
	if wsErr != nil {
		return nil, wsErr
	}
	return agentRuntimeToProto(ar, sources, ws), nil
}

// CreateAgentRuntime is the declarative-import create path. The import ownership
// mark (repository_id) is set server-side from the x-hypershell-import-repository-id
// binding and a client-supplied value is ignored, so ownership cannot be forged
// (rbac-enforcement.spec.md). The gRPC authorization layer has already confirmed
// the caller is the control-plane identity reconciling a real Repository.
func (h *agentRuntimeGRPCHandler) CreateAgentRuntime(ctx context.Context, req *pb.CreateAgentRuntimeRequest) (*pb.CreateAgentRuntimeResponse, error) {
	if err := grpcutil.ValidateStringField("name", req.GetName(), true); err != nil {
		return nil, err
	}
	if err := grpcutil.ValidateStringField("gateway_id", req.GetGatewayId(), true); err != nil {
		return nil, err
	}
	ar := &AgentRuntime{
		Name:                req.GetName(),
		GatewayId:           req.GetGatewayId(),
		SandboxTemplateId:   req.GetSandboxTemplateId(),
		ClusterId:           req.GetClusterId(),
		Selector:            req.GetSelector(),
		Description:         req.Description,
		Cron:                req.Cron,
		CoordinatorImage:    req.CoordinatorImage,
		ConcurrencyPolicy:   req.ConcurrencyPolicy,
		LoginRefreshSeconds: req.LoginRefreshSeconds,
		Parameters:          req.Parameters,
		RepositoryId:        pkgrbac.ImportOwnershipMark(ctx), // import ownership set server-side; a client-supplied repository_id is ignored
	}
	created, svcErr := h.service.Create(ctx, ar)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	sources, srcErr := h.resolveSecretSources(ctx, created)
	if srcErr != nil {
		return nil, srcErr
	}
	ws, wsErr := h.resolveWorkspace(ctx, created)
	if wsErr != nil {
		return nil, wsErr
	}
	return &pb.CreateAgentRuntimeResponse{AgentRuntime: agentRuntimeToProto(created, sources, ws)}, nil
}

// UpdateAgentRuntime replaces the mutable spec of an AgentRuntime. In an import
// context it refuses a write to a runtime owned by a different Repository (or to an
// unowned one) via EnforceImportOwnership; outside an import context that check is a
// no-op, so this cannot regress a direct REST/hsctl write. status and status_message
// are control-plane-owned and are deliberately not settable here. The reconciler
// sends the full desired spec, so selector is replaced wholesale.
func (h *agentRuntimeGRPCHandler) UpdateAgentRuntime(ctx context.Context, req *pb.UpdateAgentRuntimeRequest) (*pb.UpdateAgentRuntimeResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.GetId()); err != nil {
		return nil, err
	}
	found, svcErr := h.service.Get(ctx, req.GetId())
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if err := pkgrbac.EnforceImportOwnership(ctx, found.RepositoryId); err != nil {
		return nil, err
	}
	if req.Name != nil {
		found.Name = *req.Name
	}
	if req.GatewayId != nil {
		found.GatewayId = *req.GatewayId
	}
	if req.SandboxTemplateId != nil {
		found.SandboxTemplateId = *req.SandboxTemplateId
	}
	if req.ClusterId != nil {
		found.ClusterId = *req.ClusterId
	}
	found.Selector = req.GetSelector()
	if req.Description != nil {
		found.Description = req.Description
	}
	if req.Cron != nil {
		found.Cron = req.Cron
	}
	if req.CoordinatorImage != nil {
		found.CoordinatorImage = req.CoordinatorImage
	}
	if req.ConcurrencyPolicy != nil {
		found.ConcurrencyPolicy = req.ConcurrencyPolicy
	}
	if req.LoginRefreshSeconds != nil {
		found.LoginRefreshSeconds = req.LoginRefreshSeconds
	}
	if req.Parameters != nil {
		found.Parameters = req.Parameters
	}
	updated, svcErr := h.service.Replace(ctx, found)
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	sources, srcErr := h.resolveSecretSources(ctx, updated)
	if srcErr != nil {
		return nil, srcErr
	}
	ws, wsErr := h.resolveWorkspace(ctx, updated)
	if wsErr != nil {
		return nil, wsErr
	}
	return &pb.UpdateAgentRuntimeResponse{AgentRuntime: agentRuntimeToProto(updated, sources, ws)}, nil
}

// DeleteAgentRuntime removes an AgentRuntime. It loads the runtime first so a
// missing row is a clean NotFound (nil-found guard) and so import ownership can be
// enforced: in an import context a runtime owned by another Repository (or unowned)
// is refused, so one Repository's prune cannot delete another's resource.
func (h *agentRuntimeGRPCHandler) DeleteAgentRuntime(ctx context.Context, req *pb.DeleteAgentRuntimeRequest) (*pb.DeleteAgentRuntimeResponse, error) {
	if err := grpcutil.ValidateRequiredID(req.GetId()); err != nil {
		return nil, err
	}
	found, svcErr := h.service.Get(ctx, req.GetId())
	if svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	if found == nil {
		return nil, status.Errorf(codes.NotFound, "agent runtime %s not found", req.GetId())
	}
	if err := pkgrbac.EnforceImportOwnership(ctx, found.RepositoryId); err != nil {
		return nil, err
	}
	if svcErr := h.service.Delete(ctx, req.GetId()); svcErr != nil {
		return nil, grpcutil.ServiceErrorToGRPC(svcErr)
	}
	return &pb.DeleteAgentRuntimeResponse{}, nil
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

			sources, srcErr := h.resolveSecretSources(ctx, ar)
			if srcErr != nil {
				glog.Warningf("WatchAgentRuntimes: failed to resolve secret sources for agent runtime %s: %v", ar.ID, srcErr)
				return srcErr
			}
			ws, wsErr := h.resolveWorkspace(ctx, ar)
			if wsErr != nil {
				glog.Warningf("WatchAgentRuntimes: failed to resolve workspace for agent runtime %s: %v", ar.ID, wsErr)
				return wsErr
			}
			watchEvent := &pb.WatchAgentRuntimesResponse{
				Type:         pb.EventType(grpcutil.APIEventTypeToProto(evt.EventType)),
				ResourceId:   ar.ID,
				AgentRuntime: agentRuntimeToProto(ar, sources, ws),
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
		sources, srcErr := h.resolveSecretSources(ctx, ar)
		if srcErr != nil {
			return srcErr
		}
		ws, wsErr := h.resolveWorkspace(ctx, ar)
		if wsErr != nil {
			return wsErr
		}
		if err := stream.Send(&pb.WatchAgentRuntimesResponse{
			Type:         pb.EventType_EVENT_TYPE_UPDATED,
			ResourceId:   ar.ID,
			AgentRuntime: agentRuntimeToProto(ar, sources, ws),
		}); err != nil {
			return status.Errorf(codes.Unavailable, "failed to replay agent runtime %s: %v", ar.ID, err)
		}
		replayed++
	}
	glog.V(4).Infof("WatchAgentRuntimes: replayed %d agent runtimes", replayed)
	return nil
}
