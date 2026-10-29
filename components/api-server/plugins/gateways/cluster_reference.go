package gateways

import (
	"context"
	"slices"
	"strings"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// PlacementResolver resolves a placement intent to a concrete managed-cluster id.
// It returns resolved=true with the selected cluster id when an eligible cluster
// matches. For a capability-label selector that matches no eligible cluster it
// returns resolved=false with a nil error, so the caller can late-bind the gateway
// (create it Pending and awaiting placement) rather than fail. A local-kind intent
// that cannot be satisfied, and any validation or lookup failure, return a non-nil
// ServiceError (see gateway-provision-placement.spec.md WEB-PLC-07 and WEB-PLC-08).
type PlacementResolver func(context.Context, openapi.GatewayPlacementIntent) (clusterID string, resolved bool, svcErr *errors.ServiceError)
type PlacementAvailabilityResolver func(context.Context, [][]string) (openapi.GatewayPlacementAvailability, *errors.ServiceError)

func validatePlacementIntent(intent *openapi.GatewayPlacementIntent) *errors.ServiceError {
	mode := ""
	selector := intent.Selector
	excludedSelector := intent.ExcludedSelector
	if intent.Mode != nil {
		mode = *intent.Mode
	}
	if mode != "" {
		if mode != "local-kind" || selector != nil || excludedSelector != nil {
			return errors.Validation("placement mode must be local-kind and cannot include selector or excludedSelector")
		}
		return nil
	}
	if selector != nil {
		normalized, svcErr := managedClusters.NormalizeSelector(selector)
		if svcErr != nil {
			return svcErr
		}
		intent.Selector = normalized
		if excludedSelector != nil {
			normalized, svcErr = managedClusters.NormalizeSelector(excludedSelector)
			if svcErr != nil {
				return svcErr
			}
			intent.ExcludedSelector = normalized
		}
		return nil
	}
	return errors.Validation("placement must include selector or mode")
}

// authorizeIdentityPlacement enforces "Identity selection is platform-admin-only"
// (data-model.spec.md PLAT-DATA-02, managed-cluster-registration.spec.md
// PLAT-CLUS-14): a placement selector (or excludedSelector) that references a
// cluster's reserved managed-cluster/<name> identity capability resolves
// deterministically to exactly that cluster, but only for a caller holding
// platform:admin. Any other caller is rejected with 403 and the create does not
// resolve. Gating the selector's use -- not merely hiding the capability value --
// is the authoritative control: a leaked identity capability stays unusable to a
// non-admin caller. Requirement-only selectors (provider/*, network/*, ...) are
// unaffected and keep resolving by random selection among eligible clusters.
func authorizeIdentityPlacement(ctx context.Context, intent openapi.GatewayPlacementIntent) *errors.ServiceError {
	return requireAdminForIdentitySelectors(ctx, intent.Selector, intent.ExcludedSelector)
}

// authorizeIdentityAvailability applies the same platform:admin gate to the
// placement-availability probe (POST .../placement_availability). Without it a
// non-admin could submit a managed-cluster/<name> selector and learn from the
// available flag whether a specifically named cluster exists and is eligible,
// enumerating the fleet's identity capabilities by probe -- exactly what
// PLAT-DATA-02 / PLAT-CLUS-14 forbid. Each entry in selectors is one capability
// group evaluated independently, mirroring GatewayPlacementIntent.Selector.
func authorizeIdentityAvailability(ctx context.Context, selectors [][]string) *errors.ServiceError {
	return requireAdminForIdentitySelectors(ctx, selectors...)
}

// requireAdminForIdentitySelectors rejects a caller lacking platform:admin when
// any of the given capability groups references a reserved managed-cluster/<name>
// identity capability. Gating the selector's use -- not merely hiding the
// capability value -- is the authoritative control: a leaked identity capability
// stays unusable to a non-admin caller. Requirement-only groups (provider/*,
// network/*, ...) are unaffected.
func requireAdminForIdentitySelectors(ctx context.Context, groups ...[]string) *errors.ServiceError {
	if !slices.ContainsFunc(groups, managedClusters.SelectorReferencesIdentity) {
		return nil
	}
	if !rbac.HasPlatformAdminRole(ctx, rbac.GetUserIDFromContext(ctx)) {
		return errors.Forbidden(
			"placement by a %s/<name> identity capability requires platform:admin; normal callers place by requirement capabilities (provider/*, network/*, ...) and cannot target a specific cluster by identity",
			managedClusters.IdentityCapabilityNamespace,
		)
	}
	return nil
}

// RegisteredClusterLookup reports whether id names a ManagedCluster that a
// control plane registered (non-empty oidc_subject). It returns (false, nil) for
// an unknown id or a manually created record, and an error only when the lookup
// itself fails.
type RegisteredClusterLookup func(ctx context.Context, id string) (bool, *errors.ServiceError)

// validateClusterReference enforces "Gateways Reference a Registered Cluster"
// (managed-cluster-registration.spec.md): every control plane filters its
// watches by its own registered cluster id, so a gateway whose cluster_id is
// empty, unknown, or a manual placeholder would never be reconciled by anyone.
// Rejecting it at write time turns a silent orphan into an actionable 400.
//
// A nil lookup fails closed (500) rather than letting an unvalidated reference
// through.
func validateClusterReference(ctx context.Context, lookup RegisteredClusterLookup, clusterID string) *errors.ServiceError {
	if strings.TrimSpace(clusterID) == "" {
		return errors.Validation("cluster_id is required: a gateway must reference a registered ManagedCluster (discover it by name from GET /api/hypershell/v1/managed_clusters)")
	}
	if lookup == nil {
		return errors.GeneralError("managed cluster registry is not available to validate cluster_id")
	}
	registered, svcErr := lookup(ctx, clusterID)
	if svcErr != nil {
		return svcErr
	}
	if !registered {
		return errors.Validation("cluster_id %q does not reference a ManagedCluster with a registered control plane; gateways assigned to it would never be reconciled", clusterID)
	}
	return nil
}
