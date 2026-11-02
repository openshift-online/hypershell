package gateways

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
)

// resolvePendingPlacements re-resolves every late-bound gateway that is still
// awaiting placement (empty cluster_id with a persisted capability-label selector)
// and assigns a cluster_id to each one whose selector now matches an eligible
// ManagedCluster. It is the late-binding half of "Gateway Placement and Late
// Binding" (data-model.spec.md): a declarative gateway that imported before any
// matching cluster was connected binds as soon as one appears or gains the
// capability. It is invoked from the ManagedCluster registration controller, so it
// runs when the eligible-cluster set grows.
//
// A per-gateway resolution or assignment failure is logged and skipped rather than
// returned, so one unresolvable gateway neither aborts the sweep nor gates the
// ManagedCluster event that triggered it (which would re-run every handler on
// requeue); the next registration or capability change retries it. Only a
// whole-sweep lookup failure is returned, so the event requeues and the transient
// failure is retried.
func resolvePendingPlacements(ctx context.Context, svc GatewayService, resolve PlacementResolver) error {
	if svc == nil || resolve == nil {
		return nil
	}
	// This sweep assigns cluster_id (and clears the awaiting-placement status) as an
	// internal API-server write, so it is a control-plane-class write for the gateway
	// field policy: without this marker the default user kind would reject the cluster_id
	// change (PLAT-DATA-23). It is the API server's own late-binding logic, not an
	// externally-authored caller.
	ctx = rbac.WithGatewayWriterKind(ctx, rbac.GatewayWriterControlPlane)
	gateways, svcErr := svc.FindUnplaced(ctx)
	if svcErr != nil {
		return fmt.Errorf("list unplaced gateways: %w", svcErr)
	}
	for _, gateway := range gateways {
		intent, ok := storedPlacementToIntent(gateway.Placement)
		if !ok {
			continue
		}
		// Late binding applies only to capability-label selectors. A local-kind
		// placement resolves or hard-errors at create (WEB-PLC-08) and must not be retried
		// here, where it would error on every sweep.
		if intent.Mode != nil {
			continue
		}
		clusterID, resolved, resolveErr := resolve(ctx, intent)
		if resolveErr != nil {
			slog.WarnContext(ctx, "late-binding: resolving placement failed, will retry on the next cluster change", "gateway_id", gateway.ID, "error", resolveErr)
			continue
		}
		if !resolved {
			continue
		}
		gateway.ClusterId = clusterID
		// Clear the awaiting-placement status now that the gateway is placed; the
		// control plane sets operational health once it begins provisioning. The
		// phase stays Pending (it is now actionable), and Replace advances the
		// generation because cluster_id changed.
		gateway.Status = nil
		if _, replaceErr := svc.Replace(ctx, gateway); replaceErr != nil {
			slog.WarnContext(ctx, "late-binding: assigning cluster failed, will retry on the next cluster change", "cluster_id", clusterID, "gateway_id", gateway.ID, "error", replaceErr)
			continue
		}
		slog.InfoContext(ctx, "late-bound gateway placed on cluster", "gateway_id", gateway.ID, "cluster_id", clusterID)
	}
	return nil
}
