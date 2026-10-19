package gateways

import (
	"context"
	stderrors "errors"
	"fmt"

	"github.com/golang/glog"
)

// resolvePendingPlacements re-resolves every late-bound gateway that is still
// awaiting placement (empty cluster_id with a persisted placement selector) and
// assigns a cluster_id to each one whose selector now matches an eligible
// ManagedCluster. It is the late-binding half of "Gateway Placement and Late
// Binding" (data-model.spec.md): a declarative gateway that imported before any
// matching cluster was connected binds as soon as one appears. It is invoked from
// the ManagedCluster registration controller, so it runs when the eligible-cluster
// set grows.
//
// A resolution or assignment failure for one gateway is collected and does not
// abort the sweep, so one unresolvable gateway cannot block binding the rest. A
// selector that still matches nothing leaves the gateway Pending for a later pass.
func resolvePendingPlacements(ctx context.Context, svc GatewayService, resolve PlacementResolver) error {
	if svc == nil || resolve == nil {
		return nil
	}
	gateways, svcErr := svc.FindUnplaced(ctx)
	if svcErr != nil {
		return fmt.Errorf("list unplaced gateways: %s", svcErr)
	}
	var errs []error
	for _, gateway := range gateways {
		intent, ok := storedPlacementToIntent(gateway.Placement)
		if !ok {
			continue
		}
		clusterID, resolved, resolveErr := resolve(ctx, intent)
		if resolveErr != nil {
			errs = append(errs, fmt.Errorf("resolve placement for gateway %s: %s", gateway.ID, resolveErr))
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
			errs = append(errs, fmt.Errorf("assign cluster %s to gateway %s: %s", clusterID, gateway.ID, replaceErr))
			continue
		}
		glog.Infof("late-bound gateway %s placed on cluster %s", gateway.ID, clusterID)
	}
	return stderrors.Join(errs...)
}
