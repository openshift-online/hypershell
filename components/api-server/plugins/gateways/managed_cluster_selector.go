package gateways

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// The managed-cluster/<name> identity selector (PLAT-DATA-27) lets a platform admin
// target ONE specific ManagedCluster by its unique name through the existing
// placement selector surface. It is a selector-layer token, NOT a capability: its
// namespace is deliberately disjoint from the self-declared capability namespaces
// (provider, network, region, runtime, environment), so a label in it is
// unambiguously the identity-targeting token and never a stored, advertised
// capability. Using it is gated to platform:admin at both create boundaries and the
// placement-availability probe (SEC-RBAC-27); resolving it (by name) is fail-fast
// like a pinned cluster_id rather than late-binding like a capability selector.
const managedClusterSelectorNamespace = "managed-cluster"

// managedClusterSelectorPrefix is the "managed-cluster/" token prefix a placement
// selector label carries to name a specific cluster.
const managedClusterSelectorPrefix = managedClusterSelectorNamespace + "/"

// managedClusterSelectorNameRE matches a lowercase DNS-label cluster name, aligned
// with the capability-value segment rule the server and CLI already enforce.
var managedClusterSelectorNameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// managedClusterSelectorForbiddenReason is the actionable 403 for a non-admin caller
// that uses the identity selector. It names the gate and the alternative so the
// message guides the caller rather than merely denying.
const managedClusterSelectorForbiddenReason = "the managed-cluster/<name> placement selector targets a specific cluster by name and requires the platform:admin role; a caller without it places by capability selector (provider/, network/, region/, runtime/, environment/) only"

// isManagedClusterSelectorLabel reports whether a raw selector label is a
// managed-cluster/<name> identity token rather than a capability label.
func isManagedClusterSelectorLabel(label string) bool {
	return strings.HasPrefix(label, managedClusterSelectorPrefix)
}

// splitManagedClusterSelector partitions a selector list into the names carried by
// managed-cluster/<name> identity tokens and the remaining capability labels,
// preserving order within each partition. It does not validate; normalization
// validates the names and the capability labels.
func splitManagedClusterSelector(labels []string) (identityNames []string, capabilityLabels []string) {
	for _, label := range labels {
		if isManagedClusterSelectorLabel(label) {
			identityNames = append(identityNames, strings.TrimPrefix(label, managedClusterSelectorPrefix))
			continue
		}
		capabilityLabels = append(capabilityLabels, label)
	}
	return identityNames, capabilityLabels
}

// placementReferencesManagedClusterSelector reports whether a placement intent's
// required OR excluded selector references any managed-cluster/<name> token. The
// gate keys off this on the raw intent, before validation or resolution, so a
// non-admin is denied before the name is resolved (SEC-RBAC-27).
func placementReferencesManagedClusterSelector(intent openapi.GatewayPlacementIntent) bool {
	return selectorContainsManagedCluster(intent.Selector) || selectorContainsManagedCluster(intent.ExcludedSelector)
}

// selectorsReferenceManagedClusterSelector reports whether any probed selector of a
// placement-availability request references a managed-cluster/<name> token, so the
// probe is gated like the create boundaries (SEC-RBAC-27.03).
func selectorsReferenceManagedClusterSelector(selectors [][]string) bool {
	for _, selector := range selectors {
		if selectorContainsManagedCluster(selector) {
			return true
		}
	}
	return false
}

func selectorContainsManagedCluster(labels []string) bool {
	for _, label := range labels {
		if isManagedClusterSelectorLabel(label) {
			return true
		}
	}
	return false
}

// normalizePlacementSelector validates and normalizes a selector list that MAY mix
// managed-cluster/<name> identity tokens with capability labels. Identity tokens are
// validated as lowercase DNS-label names and PRESERVED (so the persisted placement
// records the admin's intent and late-binding re-resolution sees it); capability
// labels are normalized through the authoritative managedClusters vocabulary. The
// combined list is deduplicated and sorted for a stable representation.
func normalizePlacementSelector(labels []string) ([]string, *errors.ServiceError) {
	if len(labels) == 0 {
		return nil, errors.Validation("placement selector must contain at least one label")
	}
	identityNames, capabilityLabels := splitManagedClusterSelector(labels)

	seen := make(map[string]struct{}, len(labels))
	combined := make([]string, 0, len(labels))
	for _, name := range identityNames {
		if !managedClusterSelectorNameRE.MatchString(name) {
			return nil, errors.Validation("invalid managed-cluster selector %q: expected managed-cluster/<name> with a lowercase DNS-label cluster name", managedClusterSelectorPrefix+name)
		}
		token := managedClusterSelectorPrefix + name
		if _, dup := seen[token]; dup {
			continue
		}
		seen[token] = struct{}{}
		combined = append(combined, token)
	}
	if len(capabilityLabels) > 0 {
		normalizedCaps, svcErr := managedClusters.NormalizeSelector(capabilityLabels)
		if svcErr != nil {
			return nil, svcErr
		}
		for _, label := range normalizedCaps {
			if _, dup := seen[label]; dup {
				continue
			}
			seen[label] = struct{}{}
			combined = append(combined, label)
		}
	}
	if len(combined) == 0 {
		return nil, errors.Validation("placement selector must contain at least one label")
	}
	sort.Strings(combined)
	return combined, nil
}

// normalizeSelectorMaybeEmpty normalizes a capability-label list through the
// authoritative managedClusters vocabulary, treating an empty list as a valid empty
// result rather than a 400. An identity selector may carry no capability labels, so
// the resolver must tolerate an empty capability partition.
func normalizeSelectorMaybeEmpty(labels []string) ([]string, *errors.ServiceError) {
	if len(labels) == 0 {
		return nil, nil
	}
	return managedClusters.NormalizeSelector(labels)
}

