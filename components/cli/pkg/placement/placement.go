// Package placement builds and validates the gateway placement intent the
// hsctl create gateway flags submit. The rules mirror the API server's
// ManagedCluster capability-label validation
// (components/api-server/plugins/managedClusters, NormalizeSelector), which
// stays authoritative: these client-side checks exist only to fail fast with
// a CLI-shaped message instead of a server 400. See
// specs/platform/hsctl-placement.spec.md (CLI-PLC-01) and
// specs/web-console/gateway-provision-placement.spec.md (WEB-PLC-01/07) for
// the contract.
package placement

import (
	"fmt"
	"regexp"
	"strings"
)

// capabilitySegmentRE matches one lowercase DNS-label segment, identical in
// intent to the API server's regexp (managedClusters capabilitySegmentRE).
var capabilitySegmentRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// capabilityNamespaces is the closed set of capability-label namespaces a
// managed cluster may self-declare and a placement selector may require.
var capabilityNamespaces = map[string]bool{
	"provider":    true,
	"network":     true,
	"region":      true,
	"runtime":     true,
	"environment": true,
}

const repeatUsageHint = "repeat the flag to combine labels (e.g. --placement-selector network/public --placement-selector provider/ibm)"

// ValidateLabel reports whether one flag value is a single well-formed
// capability label. It rejects empty values, comma-separated lists (each flag
// use carries exactly one label), and anything outside the
// namespace/value vocabulary (CLI-PLC-01.05, CLI-PLC-01.07).
func ValidateLabel(label string) error {
	if label == "" {
		return fmt.Errorf("capability label must not be empty; %s", repeatUsageHint)
	}
	if strings.Contains(label, ",") {
		return fmt.Errorf("capability label %q must not contain a comma: each flag use takes one label; %s", label, repeatUsageHint)
	}
	if strings.ContainsAny(label, " \t\n") {
		return fmt.Errorf("capability label %q must not contain whitespace; %s", label, repeatUsageHint)
	}
	parts := strings.Split(label, "/")
	if len(parts) != 2 {
		return fmt.Errorf("capability label %q must be namespace/value (e.g. network/public)", label)
	}
	if !capabilityNamespaces[parts[0]] {
		return fmt.Errorf("capability label %q uses unknown namespace %q: recognized namespaces are provider, network, region, runtime, environment", label, parts[0])
	}
	for _, part := range parts {
		if !capabilitySegmentRE.MatchString(part) {
			return fmt.Errorf("capability label %q is invalid: each segment must be a lowercase DNS label (letters, digits, hyphens)", label)
		}
	}
	return nil
}

// BuildIntent validates the placement flags and returns the JSON-ready
// placement value for the gateway create request body, or an error when the
// flag combination is invalid (CLI-PLC-01). localKind and any selector flags
// are mutually exclusive; an excluded selector requires a required selector.
// Duplicate labels are passed through: the server collapses them
// (NormalizeSelector). It returns nil when no placement flag was supplied,
// leaving the caller on the pinned (cluster_id) shape.
func BuildIntent(selectors, excludes []string, localKind bool) (map[string]interface{}, error) {
	hasSelector := len(selectors) > 0
	hasExclude := len(excludes) > 0
	if !hasSelector && !hasExclude && !localKind {
		return nil, nil
	}
	if localKind && (hasSelector || hasExclude) {
		return nil, fmt.Errorf("--local-kind cannot be combined with --placement-selector or --placement-exclude-selector: the placement intent carries either a selector or the local-kind mode")
	}
	if hasExclude && !hasSelector {
		return nil, fmt.Errorf("--placement-exclude-selector requires at least one --placement-selector")
	}
	if localKind {
		return map[string]interface{}{"mode": "local-kind"}, nil
	}
	for _, label := range selectors {
		if err := ValidateLabel(label); err != nil {
			return nil, fmt.Errorf("invalid --placement-selector: %w", err)
		}
	}
	for _, label := range excludes {
		if err := ValidateLabel(label); err != nil {
			return nil, fmt.Errorf("invalid --placement-exclude-selector: %w", err)
		}
	}
	intent := map[string]interface{}{"selector": selectors}
	if hasExclude {
		intent["excludedSelector"] = excludes
	}
	return intent, nil
}

// AvailabilityRequest returns the JSON-ready body for the batched placement
// availability endpoint: the combined required-label list as a single
// selector entry (CLI-PLC-03). Excluded labels are not part of the endpoint's
// contract; the caller prints that note.
func AvailabilityRequest(selectors []string) map[string]interface{} {
	return map[string]interface{}{"selectors": [][]string{selectors}}
}
