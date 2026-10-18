package gateway

import (
	"context"
	"errors"
	"fmt"
	"log"

	gw "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/gateway/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// WorkspaceProvisioner realizes an AgentRuntime's workspace on the OpenShell gateway it
// is placed on, over the gateway-side provisioning gRPC contract
// (specs/platform/agent-workspace-gateway-api.spec.md). It is the control-plane client
// of that contract.
//
// It mirrors the per-gateway database provisioning channel in this package: it opens a
// SHORT-LIVED connection per provisioning operation (through the injected dialer, which
// re-resolves the gateway endpoint each call rather than caching it), reconciles
// idempotently with create-or-update ("ensure") RPCs, cascade-deletes unconditionally on
// AgentRuntime deletion, and returns structured errors that never carry secret material.
//
// The dialer seam (endpoint resolution from the Gateway resource and the gateway admin
// identity) is provided by the caller so the provisioning sequence can be unit-tested
// against a fake gateway server. Production endpoint discovery and admin authentication
// are a separate wiring concern tracked with the gateway-side server.
type WorkspaceProvisioner struct {
	dial WorkspaceDialer
}

// WorkspaceDialer opens a short-lived connection to the provisioning gRPC endpoint of
// the gateway identified by gatewayID and returns a client, a close function the caller
// MUST invoke when the operation completes, and an error. It re-resolves the endpoint on
// every call; the provisioner never caches a connection.
type WorkspaceDialer func(ctx context.Context, gatewayID string) (client gw.AgentWorkspaceProvisioningClient, closeConn func() error, err error)

// NewWorkspaceProvisioner builds a WorkspaceProvisioner over dial. dial must be non-nil.
func NewWorkspaceProvisioner(dial WorkspaceDialer) (*WorkspaceProvisioner, error) {
	if dial == nil {
		return nil, fmt.Errorf("gateway workspace provisioner requires a non-nil dialer")
	}
	return &WorkspaceProvisioner{dial: dial}, nil
}

// MembershipSpec binds an OIDC subject to a role in the workspace. Admin is required when
// any provider carries a non-empty refresh strategy.
type MembershipSpec struct {
	Subject string
	Admin   bool
}

// ProviderConfig is one OpenShell provider realized from a ProviderBinding: its durable
// material plus a refresh strategy. It carries durable material only, never a minted,
// rotatable credential (OpenShell owns rotation).
type ProviderConfig struct {
	// Name is the ProviderBinding name, unique within the workspace.
	Name string
	// SpecJSON is the gateway-agnostic ProviderSpec capability, serialized as JSON.
	SpecJSON string
	// RefreshType is one of app-installation-token, google-service-account-jwt,
	// static-token, or empty for a static provider with no rotation.
	RefreshType string
	// RefreshMaterial holds non-secret refresh parameters (for example app_id).
	RefreshMaterial map[string]string
	// RefreshSecretMaterial holds durable, secret-valued refresh parameters keyed by
	// provider field name (for example the GitHub App PEM). Never a minted token.
	RefreshSecretMaterial map[string][]byte
}

// hasRefresh reports whether this provider requires runtime credential rotation, which
// in turn requires an admin membership on the workspace.
func (p ProviderConfig) hasRefresh() bool {
	return p.RefreshType != "" || len(p.RefreshMaterial) > 0 || len(p.RefreshSecretMaterial) > 0
}

// InferenceRouteConfig binds inference.local to a provider and model alias. At most one
// per workspace.
type InferenceRouteConfig struct {
	ProviderName string
	ModelAlias   string
}

// WorkspaceSpec is the desired gateway-side state of an AgentRuntime's workspace.
type WorkspaceSpec struct {
	GatewayID      string
	AgentRuntimeID string
	WorkspaceName  string
	Memberships    []MembershipSpec
	Providers      []ProviderConfig
	Route          *InferenceRouteConfig
}

// ErrWorkspaceNameCollision is returned when the workspace name is already taken on the
// gateway by a DIFFERENT AgentRuntime (gateway ALREADY_EXISTS, surfaced as HTTP 409). It
// is a permanent rejection: it SHALL be resolved by correcting desired state, not by
// retrying under backoff.
var ErrWorkspaceNameCollision = errors.New("gateway workspace name collision")

// ErrAdminMembershipRequired is returned when a refresh-bearing provider is ensured on a
// workspace that has no admin membership (gateway FAILED_PRECONDITION, surfaced as HTTP
// 422). It is a permanent rejection.
var ErrAdminMembershipRequired = errors.New("gateway workspace admin membership required for credential refresh")

// Realize provisions the workspace and all sub-resources idempotently. It opens a
// short-lived connection, then ensures the workspace, reconciles the full membership set
// (so an admin membership exists before a refresh-bearing provider is ensured), ensures
// each provider, and ensures the single inference route. Re-running against unchanged
// desired state makes no change. A partial failure propagates so the reconcile is
// retried and completes on a later pass; the sequence is never left create-and-skip.
func (p *WorkspaceProvisioner) Realize(ctx context.Context, spec WorkspaceSpec) error {
	if spec.GatewayID == "" || spec.AgentRuntimeID == "" || spec.WorkspaceName == "" {
		return fmt.Errorf("gateway workspace realize: gateway_id, agent_runtime_id and workspace_name are required")
	}
	client, closeConn, err := p.dial(ctx, spec.GatewayID)
	if err != nil {
		return fmt.Errorf("gateway %s: dial workspace provisioning: %w", spec.GatewayID, err)
	}
	defer closeAndLog(closeConn)

	if _, err := client.EnsureWorkspace(ctx, &gw.EnsureWorkspaceRequest{
		AgentRuntimeId: spec.AgentRuntimeID,
		WorkspaceName:  spec.WorkspaceName,
	}); err != nil {
		return mapWorkspaceError(spec, "ensure workspace", permanentCollision, err)
	}

	if _, err := client.ReconcileMemberships(ctx, &gw.ReconcileMembershipsRequest{
		AgentRuntimeId: spec.AgentRuntimeID,
		WorkspaceName:  spec.WorkspaceName,
		Memberships:    toMembershipMessages(spec.Memberships),
	}); err != nil {
		return mapWorkspaceError(spec, "reconcile memberships", permanentNone, err)
	}

	for _, provider := range spec.Providers {
		if _, err := client.EnsureProvider(ctx, &gw.EnsureProviderRequest{
			AgentRuntimeId:  spec.AgentRuntimeID,
			WorkspaceName:   spec.WorkspaceName,
			ProviderName:    provider.Name,
			ProviderSpec:    provider.SpecJSON,
			RefreshStrategy: toRefreshStrategyMessage(provider),
		}); err != nil {
			return mapWorkspaceError(spec, fmt.Sprintf("ensure provider %q", provider.Name), permanentAdminRequired, err)
		}
	}

	if spec.Route != nil {
		if _, err := client.EnsureInferenceRoute(ctx, &gw.EnsureInferenceRouteRequest{
			AgentRuntimeId: spec.AgentRuntimeID,
			WorkspaceName:  spec.WorkspaceName,
			ProviderName:   spec.Route.ProviderName,
			ModelAlias:     spec.Route.ModelAlias,
		}); err != nil {
			return mapWorkspaceError(spec, "ensure inference route", permanentNone, err)
		}
	}

	return nil
}

// Delete cascade-removes the workspace and all sub-resources on the gateway. Deletion is
// unconditional and idempotent: an already-absent workspace is a successful no-op. A
// failure propagates so the delete is retried; the AgentRuntime SHALL NOT be reported
// finalized while gateway-side objects may remain.
func (p *WorkspaceProvisioner) Delete(ctx context.Context, gatewayID, agentRuntimeID, workspaceName string) error {
	if gatewayID == "" || agentRuntimeID == "" || workspaceName == "" {
		return fmt.Errorf("gateway workspace delete: gateway_id, agent_runtime_id and workspace_name are required")
	}
	client, closeConn, err := p.dial(ctx, gatewayID)
	if err != nil {
		return fmt.Errorf("gateway %s: dial workspace provisioning: %w", gatewayID, err)
	}
	defer closeAndLog(closeConn)

	if _, err := client.DeleteWorkspace(ctx, &gw.DeleteWorkspaceRequest{
		AgentRuntimeId: agentRuntimeID,
		WorkspaceName:  workspaceName,
	}); err != nil {
		if status.Code(err) == codes.NotFound {
			// Already absent: a successful, idempotent no-op.
			return nil
		}
		return fmt.Errorf("gateway %s: delete workspace %q: %w", gatewayID, workspaceName, err)
	}
	return nil
}

// permanentClass names the single permanent rejection an operation can produce, so a
// gRPC status code is classified by WHICH operation returned it rather than by the code
// alone. Only EnsureWorkspace turns ALREADY_EXISTS into a permanent name collision (409),
// and only EnsureProvider turns FAILED_PRECONDITION into a permanent admin-membership
// requirement (422). The same code from any other operation is a convergeable condition
// (for example an ordering race) and SHALL be retried, not treated as a permanent
// rejection (agent-workspace-gateway-api.spec.md, "Idempotency and Retry Semantics").
type permanentClass int

const (
	permanentNone permanentClass = iota
	permanentCollision
	permanentAdminRequired
)

// mapWorkspaceError classifies a gateway provisioning error for a specific operation.
// A permanent rejection is returned as a sentinel error so the caller degrades the
// runtime instead of retrying under backoff; every other error - including an
// unexpected ALREADY_EXISTS/FAILED_PRECONDITION from an operation that does not own that
// rejection - is a retryable error wrapped with context. The error text names the
// gateway, the operation, and the workspace only - never the durable material or the
// admin credential.
func mapWorkspaceError(spec WorkspaceSpec, op string, class permanentClass, err error) error {
	code := status.Code(err)
	if class == permanentCollision && code == codes.AlreadyExists {
		return fmt.Errorf("gateway %s: %s for workspace %q: %w", spec.GatewayID, op, spec.WorkspaceName, ErrWorkspaceNameCollision)
	}
	if class == permanentAdminRequired && code == codes.FailedPrecondition {
		return fmt.Errorf("gateway %s: %s for workspace %q: %w", spec.GatewayID, op, spec.WorkspaceName, ErrAdminMembershipRequired)
	}
	return fmt.Errorf("gateway %s: %s for workspace %q: %w", spec.GatewayID, op, spec.WorkspaceName, err)
}

// closeAndLog closes a short-lived gateway connection, logging a non-nil close error
// without failing the operation (the provisioning result has already been determined).
func closeAndLog(closeConn func() error) {
	if closeConn == nil {
		return
	}
	if err := closeConn(); err != nil {
		log.Printf("WARN gateway workspace provisioning: close connection: %v", err)
	}
}

func toMembershipMessages(memberships []MembershipSpec) []*gw.Membership {
	out := make([]*gw.Membership, 0, len(memberships))
	for _, m := range memberships {
		role := gw.Role_ROLE_MEMBER
		if m.Admin {
			role = gw.Role_ROLE_ADMIN
		}
		out = append(out, &gw.Membership{Subject: m.Subject, Role: role})
	}
	return out
}

func toRefreshStrategyMessage(provider ProviderConfig) *gw.RefreshStrategy {
	if !provider.hasRefresh() {
		return nil
	}
	return &gw.RefreshStrategy{
		Type:           provider.RefreshType,
		Material:       provider.RefreshMaterial,
		SecretMaterial: provider.RefreshSecretMaterial,
	}
}
