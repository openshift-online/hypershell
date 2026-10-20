package reconciler

import (
	"context"
	"fmt"
	"strings"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
)

// WorkspaceProvisioningPort is the application-owned port the AgentRuntime reconciler uses
// to realize and remove an AgentRuntime's workspace on its gateway. It is the hexagonal
// boundary the spec names (agent-workspace-gateway-api.spec.md): the reconciler depends
// only on this port, and the gateway.OpenShellProvisioner adapter (which consumes
// OpenShell's EXISTING provisioning API) implements it. Keeping the boundary here lets the
// reconciler be tested without a gateway and lets the realization adapter evolve behind one
// seam.
type WorkspaceProvisioningPort interface {
	// Realize provisions the workspace and all sub-resources idempotently.
	Realize(ctx context.Context, spec gateway.WorkspaceSpec) error
	// Delete cascade-removes the workspace and all sub-resources. Idempotent.
	Delete(ctx context.Context, gatewayID, agentRuntimeID, workspaceName string) error
}

// githubInstallationTokenMinter mints a short-lived GitHub App installation token from
// durable App material (the PEM plus app_id / installation_id). It is the bounded,
// GitHub-specific control-plane rotator: OpenShell has no app-installation-token refresh
// strategy, so the control plane mints the token and sets it on the OpenShell provider
// credential, re-minting before expiry (agent-secret-late-binding.spec.md, "Rotation
// Ownership by Refresh Strategy"). The existing githubAppInstallationTokenMinter satisfies
// this interface and caches per installation id until shortly before expiry, so a steady
// reconcile cadence does not re-mint every pass.
type githubInstallationTokenMinter interface {
	mint(ctx context.Context, material githubAppMaterial) (string, error)
}

// openshellProviderRealizer implements ProviderRealizer by translating an AgentRuntime and
// its resolved provider material into the gateway-side WorkspaceSpec and driving the
// WorkspaceProvisioningPort, which realizes each HyperShell ext kind as its OpenShell
// native kind through OpenShell's EXISTING API
// (extension-api-openshell-api-mapping.spec.md). It is the adapter from the reconciler's
// ProviderRealizer seam (#603) to the OpenShell-API realization (HYPERSHELL-412); the seam
// is unchanged, only this implementation.
//
// The AgentRuntime watch payload embeds the resolved workspace name, the
// WorkspaceMemberships, the ProviderBindings, and the InferenceRoute
// (data-model.spec.md "AgentRuntime Reconcile Payload Completeness"), so this adapter
// builds the WorkspaceSpec from those authoritative fields. Provider material is read
// cluster-local by the reconciler and passed in as ProviderMaterial.
//
// Rotation ownership is split by refresh strategy: for OpenShell-native strategies
// (google-service-account-jwt and the static-token / unknown cases) OpenShell owns rotation
// and the provider carries only durable material plus a refresh strategy; for GitHub App
// credentials (purpose app-private-key) this adapter mints the installation token from the
// stored PEM through its minter and passes it as a control-plane-rotated provider
// credential, because the installed OpenShell CLI has no app-installation-token refresh
// strategy.
//
// Remaining fidelity (follow-up): realizing one provider per ProviderBinding keyed by the
// binding's secret_source_id (rather than one per resolved provider SecretSource keyed by
// purpose), and carrying the ProviderSpec provider type and non-secret config
// (project/region) in the payload, need those fields embedded; it is tracked separately.
// End-to-end runtime activation additionally depends on the live OpenShell API adapter
// (OpenShell upstream; only the CLI and pinned ref are reachable here).
type openshellProviderRealizer struct {
	port   WorkspaceProvisioningPort
	minter githubInstallationTokenMinter
}

// NewOpenShellProviderRealizer builds a ProviderRealizer backed by the OpenShell-API
// workspace provisioning port and the GitHub installation-token minter (the control-plane
// GitHub rotator). port must be non-nil; minter may be nil only when no GitHub App provider
// is realized (a GitHub App provider with a nil minter is a loud error, never a silent
// skip).
func NewOpenShellProviderRealizer(port WorkspaceProvisioningPort, minter githubInstallationTokenMinter) (ProviderRealizer, error) {
	if port == nil {
		return nil, fmt.Errorf("openshell provider realizer requires a non-nil workspace provisioning port")
	}
	return &openshellProviderRealizer{port: port, minter: minter}, nil
}