// managedClusterIdentity is the subset of a ManagedCluster the identity-selector
// resolver needs: its id, its advertised capability set, and whether a control
// plane registered it. It keeps resolveManagedClusterIdentitySelection a pure
// function so the deterministic-resolution rules are unit-testable without a DB.
type managedClusterIdentity struct {
	ID           string
	Capabilities []string
	Registered   bool
}

// resolveManagedClusterIdentitySelection applies PLAT-DATA-27 deterministic
// resolution for a required managed-cluster/<name> token, given the looked-up named
// cluster (nil when no record carries the name). It fails fast rather than
// late-binding: an unknown name is a 400, an unregistered target is a 400 mirroring
// the pinned cluster_id rule (PLAT-CLUS-09), and a named cluster that does not
// satisfy the accompanying capability labels is a 400. On success it returns the
// cluster id to assign as the Gateway's cluster_id.
func resolveManagedClusterIdentitySelection(name string, cluster *managedClusterIdentity, requiredCaps, excludedNames, excludedCaps []string) (string, *errors.ServiceError) {
	if cluster == nil {
		return "", errors.Validation("managed-cluster/%s names no ManagedCluster; discover a cluster by name from GET /api/hypershell/v1/managed_clusters", name)
	}
	if !cluster.Registered {
		return "", errors.Validation("managed-cluster/%s has no registered control plane; a gateway assigned to it would never be reconciled", name)
	}
	for _, excluded := range excludedNames {
		if excluded == name {
			return "", errors.Validation("placement selector both targets and excludes managed-cluster/%s", name)
		}
	}
	if !capabilitySetContains(cluster.Capabilities, requiredCaps) {
		return "", errors.Validation("managed-cluster/%s does not advertise every capability the placement selector requires", name)
	}
	if !capabilitySetExcludes(cluster.Capabilities, excludedCaps) {
		return "", errors.Validation("managed-cluster/%s advertises a capability the placement selector excludes", name)
	}
	return cluster.ID, nil
}

// managedClusterSelectorAuthorizer enforces the platform-admin gate on USE of the
// managed-cluster/<name> identity selector at the create boundaries and the
// placement-availability probe. Like the dedicated_route gate, the selector is
// body-dependent, so the path-based authorization middleware never sees it; the
// handlers call this authorizer, which mirrors dedicatedRouteAuthorizer's structure
// and fail-closed rules (SEC-RBAC-27).
type managedClusterSelectorAuthorizer struct {
	bindingLookup rbac.RoleBindingLookup
	enforceRBAC   bool
}

// newManagedClusterSelectorAuthorizer builds the identity-selector gate. bindingLookup
// is the role binding service (nil when that plugin is absent, which fails closed)
// and enforceRBAC mirrors the RBAC middleware's enforcement gate (rbac.Enforced), so
// the gate is a no-op exactly when request-level authorization is.
func newManagedClusterSelectorAuthorizer(bindingLookup rbac.RoleBindingLookup, enforceRBAC bool) managedClusterSelectorAuthorizer {
	return managedClusterSelectorAuthorizer{bindingLookup: bindingLookup, enforceRBAC: enforceRBAC}
}

// authorizeCreate rejects a create whose placement references a managed-cluster/<name>
// token from a caller without platform:admin, before the name is resolved.
func (a managedClusterSelectorAuthorizer) authorizeCreate(ctx context.Context, intent openapi.GatewayPlacementIntent) *errors.ServiceError {
	return a.authorize(ctx, placementReferencesManagedClusterSelector(intent))
}

// authorizeAvailability rejects a placement-availability probe whose selectors
// reference a managed-cluster/<name> token from a caller without platform:admin,
// under the same gate as the create boundaries (SEC-RBAC-27.03).
func (a managedClusterSelectorAuthorizer) authorizeAvailability(ctx context.Context, selectors [][]string) *errors.ServiceError {
	return a.authorize(ctx, selectorsReferenceManagedClusterSelector(selectors))
}

// authorize is the shared decision. It gates the USE of the token: a request that
// references no managed-cluster/ token is unaffected; otherwise only a user caller
// holding platform:admin is permitted. A missing caller identity, an unwired lookup,
// or a denial fails closed with 403; a lookup failure surfaces as a server error so
// a transient database problem does not tell the caller to request a role it may
// already hold. A privileged control-plane or import write bypasses the user gate: a
// control-plane write is privileged, and an import create carries no managed-cluster/
// token on the user placement surface (SEC-RBAC-27); even if one appeared it would be
// equivalent to the pinned cluster_id an import may already supply, so the bypass
// grants no new reach. The check is a no-op exactly when authentication is off
// (SEC-RBAC-14).
func (a managedClusterSelectorAuthorizer) authorize(ctx context.Context, referenced bool) *errors.ServiceError {
	if !referenced {
		return nil
	}
	if !a.enforceRBAC {
		return nil
	}
	if rbac.GatewayWriterKindFromContext(ctx) != rbac.GatewayWriterUser {
		return nil
	}
	userID := rbac.GetUserIDFromContext(ctx)
	if userID == "" || a.bindingLookup == nil {
		return errors.Forbidden("%s", managedClusterSelectorForbiddenReason)
	}
	bindings, err := a.bindingLookup.FindBindingsByUserID(ctx, userID)
	if err != nil {
		return errors.GeneralError("failed to look up the role bindings of user %s: %s", userID, err)
	}
	if rbac.CanUseManagedClusterSelector(bindings) {
		return nil
	}
	return errors.Forbidden("%s", managedClusterSelectorForbiddenReason)
}
