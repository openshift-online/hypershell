package gateway

import (
	"context"
	"errors"
	"fmt"
	"log"
)

// OpenShellProvisioner realizes an AgentRuntime's workspace on the OpenShell gateway it
// is placed on by CONSUMING OpenShell's EXISTING provisioning API - the operations the
// `openshell` CLI drives (workspace, workspace member, provider, provider refresh,
// inference) - as the gateway admin identity
// (specs/platform/agent-workspace-gateway-api.spec.md,
// specs/platform/extension-api-openshell-api-mapping.spec.md). It is NOT a client of a
// bespoke HyperShell contract OpenShell must implement; that contract was superseded by
// HYPERSHELL-412.
//
// It mirrors the per-gateway database provisioning channel's DISCIPLINE in this package:
// it opens a SHORT-LIVED session per provisioning pass (through the injected session
// factory, which re-resolves the gateway endpoint each call rather than caching it),
// reconciles idempotently with create-or-update ("ensure") operations, cascade-deletes
// unconditionally on AgentRuntime deletion, and returns structured errors that never
// carry secret material.
//
// The session factory seam (endpoint resolution from the Gateway resource and the gateway
// admin identity) is provided by the caller so the provisioning sequence can be
// unit-tested against a fake OpenShell API. Production endpoint discovery and admin
// authentication, and the live OpenShell API adapter itself, are a separate wiring concern
// tracked with the OpenShell upstream client surface (only the pinned ref and the CLI are
// reachable from this repository).
type OpenShellProvisioner struct {
	session OpenShellSessionFactory
}

// OpenShellSessionFactory opens a short-lived session to the OpenShell provisioning API of
// the gateway identified by gatewayID and returns an OpenShellAPI, a close function the
// caller MUST invoke when the pass completes, and an error. It re-resolves the endpoint on
// every call; the provisioner never caches a session.
type OpenShellSessionFactory func(ctx context.Context, gatewayID string) (api OpenShellAPI, closeSession func() error, err error)

// OpenShellAPI is the subset of OpenShell's existing provisioning API that the control
// plane consumes, one method per operation the `openshell` CLI drives (see the 1:1 map in
// specs/platform/extension-api-openshell-api-mapping.spec.md). Each operation is
// idempotent create-or-update ("ensure") so a reconcile that re-runs against unchanged
// desired state is a no-op. An implementation SHALL return ErrWorkspaceNameCollision when
// a workspace name is already taken by a different owner, and ErrAdminMembershipRequired
// when a refresh-bearing or rotated provider is ensured without an admin membership; every
// other error is treated as retryable.
type OpenShellAPI interface {
	// EnsureWorkspace creates the workspace when absent; a workspace already owned by the
	// same agentRuntimeID is a no-op success. OpenShell `workspace create`.
	EnsureWorkspace(ctx context.Context, agentRuntimeID, workspaceName string) error
	// ReconcileMemberships converges the FULL membership set: adds missing, updates
	// changed roles, removes memberships no longer declared. OpenShell `workspace member`.
	ReconcileMemberships(ctx context.Context, workspaceName string, memberships []MembershipSpec) error
	// EnsureProvider creates or updates one OpenShell provider. OpenShell `provider
	// create` / `provider update`. The credential it carries is either a static credential
	// or the control-plane-minted GitHub installation token; OpenShell-owned refresh
	// material is supplied separately through ConfigureProviderRefresh.
	EnsureProvider(ctx context.Context, workspaceName string, provider ProviderRealization) error
	// ConfigureProviderRefresh configures an OpenShell-owned refresh strategy for a
	// provider so OpenShell mints and rotates the usable credential. OpenShell `provider
	// refresh configure`. It is called ONLY for OpenShell-native strategies, never for the
	// GitHub app-installation-token (control-plane-rotated) path.
	ConfigureProviderRefresh(ctx context.Context, workspaceName string, refresh RefreshRealization) error
	// SetInferenceRoute converges the single inference route binding inference.local to a
	// provider and model alias. OpenShell `inference set`.
	SetInferenceRoute(ctx context.Context, workspaceName, providerName, modelAlias string) error
	// DeleteWorkspace cascade-deletes the workspace and all sub-resources. OpenShell
	// `workspace delete`. An already-absent workspace is a successful no-op.
	DeleteWorkspace(ctx context.Context, workspaceName string) error
}

