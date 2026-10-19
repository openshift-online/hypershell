package agentRuntimes

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// WorkspaceView and its sub-views are the subset of an AgentRuntime's gateway-side
// workspace objects embedded in the AgentRuntime gRPC payload so the control-plane
// ProviderRealizer can provision the workspace, memberships, providers, and
// inference route from a single event without a second round trip
// (data-model.spec.md "AgentRuntime Reconcile Payload Completeness"). They are
// declared in this (consumer) package so the agentWorkspaces plugin implements the
// seam without agentRuntimes importing it, keeping the dependency one-directional
// (mirrors SecretSourceView / SecretSourceLookup). They carry only durable-material
// REFERENCES (secret_source_id), never secret bytes.
type WorkspaceView struct {
	ID               string
	Name             string
	GatewayID        string
	ProviderBindings []ProviderBindingView
	Memberships      []WorkspaceMembershipView
	Route            *InferenceRouteView
}

// ProviderBindingView is one ProviderBinding the realizer realizes as an OpenShell
// provider. RefreshStrategy and SecretSourceID are references only.
type ProviderBindingView struct {
	ID              string
	Name            string
	WorkspaceID     string
	ProviderSpecID  string
	SecretSourceID  *string
	RefreshStrategy *string
	Status          *string
}

// WorkspaceMembershipView is one WorkspaceMembership (OIDC subject -> role).
type WorkspaceMembershipView struct {
	ID          string
	WorkspaceID string
	Subject     string
	Role        string
}

// InferenceRouteView is the workspace's single InferenceRoute (0..1).
type InferenceRouteView struct {
	ID                string
	WorkspaceID       string
	ProviderBindingID string
	Model             string
}

// WorkspaceLookup returns the workspace view owned by an AgentRuntime, or nil when
// the runtime has no workspace yet. A nil lookup (the agentWorkspaces plugin is
// absent) is treated as "no workspace" rather than breaking the watch.
type WorkspaceLookup func(ctx context.Context, agentRuntimeID string) (*WorkspaceView, *errors.ServiceError)

// WorkspaceLookupSource is implemented by the agentWorkspaces plugin's service
// locator so the agentRuntimes plugin can resolve the lookup through the shared
// environment service registry without a compile-time import of agentWorkspaces.
type WorkspaceLookupSource interface {
	WorkspaceLookup() WorkspaceLookup
}
