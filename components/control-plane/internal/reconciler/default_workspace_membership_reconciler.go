package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	"google.golang.org/grpc"
)

// DefaultWorkspaceMembershipReconciler reconciles each gateway's live
// DefaultWorkspaceMembership rows (data-model.spec.md, PLAT-DATA-27) against
// that gateway's OpenShell "default" workspace membership (CTRL-CORE-20).
//
// It never calls EnsureWorkspace or any workspace-create operation: OpenShell
// already creates and protects the "default" workspace at gateway startup and
// rejects deleting it, so this reconciler only ever adds or removes members.
//
// Full desired-state convergence, not partial: on each pass it loads ALL live
// DefaultWorkspaceMembership rows for the gateway and converges OpenShell's
// actual "default" workspace membership to exactly that set -- a membership
// added directly in OpenShell outside HyperShell is removed on the next pass.
//
// It is keyed and dispatched per gateway (see watcher.WatchDefaultWorkspaceMemberships),
// which gives it the batching/coalescing CTRL-CORE-20 requires for free: the
// reconcile queue already serializes and coalesces work per key, so multiple
// membership events for the same gateway arriving close together collapse into
// one Handle call against the latest desired set.
type DefaultWorkspaceMembershipReconciler struct {
	grpcConn  *grpc.ClientConn
	clusterID string
	// session opens a short-lived OpenShell API session for a gateway. It is nil
	// until a live OpenShell API session factory is wired (tracked alongside the
	// AgentRuntime/WorkspaceMembership realization in cmd/hypershell-controller;
	// see gateway.OpenShellSessionFactory). Handle no-ops, logging at debug, while
	// it is nil, the same "not yet active" posture the AgentRuntime provider
	// realizer takes.
	session gateway.OpenShellSessionFactory
}

// NewDefaultWorkspaceMembershipReconciler builds the reconciler. session may be
// nil; see the session field doc.
func NewDefaultWorkspaceMembershipReconciler(grpcConn *grpc.ClientConn, clusterID string, session gateway.OpenShellSessionFactory) *DefaultWorkspaceMembershipReconciler {
	return &DefaultWorkspaceMembershipReconciler{grpcConn: grpcConn, clusterID: clusterID, session: session}
}

// defaultWorkspaceName is OpenShell's reserved, always-present workspace every
// gateway's `ensure_default_workspace` creates at startup.
const defaultWorkspaceName = "default"

// defaultWorkspaceReconcileFailedReason is the stable, credential-free
// last_reconcile_error value stored when a reconcile pass fails
// (data-model.spec.md, PLAT-DATA-27: "SHALL NOT contain a raw OpenShell error
// body or any credential"). The real error is logged, never persisted.
const defaultWorkspaceReconcileFailedReason = "openshell_default_workspace_reconcile_failed"