// NewOpenShellProvisioner builds an OpenShellProvisioner over session. session must be
// non-nil.
func NewOpenShellProvisioner(session OpenShellSessionFactory) (*OpenShellProvisioner, error) {
	if session == nil {
		return nil, fmt.Errorf("openshell workspace provisioner requires a non-nil session factory")
	}
	return &OpenShellProvisioner{session: session}, nil
}

// MembershipSpec binds an OIDC subject to a role in the workspace. Admin is required when
// any provider is refresh-bearing (OpenShell-owned) or control-plane-rotated (GitHub),
// because OpenShell restricts provider credential updates to workspace administrators.
type MembershipSpec struct {
	Subject string
	Admin   bool
}

// ProviderConfig is one OpenShell provider realized from a ProviderBinding. It carries the
// gateway-agnostic provider type, the credential wiring, and - for OpenShell-owned refresh
// strategies only - the durable refresh material. It never carries a minted, rotatable
// credential for an OpenShell-owned strategy (OpenShell owns that rotation).
type ProviderConfig struct {
	// Name is the ProviderBinding name, unique within the workspace.
	Name string
	// ProviderType is the gateway-agnostic OpenShell provider type (for example
	// google-vertex-ai or github).
	ProviderType string
	// CredentialKey is the injectable credential key the provider exposes (for example
	// GH_TOKEN or GOOGLE_VERTEX_AI_SERVICE_ACCOUNT_TOKEN).
	CredentialKey string
	// ControlPlaneCredential is a credential VALUE the control plane supplies directly:
	// the minted GitHub installation token (Rotated=true) or a static token. It is empty
	// when OpenShell owns rotation through Refresh. It is never logged.
	ControlPlaneCredential string
	// Rotated reports that the control plane owns rotating ControlPlaneCredential at
	// runtime (the GitHub app-installation-token path). A rotated provider requires an
	// admin membership just like an OpenShell-refreshed one.
	Rotated bool
	// Config holds non-secret, provider-type-specific configuration (for example project
	// and region).
	Config map[string]string
	// Refresh is set ONLY for OpenShell-owned refresh strategies; it is nil for the GitHub
	// control-plane rotator and for static providers.
	Refresh *RefreshConfig
}

// RefreshConfig is an OpenShell-owned refresh strategy for a provider: OpenShell mints and
// rotates the usable credential from this material. It is never used for the GitHub
// app-installation-token path (which the control plane rotates).
type RefreshConfig struct {
	// CredentialKey is the injectable credential key OpenShell refreshes.
	CredentialKey string
	// Strategy is an OpenShell-native refresh strategy (for example
	// google-service-account-jwt, oauth2-refresh-token, oauth2-client-credentials,
	// aws-sts-assume-role).
	Strategy string
	// Material holds non-secret refresh parameters (for example client_email).
	Material map[string]string
	// SecretMaterial holds durable, secret-valued refresh parameters keyed by provider
	// field name (for example the service-account private key). Never a minted token.
	SecretMaterial map[string][]byte
}

// NeedsAdmin reports whether this provider requires an admin membership: either OpenShell
// refreshes it or the control plane rotates it, and OpenShell restricts both to admins.
func (p ProviderConfig) NeedsAdmin() bool {
	return p.Refresh != nil || p.Rotated
}

// ProviderRealization is the input to EnsureProvider: the provider identity, type,
// credential wiring, and non-secret config.
type ProviderRealization struct {
	Name            string
	ProviderType    string
	CredentialKey   string
	CredentialValue string
	Config          map[string]string
}

// RefreshRealization is the input to ConfigureProviderRefresh for an OpenShell-owned
// refresh strategy.
type RefreshRealization struct {
	ProviderName   string
	CredentialKey  string
	Strategy       string
	Material       map[string]string
	SecretMaterial map[string][]byte
}

// InferenceRouteConfig binds inference.local to a provider and model alias. At most one
// per workspace.
type InferenceRouteConfig struct {
	ProviderName string
	ModelAlias   string
}

