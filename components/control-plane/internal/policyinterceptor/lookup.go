package policyinterceptor

import (
	"context"
	"errors"

	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
)

// ErrLookupUnsupported reports that the sandbox resources cannot be read, so a
// start or resume re-check is skipped (allowed, logged, and counted).
var ErrLookupUnsupported = errors.New("sandbox resource lookup is not supported")

// SandboxLookup reads the stored resources of an existing sandbox so a
// StartSandbox or ResumeSandbox binding can re-check it against the enforced
// policies (PLAT-PGP-13, PLAT-PGP-16). OpenShell never populates
// validate.current_state (AGT-EXT-06), so the request only names the sandbox and
// its workspace; the stored spec must come from somewhere else.
type SandboxLookup interface {
	// Lookup returns the Cedar context of the named sandbox with fleet defaults
	// applied, or ErrLookupUnsupported (or another error) when it cannot.
	Lookup(ctx context.Context, workspace, name string) (cedar.Context, error)
}

// NoSandboxLookup is the production lookup today. The gateway pod's service
// account can read agents.x-k8s.io Sandbox objects in the managed and operator
// workspace modes, but the mapping from an OpenShell sandbox name and
// workspace to that object (label-or-annotation naming, two Sandbox API
// versions, namespaces shared across gateways on one cluster) is internal to the
// OpenShell Kubernetes driver and not a published contract, and the default
// interceptor timeout (500ms) leaves no room for an uncached API read. Until
// OpenShell populates validate.current_state or publishes that mapping, the
// re-check allows the call and reports the skip (a recorded gap, not invented
// upstream behavior).
type NoSandboxLookup struct{}

// Lookup always returns ErrLookupUnsupported.
func (NoSandboxLookup) Lookup(context.Context, string, string) (cedar.Context, error) {
	return cedar.Context{}, ErrLookupUnsupported
}