// Handle reconciles ALL live DefaultWorkspaceMembership rows for one gateway.
// event.ResourceID is the gateway id: the watcher keys and coalesces the
// reconcile queue by gateway, not by individual membership row, exactly because
// this reconciler always re-lists and converges the gateway's FULL desired set
// rather than acting on a single row.
func (r *DefaultWorkspaceMembershipReconciler) Handle(ctx context.Context, event watcher.Event[*pb.DefaultWorkspaceMembership]) error {
	gatewayID := event.ResourceID
	if gatewayID == "" {
		slog.WarnContext(ctx, "Default workspace membership event has no gateway id, skipping")
		return nil
	}

	if r.session == nil {
		slog.DebugContext(ctx, "OpenShell session factory not configured, skipping default workspace membership reconcile", "gateway_id", gatewayID)
		return nil
	}

	ctx, endSpan := cpotel.StartReconcileSpan(ctx, "DefaultWorkspaceMembership", event.Type.String(), "")
	var reconcileErr error
	defer func() {
		outcome, reason := cpotel.ClassifyReconcileOutcome(reconcileErr)
		endSpan(outcome, reason, reconcileErr)
	}()

	clusterFilter, err := watcher.ClusterFilter(r.clusterID)
	if err != nil {
		reconcileErr = err
		return reconcileErr
	}
	client := pb.NewDefaultWorkspaceMembershipServiceClient(r.grpcConn)
	resp, err := client.ListDefaultWorkspaceMemberships(ctx, &pb.ListDefaultWorkspaceMembershipsRequest{
		GatewayId: &gatewayID,
		ClusterId: clusterFilter,
	})
	if err != nil {
		reconcileErr = fmt.Errorf("list default workspace memberships for gateway %s: %w", gatewayID, err)
		return reconcileErr
	}

	rows := resp.GetItems()
	specs := make([]gateway.MembershipSpec, 0, len(rows))
	for _, m := range rows {
		if m == nil {
			continue
		}
		// Role is always "user" in version 1 (PLAT-DATA-27.03): automatic
		// membership never grants the OpenShell workspace "admin" role.
		specs = append(specs, gateway.MembershipSpec{Subject: m.GetSubject(), Admin: false})
	}

	api, closeSession, err := r.session(ctx, gatewayID)
	if err != nil {
		reconcileErr = fmt.Errorf("open openshell session for gateway %s: %w", gatewayID, err)
		r.markRows(ctx, rows, false, reconcileErr)
		return reconcileErr
	}
	defer func() {
		if closeErr := closeSession(); closeErr != nil {
			slog.WarnContext(ctx, "Closing openshell session failed", "gateway_id", gatewayID, "error", closeErr)
		}
	}()

	// Full desired-state convergence: ReconcileMemberships adds subjects present
	// in specs but absent from OpenShell and removes OpenShell members absent
	// from specs, including a membership added directly in OpenShell outside
	// HyperShell (CTRL-CORE-20.02).
	if err := api.ReconcileMemberships(ctx, defaultWorkspaceName, specs); err != nil {
		reconcileErr = fmt.Errorf("reconcile default workspace memberships for gateway %s: %w", gatewayID, err)
		r.markRows(ctx, rows, false, reconcileErr)
		return reconcileErr
	}

	r.markRows(ctx, rows, true, nil)
	cpotel.RecordDefaultWorkspaceReconcileSuccess(ctx, r.clusterID, time.Now())
	return nil
}

// markRows reports each row's reconciliation status back to the API server
// (CTRL-CORE-20, "Status") and republishes the OBS-CRM-14 pending/failed
// gauges. A failure here is logged, not returned: it must not mask (or be
// conflated with) the primary reconcile outcome already decided above, and the
// status write-back is itself retried on the next pass through this same
// method the next time the gateway's membership set is reconciled.
func (r *DefaultWorkspaceMembershipReconciler) markRows(ctx context.Context, rows []*pb.DefaultWorkspaceMembership, ok bool, cause error) {
	status := "ready"
	var lastErr *string
	if !ok {
		status = "failed"
		reason := defaultWorkspaceReconcileFailedReason
		lastErr = &reason
		slog.WarnContext(ctx, "Default workspace membership reconcile failed", "error", cause)
	}
	client := pb.NewDefaultWorkspaceMembershipServiceClient(r.grpcConn)
	for _, m := range rows {
		if m == nil {
			continue
		}
		id := m.GetMetadata().GetId()
		if _, err := client.UpdateDefaultWorkspaceMembershipStatus(ctx, &pb.UpdateDefaultWorkspaceMembershipStatusRequest{
			Id:                   id,
			GatewayId:            m.GetGatewayId(),
			ReconciliationStatus: status,
			LastReconcileError:   lastErr,
		}); err != nil {
			slog.WarnContext(ctx, "Failed to update default workspace membership status", "membership_id", id, "status", status, "error", err)
			continue
		}
		cpotel.RecordDefaultWorkspaceMembershipState(ctx, r.clusterID, id, status, false)
	}
}
