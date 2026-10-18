package reconciler

import (
	"context"
	"fmt"
	"strings"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
)

// WorkspaceProvisioningPort is the application-owned port the AgentRuntime reconciler
// uses to realize and remove an AgentRuntime's workspace on its gateway. It is the
// hexagonal boundary the spec names (agent-workspace-gateway-api.spec.md): the reconciler
// depends only on this port, and the gateway gRPC client in internal/gateway is the
// adapter that implements it. Keeping the boundary here lets the reconciler be tested
// without a gateway and lets the gateway contract evolve behind one seam.
type WorkspaceProvisioningPort interface {
	// Realize provisions the workspace and all sub-resources idempotently.
	Realize(ctx context.Context, spec gateway.WorkspaceSpec) error
	// Delete cascade-removes the workspace and all sub-resources. Idempotent.
	Delete(ctx context.Context, gatewayID, agentRuntimeID, workspaceName string) error
}

// gatewayProviderRealizer implements ProviderRealizer by translating an AgentRuntime and
// its resolved provider material into the gateway-side WorkspaceSpec and driving the
// WorkspaceProvisioningPort. It is the adapter from the reconciler's ProviderRealizer
// seam (#603) to the gateway-side provisioning contract (#526).
//
// The AgentRuntime watch payload embeds the resolved workspace name, the
// WorkspaceMemberships, the ProviderBindings, and the InferenceRoute
// (data-model.spec.md "AgentRuntime Reconcile Payload Completeness"), so this adapter
// builds the WorkspaceSpec from those authoritative fields: the workspace name and
// memberships come from the payload, and the inference route is bound to its
// ProviderBinding's provider name. Provider material is read cluster-local by the
// reconciler and passed in as ProviderMaterial; each provider carries durable material
// plus a refresh strategy only, and OpenShell owns rotation.
//
// Remaining fidelity (follow-up): realizing one provider per ProviderBinding keyed by the
// binding's secret_source_id (rather than one per resolved provider SecretSource keyed by
// purpose) needs the SecretSource id carried in the payload so material joins to its
// binding; it is tracked separately. End-to-end runtime activation additionally depends on
// the gateway-side provisioning server (OpenShell upstream, a declared non-goal of #606).
type gatewayProviderRealizer struct {
	port WorkspaceProvisioningPort
}

// NewGatewayProviderRealizer builds a ProviderRealizer backed by the gateway-side
// workspace provisioning port. port must be non-nil.
func NewGatewayProviderRealizer(port WorkspaceProvisioningPort) (ProviderRealizer, error) {
	if port == nil {
		return nil, fmt.Errorf("gateway provider realizer requires a non-nil workspace provisioning port")
	}
	return &gatewayProviderRealizer{port: port}, nil
}

// RealizeProviders maps the resolved provider material to a gateway WorkspaceSpec and
// realizes it. Each provider carries durable material plus a refresh strategy only; no
// minted credential crosses the contract and OpenShell owns rotation.
func (g *gatewayProviderRealizer) RealizeProviders(ctx context.Context, ar *pb.AgentRuntime, materials []ProviderMaterial) error {
	spec, err := workspaceSpecFor(ar, materials)
	if err != nil {
		return err
	}
	return g.port.Realize(ctx, spec)
}

// DeleteProviders cascade-removes the AgentRuntime's gateway-side workspace and its
// sub-resources.
func (g *gatewayProviderRealizer) DeleteProviders(ctx context.Context, ar *pb.AgentRuntime) error {
	return g.port.Delete(ctx, ar.GetGatewayId(), ar.GetMetadata().GetId(), workspaceNameFor(ar))
}

