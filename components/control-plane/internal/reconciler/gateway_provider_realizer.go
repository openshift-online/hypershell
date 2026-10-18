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
// Material mapping fidelity: the AgentRuntime watch payload currently embeds its
// SecretSources but NOT its ProviderBindings, workspace name, memberships, or inference
// route (see agent_runtimes.proto). This adapter therefore derives the workspace name
// from the AgentRuntime and realizes one provider per resolved provider SecretSource,
// keyed by the SecretSource purpose. Realizing providers at full ProviderBinding fidelity
// (binding name, ProviderSpec capability, refresh strategy, memberships, and the
// inference route) requires extending the AgentRuntime watch payload to embed those
// objects; that payload extension is the prerequisite for activating this realizer in
// production and is tracked as follow-up work. Until then the realizer is constructible
// and unit-tested but not wired into the production reconciler (the reconciler keeps the
// transitional raw-Secret placement, guarded by a nil realizer).
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
	}
	if anyRefresh {
		// A refresh-bearing provider requires a workspace admin. The admin subject is
		// the agent's own gateway identity; its subject claim is carried with the
		// gateway-identity SecretSource and is embedded in the watch payload alongside
		// the ProviderBindings in the follow-up payload extension. Until then the
		// membership subject is derived from the AgentRuntime id so an admin membership
		// is always present for a refresh-bearing workspace.
		spec.Memberships = []gateway.MembershipSpec{{Subject: ar.GetMetadata().GetId(), Admin: true}}
	}
	return spec, nil
}

// workspaceNameFor derives the workspace name from the AgentRuntime. The authoritative
// name is AgentWorkspace.name once that kind is embedded in the watch payload; until then
// the AgentRuntime name is used, which is stable per runtime.
func workspaceNameFor(ar *pb.AgentRuntime) string {
	name := strings.TrimSpace(ar.GetName())
	if name == "" {
		return ar.GetMetadata().GetId()
	}
	return name
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
