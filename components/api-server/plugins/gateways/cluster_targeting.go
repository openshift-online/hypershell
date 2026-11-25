package gateways

import (
	"context"
	"strings"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// A caller chooses a gateway's cluster, rather than letting capability placement
// pick one, in three ways:
//
//  1. pinning it by cluster_id (SEC-RBAC-34);
//  2. the managed-cluster/<name> identity selector (PLAT-DATA-31, SEC-RBAC-33);
//  3. requiring a purpose/ label, a capability a cluster advertises to say what it
//     is set aside for, such as purpose/release-check on a hub (SEC-RBAC-35).
//
// All three need gateway:cluster-targeting; platform:admin includes it. A plain
// gateway:creator places by the other capability namespaces only.

// purposeNamespace is the capability namespace whose labels say what a cluster is
// set aside for. A cluster advertises them like any other capability, but
// REQUIRING one in a placement selector needs gateway:cluster-targeting. Excluding
// one does not: keeping a gateway off such a cluster grants no reach.
const purposeNamespace = "purpose"

const purposePrefix = purposeNamespace + "/"

const ordinaryPlacementHint = "place the gateway with a capability placement selector (provider/, network/, region/, runtime/, environment/) instead"

var (
	pinnedClusterForbiddenReason = "pinning a gateway to a specific cluster by cluster_id requires the " + roles.RoleGatewayClusterTargeting + " role (platform:admin includes it); without it, " + ordinaryPlacementHint

	managedClusterSelectorForbiddenReason = "the managed-cluster/<name> placement selector targets a specific cluster by name and requires the " + roles.RoleGatewayClusterTargeting + " role (platform:admin includes it); without it, " + ordinaryPlacementHint

	purposeLabelForbiddenReason = "requiring a purpose/ label in a placement selector targets clusters set aside for a specific purpose and requires the " + roles.RoleGatewayClusterTargeting + " role (platform:admin includes it); without it, " + ordinaryPlacementHint

	importClusterTargetingForbiddenReason = "an import cannot choose a gateway's cluster: cluster_id, the managed-cluster/<name> selector, and required purpose/ labels need the " + roles.RoleGatewayClusterTargeting + " role, which a Repository import does not hold; declare a capability placement selector in the manifest instead"
)

func isPurposeLabel(label string) bool {
	return strings.HasPrefix(strings.TrimSpace(label), purposePrefix)
}

func selectorContainsPurposeLabel(labels []string) bool {
	for _, label := range labels {
		if isPurposeLabel(label) {
			return true
		}
	}
	return false
}

// placementTargetingReason returns the 403 reason when a placement intent chooses a
// cluster, or "" when it places by ordinary capabilities only. A managed-cluster/
// token is gated in the required AND excluded selector (SEC-RBAC-33); a purpose/
// label only in the required selector (SEC-RBAC-35).
func placementTargetingReason(intent openapi.GatewayPlacementIntent) string {
	if selectorContainsManagedCluster(intent.Selector) || selectorContainsManagedCluster(intent.ExcludedSelector) {
		return managedClusterSelectorForbiddenReason
	}
	if selectorContainsPurposeLabel(intent.Selector) {
		return purposeLabelForbiddenReason
	}
	return ""
}

// availabilityTargetingReason is the probe counterpart: every probed selector is a
// required selector, so either kind of targeting label gates it (SEC-RBAC-33.03).
func availabilityTargetingReason(selectors [][]string) string {
	for _, selector := range selectors {
		if selectorContainsManagedCluster(selector) {
			return managedClusterSelectorForbiddenReason
		}
	}
	for _, selector := range selectors {
		if selectorContainsPurposeLabel(selector) {
			return purposeLabelForbiddenReason
		}
	}
	return ""
}

// clusterTargetingAuthorizer enforces the gateway:cluster-targeting gate at the
// create boundaries, the gRPC cluster_id reassignment, and the placement
// availability probe. The inputs are body-dependent, so the path authorization
// middleware never sees them; the handlers call this authorizer, which mirrors
// dedicatedRouteAuthorizer's fail-closed structure.
type clusterTargetingAuthorizer struct {
	bindingLookup rbac.RoleBindingLookup
	enforceRBAC   bool
}

// newClusterTargetingAuthorizer builds the gate. bindingLookup is the role binding
// service (nil when that plugin is absent, which fails closed) and enforceRBAC
// mirrors the RBAC middleware's enforcement gate (rbac.Enforced).
func newClusterTargetingAuthorizer(bindingLookup rbac.RoleBindingLookup, enforceRBAC bool) clusterTargetingAuthorizer {
	return clusterTargetingAuthorizer{bindingLookup: bindingLookup, enforceRBAC: enforceRBAC}
}

// authorizePin gates a write that pins (or re-pins) a gateway by cluster_id. The
// caller passes pinned=false when no cluster_id is set or when an update re-sends
// the unchanged stored value, which is not a reassignment.
func (a clusterTargetingAuthorizer) authorizePin(ctx context.Context, pinned bool) *errors.ServiceError {
	if !pinned {
		return nil
	}
	return a.authorize(ctx, pinnedClusterForbiddenReason)
}

// authorizePlacement gates a create whose placement selector chooses a cluster,
// before any name or label is resolved.
func (a clusterTargetingAuthorizer) authorizePlacement(ctx context.Context, intent openapi.GatewayPlacementIntent) *errors.ServiceError {
	reason := placementTargetingReason(intent)
	if reason == "" {
		return nil
	}
	return a.authorize(ctx, reason)
}

// authorizeAvailability gates a placement-availability probe under the same rule.
func (a clusterTargetingAuthorizer) authorizeAvailability(ctx context.Context, selectors [][]string) *errors.ServiceError {
	reason := availabilityTargetingReason(selectors)
	if reason == "" {
		return nil
	}
	return a.authorize(ctx, reason)
}

// authorize is the shared decision for a request that chooses a cluster.
//
//   - A control-plane write is privileged and bypasses it (for example the API
//     server's own late-binding rebind).
//   - An import write is always denied: a Repository import holds no roles, so it
//     cannot hold gateway:cluster-targeting.
//   - A user write needs gateway:cluster-targeting or platform:admin. A missing
//     identity, an unwired lookup, or a denial fails closed with 403; a lookup
//     failure is a server error, so a database problem does not tell the caller to
//     request a role it may already hold.
//
// The whole check is a no-op exactly when authentication is off (SEC-RBAC-14).
func (a clusterTargetingAuthorizer) authorize(ctx context.Context, reason string) *errors.ServiceError {
	if !a.enforceRBAC {
		return nil
	}
	switch rbac.GatewayWriterKindFromContext(ctx) {
	case rbac.GatewayWriterControlPlane:
		return nil
	case rbac.GatewayWriterImport:
		return errors.Forbidden("%s", importClusterTargetingForbiddenReason)
	}
	userID := rbac.GetUserIDFromContext(ctx)
	if userID == "" || a.bindingLookup == nil {
		return errors.Forbidden("%s", reason)
	}
	bindings, err := a.bindingLookup.FindBindingsByUserID(ctx, userID)
	if err != nil {
		return errors.GeneralError("failed to look up the role bindings of user %s: %s", userID, err)
	}
	if rbac.CanTargetGatewayCluster(bindings) {
		return nil
	}
	return errors.Forbidden("%s", reason)
}