// WorkspaceSpec is the desired OpenShell-side state of an AgentRuntime's workspace.
type WorkspaceSpec struct {
	GatewayID      string
	AgentRuntimeID string
	WorkspaceName  string
	Memberships    []MembershipSpec
	Providers      []ProviderConfig
	Route          *InferenceRouteConfig
}

// ErrWorkspaceNameCollision is returned when the workspace name is already taken on the
// gateway by a DIFFERENT AgentRuntime (surfaced as HTTP 409). It is a permanent rejection:
// it SHALL be resolved by correcting desired state, not by retrying under backoff.
var ErrWorkspaceNameCollision = errors.New("openshell workspace name collision")

// ErrAdminMembershipRequired is returned when a refresh-bearing or control-plane-rotated
// provider is ensured on a workspace that has no admin membership (surfaced as HTTP 422).
// It is a permanent rejection.
var ErrAdminMembershipRequired = errors.New("openshell workspace admin membership required for credential refresh")

// Realize provisions the workspace and all sub-resources idempotently through OpenShell's
// existing API. It opens a short-lived session, ensures the workspace, reconciles the full
// membership set (so an admin membership exists before a refresh-bearing or rotated
// provider is ensured), ensures each provider (configuring an OpenShell-owned refresh
// strategy where one applies; the GitHub token the control plane minted is set as the
// provider credential with no OpenShell refresh), and sets the single inference route.
// Re-running against unchanged desired state makes no change. A partial failure propagates
// so the reconcile is retried and completes on a later pass; the sequence is never left
// create-and-skip.
func (p *OpenShellProvisioner) Realize(ctx context.Context, spec WorkspaceSpec) error {
	if spec.GatewayID == "" || spec.AgentRuntimeID == "" || spec.WorkspaceName == "" {
		return fmt.Errorf("openshell workspace realize: gateway_id, agent_runtime_id and workspace_name are required")
	}
	api, closeSession, err := p.session(ctx, spec.GatewayID)
	if err != nil {
		return fmt.Errorf("gateway %s: open openshell session: %w", spec.GatewayID, err)
	}
	defer closeAndLog(closeSession)

	if err := api.EnsureWorkspace(ctx, spec.AgentRuntimeID, spec.WorkspaceName); err != nil {
		return wrapWorkspaceError(spec, "ensure workspace", permanentCollision, err)
	}

	if err := api.ReconcileMemberships(ctx, spec.WorkspaceName, spec.Memberships); err != nil {
		return wrapWorkspaceError(spec, "reconcile memberships", permanentNone, err)
	}

	for _, provider := range spec.Providers {
		if err := api.EnsureProvider(ctx, spec.WorkspaceName, ProviderRealization{
			Name:            provider.Name,
			ProviderType:    provider.ProviderType,
			CredentialKey:   provider.CredentialKey,
			CredentialValue: provider.ControlPlaneCredential,
			Config:          provider.Config,
		}); err != nil {
			return wrapWorkspaceError(spec, fmt.Sprintf("ensure provider %q", provider.Name), permanentAdminRequired, err)
		}
		// OpenShell-owned refresh is configured ONLY for OpenShell-native strategies. The
		// GitHub app-installation-token path (provider.Rotated) carries its credential
		// through EnsureProvider above and is rotated by the control plane, so it never
		// configures an OpenShell refresh strategy (agent-secret-late-binding.spec.md,
		// "Rotation Ownership by Refresh Strategy").
		if provider.Refresh != nil {
			if err := api.ConfigureProviderRefresh(ctx, spec.WorkspaceName, RefreshRealization{
				ProviderName:   provider.Name,
				CredentialKey:  provider.Refresh.CredentialKey,
				Strategy:       provider.Refresh.Strategy,
				Material:       provider.Refresh.Material,
				SecretMaterial: provider.Refresh.SecretMaterial,
			}); err != nil {
				return wrapWorkspaceError(spec, fmt.Sprintf("configure refresh for provider %q", provider.Name), permanentAdminRequired, err)
			}
		}
	}

	if spec.Route != nil {
		if err := api.SetInferenceRoute(ctx, spec.WorkspaceName, spec.Route.ProviderName, spec.Route.ModelAlias); err != nil {
			return wrapWorkspaceError(spec, "set inference route", permanentNone, err)
		}
	}

	return nil
}