// workspaceSpecFor builds the desired gateway-side workspace from an AgentRuntime and its
// resolved provider material. A provider whose material drives a non-empty refresh
// strategy requires an admin membership; the gateway enforces that (FAILED_PRECONDITION),
// so the membership set is derived to include an admin when any provider refreshes.
func workspaceSpecFor(ar *pb.AgentRuntime, materials []ProviderMaterial) (gateway.WorkspaceSpec, error) {
	if ar.GetGatewayId() == "" {
		return gateway.WorkspaceSpec{}, fmt.Errorf("agent runtime %s: cannot realize providers without a gateway_id", ar.GetMetadata().GetId())
	}
	providers := make([]gateway.ProviderConfig, 0, len(materials))
	anyRefresh := false
	for _, m := range materials {
		// Defensive SecretPending gate: the caller (AgentRuntimeReconciler) holds the
		// runtime at Provisioning and never hands us a source whose material is absent
		// (agent-secret-late-binding.spec.md "SecretPending Condition Gates Provider
		// Creation"). Skip any empty-material entry anyway so this adapter never asks
		// the gateway to realize a provider with empty or partial material.
		if m.Material == "" {
			return gateway.WorkspaceSpec{}, fmt.Errorf("agent runtime %s: refusing to realize provider %q with empty material", ar.GetMetadata().GetId(), m.Source.GetName())
		}
		refreshType := refreshTypeForPurpose(m.Source.GetPurpose())
		if refreshType != "" {
			anyRefresh = true
		}
		providers = append(providers, gateway.ProviderConfig{
			Name:                  m.Source.GetName(),
			RefreshType:           refreshType,
			RefreshSecretMaterial: map[string][]byte{m.Source.GetPurpose(): []byte(m.Material)},
		})
	}
	spec := gateway.WorkspaceSpec{
		GatewayID:      ar.GetGatewayId(),
		AgentRuntimeID: ar.GetMetadata().GetId(),
		WorkspaceName:  workspaceNameFor(ar),
		Providers:      providers,
		Memberships:    membershipsFor(ar, anyRefresh),
		Route:          routeFor(ar),
	}
	return spec, nil
}

// workspaceNameFor returns the authoritative AgentWorkspace.name embedded in the watch
// payload (data-model.spec.md "AgentRuntime Reconcile Payload Completeness"), falling
// back to the AgentRuntime name and then its id so a workspace name is always present.
func workspaceNameFor(ar *pb.AgentRuntime) string {
	if name := strings.TrimSpace(ar.GetWorkspaceName()); name != "" {
		return name
	}
	if name := strings.TrimSpace(ar.GetName()); name != "" {
		return name
	}
	return ar.GetMetadata().GetId()
}

// membershipsFor builds the declarative membership set from the payload's embedded
// WorkspaceMemberships. When the payload carries none but a provider refreshes, it falls
// back to a single admin membership derived from the AgentRuntime id so a refresh-bearing
// workspace always has the admin the gateway requires (agent-workspace-gateway-api.spec.md
// "Admin-Membership-Required Enforcement").
func membershipsFor(ar *pb.AgentRuntime, anyRefresh bool) []gateway.MembershipSpec {
	embedded := ar.GetWorkspaceMemberships()
	if len(embedded) > 0 {
		out := make([]gateway.MembershipSpec, 0, len(embedded))
		for _, m := range embedded {
			out = append(out, gateway.MembershipSpec{
				Subject: m.GetSubject(),
				Admin:   m.GetRole() == "admin",
			})
		}
		return out
	}
	if anyRefresh {
		// Last-resort placeholder: the AgentRuntime id is NOT an OIDC subject, so this
		// admin membership grants admin to no real identity; it only satisfies the
		// gateway's "an admin must exist" precondition for a refresh-bearing provider.
		// With the api-server 422 admin-required check and the embedded memberships in
		// place this should not fire in practice; it is retained only so a refresh-bearing
		// workspace is never realized with zero admins.
		return []gateway.MembershipSpec{{Subject: ar.GetMetadata().GetId(), Admin: true}}
	}
	return nil
}

// routeFor builds the single InferenceRoute from the payload, resolving the bound
// provider name from the ProviderBinding the route references. It returns nil when the
// runtime has no route.
func routeFor(ar *pb.AgentRuntime) *gateway.InferenceRouteConfig {
	route := ar.GetInferenceRoute()
	if route == nil {
		return nil
	}
	return &gateway.InferenceRouteConfig{
		ProviderName: providerNameForBinding(ar, route.GetProviderBindingId()),
		ModelAlias:   route.GetModel(),
	}
}

// providerNameForBinding resolves a ProviderBinding id to its name from the payload's
// embedded ProviderBindings. It falls back to the id when the binding is not present so
// the route still carries a stable reference for the gateway to validate.
func providerNameForBinding(ar *pb.AgentRuntime, bindingID string) string {
	for _, b := range ar.GetProviderBindings() {
		if b.GetMetadata().GetId() == bindingID {
			return b.GetName()
		}
	}
	return bindingID
}

// refreshTypeForPurpose maps a SecretSource purpose to the OpenShell refresh strategy
// OpenShell uses to mint and rotate the usable credential. A purpose with no runtime
// rotation yields an empty strategy (a static provider).
func refreshTypeForPurpose(purpose string) string {
	switch purpose {
	case "app-private-key":
		return "app-installation-token"
	case "service-account-key":
		return "google-service-account-jwt"
	case "api-key":
		return "static-token"
	default:
		return ""
	}
}