// RealizeProviders maps the resolved provider material to a gateway WorkspaceSpec and
// realizes it through OpenShell's existing API. OpenShell owns rotation for its native
// strategies; the control-plane GitHub rotator owns rotation for GitHub App credentials.
func (g *openshellProviderRealizer) RealizeProviders(ctx context.Context, ar *pb.AgentRuntime, materials []ProviderMaterial) error {
	spec, err := g.workspaceSpecFor(ctx, ar, materials)
	if err != nil {
		return err
	}
	return g.port.Realize(ctx, spec)
}

// DeleteProviders cascade-removes the AgentRuntime's gateway-side workspace and its
// sub-resources.
func (g *openshellProviderRealizer) DeleteProviders(ctx context.Context, ar *pb.AgentRuntime) error {
	return g.port.Delete(ctx, ar.GetGatewayId(), ar.GetMetadata().GetId(), workspaceNameFor(ar))
}

// workspaceSpecFor builds the desired gateway-side workspace from an AgentRuntime and its
// resolved provider material. A provider whose material drives an OpenShell-owned refresh
// strategy or that is control-plane-rotated (GitHub) requires an admin membership, so the
// membership set is derived to include an admin when any provider needs one.
func (g *openshellProviderRealizer) workspaceSpecFor(ctx context.Context, ar *pb.AgentRuntime, materials []ProviderMaterial) (gateway.WorkspaceSpec, error) {
	if ar.GetGatewayId() == "" {
		return gateway.WorkspaceSpec{}, fmt.Errorf("agent runtime %s: cannot realize providers without a gateway_id", ar.GetMetadata().GetId())
	}
	providers := make([]gateway.ProviderConfig, 0, len(materials))
	anyAdmin := false
	for _, m := range materials {
		// Defensive SecretPending gate: the caller (AgentRuntimeReconciler) holds the
		// runtime at Provisioning and never hands us a source whose material is absent
		// (agent-secret-late-binding.spec.md "SecretPending Condition Gates Provider
		// Creation"). Refuse any empty-material entry anyway so this adapter never asks the
		// gateway to realize a provider with empty or partial material.
		if m.Material == "" {
			return gateway.WorkspaceSpec{}, fmt.Errorf("agent runtime %s: refusing to realize provider %q with empty material", ar.GetMetadata().GetId(), m.Source.GetName())
		}
		provider, err := g.providerConfigFor(ctx, ar, m)
		if err != nil {
			return gateway.WorkspaceSpec{}, err
		}
		if provider.NeedsAdmin() {
			anyAdmin = true
		}
		providers = append(providers, provider)
	}
	return gateway.WorkspaceSpec{
		GatewayID:      ar.GetGatewayId(),
		AgentRuntimeID: ar.GetMetadata().GetId(),
		WorkspaceName:  workspaceNameFor(ar),
		Providers:      providers,
		Memberships:    membershipsFor(ar, anyAdmin),
		Route:          routeFor(ar),
	}, nil
}

// providerConfigFor maps one resolved SecretSource to an OpenShell provider realization,
// splitting rotation ownership by the SecretSource purpose:
//   - app-private-key (GitHub App): the control-plane GitHub rotator mints the installation
//     token from the stored PEM and sets it as the provider credential (Rotated), with NO
//     OpenShell refresh strategy, because OpenShell has none for it.
//   - service-account-key: an OpenShell-owned google-service-account-jwt refresh strategy;
//     OpenShell mints and rotates the usable Vertex token from the durable material.
//   - any other purpose: a static provider whose credential OpenShell does not rotate.
func (g *openshellProviderRealizer) providerConfigFor(ctx context.Context, ar *pb.AgentRuntime, m ProviderMaterial) (gateway.ProviderConfig, error) {
	name := m.Source.GetName()
	switch m.Source.GetPurpose() {
	case "app-private-key":
		return g.githubProviderConfig(ctx, ar, m)
	case "service-account-key":
		const credentialKey = "GOOGLE_VERTEX_AI_SERVICE_ACCOUNT_TOKEN"
		return gateway.ProviderConfig{
			Name:          name,
			ProviderType:  "google-vertex-ai",
			CredentialKey: credentialKey,
			Refresh: &gateway.RefreshConfig{
				CredentialKey:  credentialKey,
				Strategy:       "google-service-account-jwt",
				SecretMaterial: map[string][]byte{"service-account-key": []byte(m.Material)},
			},
		}, nil
	default:
		// A static provider: the credential is used as-is and OpenShell does not rotate it
		// (static-token). No admin membership is required.
		return gateway.ProviderConfig{
			Name:                   name,
			CredentialKey:          m.Source.GetPurpose(),
			ControlPlaneCredential: m.Material,
		}, nil
	}
}

