package gateways

import (
	"context"
	"strings"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

type PlacementResolver func(context.Context, openapi.GatewayPlacementIntent) (string, *errors.ServiceError)
type PlacementAvailabilityResolver func(context.Context) (openapi.GatewayPlacementAvailability, *errors.ServiceError)

func validatePlacementIntent(intent openapi.GatewayPlacementIntent) *errors.ServiceError {
	mode, network, provider := "", "", ""
	if intent.Mode != nil {
		mode = *intent.Mode
	}
	if intent.Network != nil {
		network = *intent.Network
	}
	if intent.Provider != nil {
		provider = *intent.Provider
	}
	if mode != "" {
		if mode != "local-kind" || network != "" || provider != "" {
			return errors.Validation("placement mode must be local-kind and cannot include network or provider")
		}
		return nil
	}
	if network != "public" && network != "vpn" {
		return errors.Validation("placement.network must be public or vpn")
	}
	if provider != "aws" && provider != "ibm" {
		return errors.Validation("placement.provider must be aws or ibm")
	}
	if network == "vpn" && provider != "aws" {
		return errors.Validation("VPN placement requires provider aws")
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
