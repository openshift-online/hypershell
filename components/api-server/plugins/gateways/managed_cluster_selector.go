package gateways

import (
	"regexp"
	"sort"
	"strings"

	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// The managed-cluster/<name> identity selector (PLAT-DATA-31) lets a platform admin
// target ONE specific ManagedCluster by its unique name through the existing
// placement selector surface. It is a selector-layer token, NOT a capability: its
// namespace is deliberately disjoint from the self-declared capability namespaces
// (provider, network, region, runtime, environment, placement), so a label in it is
// unambiguously the identity-targeting token and never a stored, advertised
// capability. Using it is gated to gateway:cluster-targeting (platform:admin includes
// it) at both create boundaries and the placement-availability probe (SEC-RBAC-33);
// resolving it (by name) is fail-fast
// like a pinned cluster_id rather than late-binding like a capability selector.
const managedClusterSelectorNamespace = "managed-cluster"

// managedClusterSelectorPrefix is the "managed-cluster/" token prefix a placement
// selector label carries to name a specific cluster.
const managedClusterSelectorPrefix = managedClusterSelectorNamespace + "/"

// managedClusterSelectorNameRE matches a lowercase DNS-label cluster name, aligned
// with the capability-value segment rule the server and CLI already enforce.
var managedClusterSelectorNameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

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

// resolveManagedClusterIdentitySelection applies PLAT-DATA-31 deterministic
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