// githubProviderConfig realizes a GitHub provider from durable App material. When the
// material is a GitHub App object (PEM plus app_id / installation_id) the control-plane
// GitHub rotator mints a short-lived installation token and sets it as the provider
// credential (Rotated=true, no OpenShell refresh). A value that is not App material is
// treated as a ready static GitHub token (no control-plane rotation).
func (g *openshellProviderRealizer) githubProviderConfig(ctx context.Context, ar *pb.AgentRuntime, m ProviderMaterial) (gateway.ProviderConfig, error) {
	const credentialKey = "GH_TOKEN"
	material, ok := parseGitHubAppMaterial(m.Material)
	if !ok {
		// Not App material: use the stored value as a static GitHub token. OpenShell does
		// not rotate it and the control plane does not re-mint it.
		return gateway.ProviderConfig{
			Name:                   m.Source.GetName(),
			ProviderType:           "github",
			CredentialKey:          credentialKey,
			ControlPlaneCredential: m.Material,
		}, nil
	}
	if g.minter == nil {
		// A GitHub App provider needs the control-plane rotator; never silently skip it.
		return gateway.ProviderConfig{}, fmt.Errorf("agent runtime %s: provider %q is a GitHub App credential but no control-plane GitHub rotator is configured", ar.GetMetadata().GetId(), m.Source.GetName())
	}
	token, err := g.minter.mint(ctx, material)
	if err != nil {
		// The minter's error carries only the app / installation ids (safe references),
		// never the PEM or the token.
		return gateway.ProviderConfig{}, fmt.Errorf("agent runtime %s: mint github installation token for provider %q: %w", ar.GetMetadata().GetId(), m.Source.GetName(), err)
	}
	return gateway.ProviderConfig{
		Name:                   m.Source.GetName(),
		ProviderType:           "github",
		CredentialKey:          credentialKey,
		ControlPlaneCredential: token,
		Rotated:                true,
	}, nil
}

// workspaceNameFor returns the authoritative AgentWorkspace.name embedded in the watch
// payload (data-model.spec.md "AgentRuntime Reconcile Payload Completeness"), falling back
// to the AgentRuntime name and then its id so a workspace name is always present.
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
// WorkspaceMemberships. When the payload carries none but a provider needs an admin
// (OpenShell-refreshed or control-plane-rotated), it falls back to a single admin
// membership derived from the AgentRuntime id so such a workspace always has the admin the
// gateway requires (agent-workspace-gateway-api.spec.md "Admin-Membership-Required
// Enforcement").
func membershipsFor(ar *pb.AgentRuntime, anyAdmin bool) []gateway.MembershipSpec {
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
	if anyAdmin {
		// Last-resort placeholder: the AgentRuntime id is NOT an OIDC subject, so this admin
		// membership grants admin to no real identity; it only satisfies the gateway's "an
		// admin must exist" precondition for a refresh-bearing or rotated provider. With the
		// api-server 422 admin-required check and the embedded memberships in place this
		// should not fire in practice; it is retained only so such a workspace is never
		// realized with zero admins.
		return []gateway.MembershipSpec{{Subject: ar.GetMetadata().GetId(), Admin: true}}
	}
	return nil
}

// routeFor builds the single InferenceRoute from the payload, resolving the bound provider
// name from the ProviderBinding the route references. It returns nil when the runtime has
// no route.
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
// embedded ProviderBindings. It falls back to the id when the binding is not present so the
// route still carries a stable reference for the gateway to validate.
func providerNameForBinding(ar *pb.AgentRuntime, bindingID string) string {
	for _, b := range ar.GetProviderBindings() {
		if b.GetMetadata().GetId() == bindingID {
			return b.GetName()
		}
	}
	return bindingID
}
