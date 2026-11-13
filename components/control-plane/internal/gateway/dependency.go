package gateway

import (
	"fmt"
	"sync"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/discovery"
)

// DependencyState is the three-state result of evaluating a feature dependency
// through the shared, refreshable dependency discovery (CTRL-CORE-18). A failure
// to discover one group never implies that an unrelated group is absent: each
// dependency is evaluated for its own group/version/resource, so a stale or
// unreachable neighbor (for example projectcalico.org/v3) leaves every other
// dependency's state untouched.
type DependencyState int

const (
	// DependencyUnknown means a transport, authorization, or stale-group discovery
	// error prevented a current determination. Affected work is waiting/retryable
	// with bounded backoff, not treated as absent.
	DependencyUnknown DependencyState = iota
	// DependencyPresent means successful discovery confirms the required resource
	// exists.
	DependencyPresent
	// DependencyAbsent means successful discovery confirms the required resource is
	// absent: a prerequisite the operator must install.
	DependencyAbsent
)

func (s DependencyState) String() string {
	switch s {
	case DependencyPresent:
		return "present"
	case DependencyAbsent:
		return "confirmed-absent"
	default:
		return "unknown/unavailable"
	}
}

// DependencyResult is the outcome of evaluating a single dependency.
type DependencyResult struct {
	// State is the current three-state determination.
	State DependencyState
	// Err carries the discovery error when State is DependencyUnknown; nil
	// otherwise.
	Err error
	// LastKnown is the most recent successfully determined state (Present or
	// Absent) for this dependency, or DependencyUnknown if none has been observed.
	// It is populated only when State is DependencyUnknown, so a transient
	// discovery failure does not erase a prior successful result (CTRL-CORE-18).
	LastKnown DependencyState
}

// DependencyRef names a specific group/version and the resource kind a feature
// requires within it.
type DependencyRef struct {
	// GroupVersion is the discovery group/version, e.g. "cert-manager.io/v1".
	GroupVersion string
	// Kind is the resource kind the feature needs, e.g. "Certificate" or
	// "GRPCRoute". Empty matches any served resource in the group/version.
	Kind string
}

func (r DependencyRef) String() string {
	if r.Kind == "" {
		return r.GroupVersion
	}
	return r.GroupVersion + " " + r.Kind
}

// Well-known dependency references used across the reconcilers so every feature
// checks the same group/version/resource (CTRL-CORE-18).
var (
	// DependencyCertManager is the cert-manager Certificate CRD, the prerequisite
	// for all TLS certificate lifecycle work (OSG-TLS-01).
	DependencyCertManager = DependencyRef{GroupVersion: "cert-manager.io/v1", Kind: "Certificate"}
	// DependencyGatewayAPI is the Gateway API GRPCRoute, the prerequisite for
	// tenant ingress (OSG-ROUT-02, CTRL-CORE-19).
	DependencyGatewayAPI = DependencyRef{GroupVersion: "gateway.networking.k8s.io/v1", Kind: "GRPCRoute"}
)

// DependencyDiscovery is the shared, refreshable dependency-discovery contract
// (CTRL-CORE-18). It evaluates each required group/version/resource on demand
// and retains the last successfully determined state per dependency so a
// transient failure reports unknown/unavailable without discarding a prior
// present/absent result. Evaluations are per-dependency, so a partial discovery
// result preserves successfully resolved groups while recording errors for the
// ones that failed. It is safe for concurrent use.
type DependencyDiscovery struct {
	disc discovery.DiscoveryInterface

	mu       sync.Mutex
	lastGood map[string]DependencyState
}

// NewDependencyDiscovery builds a dependency discovery backed by the given
// discovery client. The gateway reconciler holds one instance for the controller
// lifetime so every gateway provisioning pass refreshes feature prerequisites
// through the same last-known-good cache (CTRL-CORE-18). The type is deliberately
// reconciler-agnostic: a single instance can be threaded through the exposure,
// health, and managed-cluster reconcilers as they adopt the discovery contract so
// they observe a consistent dependency state.
func NewDependencyDiscovery(disc discovery.DiscoveryInterface) *DependencyDiscovery {
	return &DependencyDiscovery{
		disc:     disc,
		lastGood: make(map[string]DependencyState),
	}
}

// Evaluate refreshes and returns the current state of a dependency. A successful
// discovery updates the retained last-known-good state; an unknown/unavailable
// result reports the retained state in LastKnown instead of overwriting it.
func (d *DependencyDiscovery) Evaluate(ref DependencyRef) DependencyResult {
	res := evaluateDependency(d.disc, ref)

	d.mu.Lock()
	defer d.mu.Unlock()
	if res.State == DependencyUnknown {
		res.LastKnown = d.lastGood[ref.GroupVersion+"/"+ref.Kind]
		return res
	}
	d.lastGood[ref.GroupVersion+"/"+ref.Kind] = res.State
	return res
}

// evaluateDependency performs one discovery call for a single group/version and
// classifies the outcome. Querying the specific group/version (rather than the
// whole server) isolates the result from unrelated groups: a stale or
// unreachable neighbor does not appear here at all. A NotFound response is the
// only signal strong enough to confirm absence; every other error is
// unknown/unavailable and therefore retryable.
func evaluateDependency(disc discovery.DiscoveryInterface, ref DependencyRef) DependencyResult {
	resources, err := disc.ServerResourcesForGroupVersion(ref.GroupVersion)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return DependencyResult{State: DependencyAbsent}
		}
		return DependencyResult{State: DependencyUnknown, Err: fmt.Errorf("discover %s: %w", ref, err)}
	}
	if ref.Kind == "" {
		return DependencyResult{State: DependencyPresent}
	}
	for _, r := range resources.APIResources {
		if r.Kind == ref.Kind {
			return DependencyResult{State: DependencyPresent}
		}
	}
	// The group/version is served but does not carry the required kind: the
	// resource is confirmed absent.
	return DependencyResult{State: DependencyAbsent}
}
