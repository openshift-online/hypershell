package agentRuntimes

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// SecretSourceView is the subset of a SecretSource the AgentRuntime gRPC payload
// carries so the control-plane reconciler can render one ExternalSecret per
// source and wire the coordinator CronJob env vars by purpose. It is declared in
// this (consumer) package so the secretSources plugin implements the seam
// without agentRuntimes importing it - mirroring the roleBindings <-> gateways
// GatewayClusterLookup seam and keeping the dependency one-directional.
type SecretSourceView struct {
	Name        string
	Purpose     string
	Backend     string
	Path        string
	KeyMappings *string
}

// SecretSourceLookup returns the SecretSources owned by an AgentRuntime, ordered
// deterministically by the backing store. A runtime with no sources returns an
// empty slice and a nil error.
type SecretSourceLookup func(ctx context.Context, agentRuntimeID string) ([]SecretSourceView, *errors.ServiceError)

// SecretSourceLookupSource is implemented by the secretSources plugin's service
// locator so the agentRuntimes plugin can resolve the lookup through the shared
// environment service registry without a compile-time import of secretSources.
type SecretSourceLookupSource interface {
	SecretSourceLookup() SecretSourceLookup
}