// Delete cascade-removes the workspace and all sub-resources on the gateway. Deletion is
// unconditional and idempotent: an already-absent workspace is a successful no-op. A
// failure propagates so the delete is retried; the AgentRuntime SHALL NOT be reported
// finalized while gateway-side objects may remain.
func (p *OpenShellProvisioner) Delete(ctx context.Context, gatewayID, agentRuntimeID, workspaceName string) error {
	if gatewayID == "" || agentRuntimeID == "" || workspaceName == "" {
		return fmt.Errorf("openshell workspace delete: gateway_id, agent_runtime_id and workspace_name are required")
	}
	api, closeSession, err := p.session(ctx, gatewayID)
	if err != nil {
		return fmt.Errorf("gateway %s: open openshell session: %w", gatewayID, err)
	}
	defer closeAndLog(closeSession)

	if err := api.DeleteWorkspace(ctx, workspaceName); err != nil {
		return fmt.Errorf("gateway %s: delete workspace %q: %w", gatewayID, workspaceName, err)
	}
	return nil
}

// permanentClass names the single permanent rejection an operation may legitimately
// produce, so a sentinel is classified by WHICH operation returned it rather than by its
// identity alone. Only EnsureWorkspace may turn ErrWorkspaceNameCollision into a permanent
// name collision (409), and only a provider operation (EnsureProvider /
// ConfigureProviderRefresh) may turn ErrAdminMembershipRequired into a permanent
// admin-membership requirement (422). The same sentinel from any other operation is an
// unexpected, convergeable condition (for example an ordering race) and SHALL be retried,
// not treated as permanent (agent-workspace-gateway-api.spec.md "Idempotency and Retry
// Semantics").
type permanentClass int

const (
	permanentNone permanentClass = iota
	permanentCollision
	permanentAdminRequired
)

// wrapWorkspaceError wraps an OpenShell provisioning error for a specific operation with
// context. When the error is the permanent sentinel the operation is allowed to produce, it
// is preserved (via %w) so it remains discoverable through errors.Is and the caller degrades
// the runtime instead of retrying. Every other error - including an unexpected permanent
// sentinel surfaced from an operation that does not own it - is returned as a retryable
// error: an unexpected sentinel has its chain broken (%v, not %w) so errors.Is no longer
// matches it and the reconcile retries. The error text names the gateway, the operation, and
// the workspace only - never the durable material or the admin credential.
func wrapWorkspaceError(spec WorkspaceSpec, op string, permit permanentClass, err error) error {
	if permit == permanentCollision && errors.Is(err, ErrWorkspaceNameCollision) {
		return fmt.Errorf("gateway %s: %s for workspace %q: %w", spec.GatewayID, op, spec.WorkspaceName, err)
	}
	if permit == permanentAdminRequired && errors.Is(err, ErrAdminMembershipRequired) {
		return fmt.Errorf("gateway %s: %s for workspace %q: %w", spec.GatewayID, op, spec.WorkspaceName, err)
	}
	if errors.Is(err, ErrWorkspaceNameCollision) || errors.Is(err, ErrAdminMembershipRequired) {
		// An unexpected permanent sentinel from the wrong operation: do NOT let it
		// propagate as permanent. Break the chain so the reconcile retries.
		return fmt.Errorf("gateway %s: %s for workspace %q: %v (unexpected for this operation; retryable)", spec.GatewayID, op, spec.WorkspaceName, err)
	}
	return fmt.Errorf("gateway %s: %s for workspace %q: %w", spec.GatewayID, op, spec.WorkspaceName, err)
}

// closeAndLog closes a short-lived OpenShell session, logging a non-nil close error
// without failing the operation (the provisioning result has already been determined).
func closeAndLog(closeSession func() error) {
	if closeSession == nil {
		return
	}
	if err := closeSession(); err != nil {
		log.Printf("WARN openshell workspace provisioning: close session: %v", err)
	}
}
