package reconciler

import (
	"context"
	"errors"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
)

// recordingPort records the WorkspaceSpec and delete calls the realizer drives it with.
type recordingPort struct {
	realized gateway.WorkspaceSpec
	realErr  error
	deleted  struct {
		gatewayID, agentRuntimeID, workspaceName string
	}
}

func (p *recordingPort) Realize(_ context.Context, spec gateway.WorkspaceSpec) error {
	p.realized = spec
	return p.realErr
}

func (p *recordingPort) Delete(_ context.Context, gatewayID, agentRuntimeID, workspaceName string) error {
	p.deleted.gatewayID = gatewayID
	p.deleted.agentRuntimeID = agentRuntimeID
	p.deleted.workspaceName = workspaceName
	return nil
}

// fakeMinter is a stand-in control-plane GitHub rotator that records the material it was
// asked to mint from and returns a canned token.
type fakeMinter struct {
	token    string
	err      error
	minted   githubAppMaterial
	mintance int
}

func (m *fakeMinter) mint(_ context.Context, material githubAppMaterial) (string, error) {
	m.mintance++
	m.minted = material
	if m.err != nil {
		return "", m.err
	}
	return m.token, nil
}

// githubAppJSON is a valid GitHub App material value (the durable PEM plus app_id /
// installation_id) the realizer mints an installation token from.
const githubAppJSON = `{"app_id":"123","installation_id":"456","private_key":"PEM"}`

func realizerAR() *pb.AgentRuntime {
	return &pb.AgentRuntime{
		Metadata:  &pb.ObjectReference{Id: "ar-1"},
		Name:      "amber-reviews",
		GatewayId: "gw-1",
	}
}

func TestOpenShellProviderRealizer_GitHubAppIsControlPlaneRotated(t *testing.T) {
	port := &recordingPort{}
	minter := &fakeMinter{token: "ghs-minted-token"}
	realizer, err := NewOpenShellProviderRealizer(port, minter)
	if err != nil {
		t.Fatalf("NewOpenShellProviderRealizer: %v", err)
	}
	materials := []ProviderMaterial{{
		Source:   &pb.SecretSource{Name: "github", Purpose: "app-private-key"},
		Material: githubAppJSON,
	}}

	if err := realizer.RealizeProviders(context.Background(), realizerAR(), materials); err != nil {
		t.Fatalf("RealizeProviders: %v", err)
	}
	spec := port.realized
	if spec.GatewayID != "gw-1" || spec.AgentRuntimeID != "ar-1" || spec.WorkspaceName != "amber-reviews" {
		t.Fatalf("workspace identity not mapped: %+v", spec)
	}
	if len(spec.Providers) != 1 {
		t.Fatalf("providers = %+v, want 1", spec.Providers)
	}
	p := spec.Providers[0]
	if p.ProviderType != "github" || p.CredentialKey != "GH_TOKEN" {
		t.Fatalf("github provider not mapped: %+v", p)
	}
	if !p.Rotated || p.ControlPlaneCredential != "ghs-minted-token" {
		t.Fatalf("github credential must be the control-plane-minted, rotated token: %+v", p)
	}
	if p.Refresh != nil {
		t.Fatalf("github provider must NOT configure an OpenShell refresh strategy: %+v", p.Refresh)
	}
	if minter.mintance != 1 || minter.minted.AppID != "123" || minter.minted.InstallationID != "456" {
		t.Fatalf("minter not driven with the parsed App material: %+v", minter)
	}
	// A rotated provider requires an admin membership.
	if len(spec.Memberships) != 1 || !spec.Memberships[0].Admin {
		t.Fatalf("admin membership must be derived for a rotated workspace: %+v", spec.Memberships)
	}
}

func TestOpenShellProviderRealizer_GitHubAppWithoutMinterIsError(t *testing.T) {
	port := &recordingPort{}
	realizer, _ := NewOpenShellProviderRealizer(port, nil)
	materials := []ProviderMaterial{{
		Source:   &pb.SecretSource{Name: "github", Purpose: "app-private-key"},
		Material: githubAppJSON,
	}}

	if err := realizer.RealizeProviders(context.Background(), realizerAR(), materials); err == nil {
		t.Fatalf("a GitHub App credential with no control-plane rotator must be a loud error")
	}
}

func TestOpenShellProviderRealizer_GitHubMintFailurePropagates(t *testing.T) {
	port := &recordingPort{}
	minter := &fakeMinter{err: errors.New("github down")}
	realizer, _ := NewOpenShellProviderRealizer(port, minter)
	materials := []ProviderMaterial{{
		Source:   &pb.SecretSource{Name: "github", Purpose: "app-private-key"},
		Material: githubAppJSON,
	}}

	if err := realizer.RealizeProviders(context.Background(), realizerAR(), materials); err == nil {
		t.Fatalf("a mint failure must propagate so the reconcile is retried")
	}
}

func TestOpenShellProviderRealizer_ServiceAccountIsOpenShellRefreshed(t *testing.T) {
	port := &recordingPort{}
	realizer, _ := NewOpenShellProviderRealizer(port, nil)
	materials := []ProviderMaterial{{
		Source:   &pb.SecretSource{Name: "vertex", Purpose: "service-account-key"},
		Material: "SA-KEY",
	}}

	if err := realizer.RealizeProviders(context.Background(), realizerAR(), materials); err != nil {
		t.Fatalf("RealizeProviders: %v", err)
	}
	p := port.realized.Providers[0]
	if p.ProviderType != "google-vertex-ai" {
		t.Fatalf("provider type = %q, want google-vertex-ai", p.ProviderType)
	}
	if p.Refresh == nil || p.Refresh.Strategy != "google-service-account-jwt" {
		t.Fatalf("service-account provider must configure the OpenShell google-service-account-jwt refresh: %+v", p.Refresh)
	}
	if string(p.Refresh.SecretMaterial["service-account-key"]) != "SA-KEY" {
		t.Fatalf("durable material not carried into the refresh config")
	}
	if p.Rotated {
		t.Fatalf("an OpenShell-refreshed provider must not be marked control-plane-rotated")
	}
	if len(port.realized.Memberships) != 1 || !port.realized.Memberships[0].Admin {
		t.Fatalf("admin membership must be derived for an OpenShell-refreshed workspace: %+v", port.realized.Memberships)
	}
}

func TestOpenShellProviderRealizer_StaticProviderNeedsNoAdmin(t *testing.T) {
	port := &recordingPort{}
	realizer, _ := NewOpenShellProviderRealizer(port, nil)
	materials := []ProviderMaterial{{
		Source:   &pb.SecretSource{Name: "wiki", Purpose: "unknown-static"},
		Material: "token",
	}}

	if err := realizer.RealizeProviders(context.Background(), realizerAR(), materials); err != nil {
		t.Fatalf("RealizeProviders: %v", err)
	}
	p := port.realized.Providers[0]
	if p.Refresh != nil || p.Rotated {
		t.Fatalf("unknown purpose must yield a static provider with no refresh and no rotation: %+v", p)
	}
	if p.ControlPlaneCredential != "token" {
		t.Fatalf("static provider must carry its credential value: %+v", p)
	}
	if len(port.realized.Memberships) != 0 {
		t.Fatalf("a static provider must not force an admin membership")
	}
}

func TestOpenShellProviderRealizer_EmptyMaterialIsRefused(t *testing.T) {
	port := &recordingPort{}
	realizer, _ := NewOpenShellProviderRealizer(port, nil)
	materials := []ProviderMaterial{{
		Source:   &pb.SecretSource{Name: "github", Purpose: "app-private-key"},
		Material: "",
	}}

	if err := realizer.RealizeProviders(context.Background(), realizerAR(), materials); err == nil {
		t.Fatalf("expected a refusal to realize a provider with empty material")
	}
}

func TestOpenShellProviderRealizer_DeleteCascades(t *testing.T) {
	port := &recordingPort{}
	realizer, _ := NewOpenShellProviderRealizer(port, nil)

	if err := realizer.DeleteProviders(context.Background(), realizerAR()); err != nil {
		t.Fatalf("DeleteProviders: %v", err)
	}
	if port.deleted.gatewayID != "gw-1" || port.deleted.agentRuntimeID != "ar-1" || port.deleted.workspaceName != "amber-reviews" {
		t.Fatalf("delete not driven with the workspace identity: %+v", port.deleted)
	}
}

func TestOpenShellProviderRealizer_MissingGatewayIsError(t *testing.T) {
	port := &recordingPort{}
	realizer, _ := NewOpenShellProviderRealizer(port, nil)
	ar := realizerAR()
	ar.GatewayId = ""

	if err := realizer.RealizeProviders(context.Background(), ar, nil); err == nil {
		t.Fatalf("expected an error when the AgentRuntime has no gateway_id")
	}
}

func TestNewOpenShellProviderRealizer_RequiresPort(t *testing.T) {
	if _, err := NewOpenShellProviderRealizer(nil, nil); err == nil {
		t.Fatalf("expected an error for a nil port")
	}
}

// TestOpenShellProviderRealizer_UsesEmbeddedPayload verifies the realizer builds the
// WorkspaceSpec from the authoritative embedded payload: the resolved workspace name, the
// WorkspaceMemberships, and the InferenceRoute (with the provider name resolved from its
// ProviderBinding) rather than deriving them from the AgentRuntime alone.
func TestOpenShellProviderRealizer_UsesEmbeddedPayload(t *testing.T) {
	port := &recordingPort{}
	realizer, _ := NewOpenShellProviderRealizer(port, nil)
	ar := &pb.AgentRuntime{
		Metadata:      &pb.ObjectReference{Id: "ar-1"},
		Name:          "runtime-name",
		GatewayId:     "gw-1",
		WorkspaceName: strPtr("amber-reviews"),
		WorkspaceMemberships: []*pb.WorkspaceMembership{
			{Metadata: &pb.ObjectReference{Id: "wm-1"}, Subject: "sub-admin", Role: "admin"},
			{Metadata: &pb.ObjectReference{Id: "wm-2"}, Subject: "sub-member", Role: "member"},
		},
		ProviderBindings: []*pb.ProviderBinding{
			{Metadata: &pb.ObjectReference{Id: "pb-1"}, Name: "inference"},
		},
		InferenceRoute: &pb.InferenceRoute{
			Metadata:          &pb.ObjectReference{Id: "ir-1"},
			ProviderBindingId: "pb-1",
			Model:             "claude-sonnet-4-5",
		},
	}
	materials := []ProviderMaterial{{
		Source:   &pb.SecretSource{Name: "vertex", Purpose: "service-account-key"},
		Material: "SA-KEY",
	}}

	if err := realizer.RealizeProviders(context.Background(), ar, materials); err != nil {
		t.Fatalf("RealizeProviders: %v", err)
	}
	spec := port.realized
	if spec.WorkspaceName != "amber-reviews" {
		t.Fatalf("workspace name = %q, want the embedded amber-reviews", spec.WorkspaceName)
	}
	if len(spec.Memberships) != 2 {
		t.Fatalf("memberships = %+v, want the two embedded memberships", spec.Memberships)
	}
	if spec.Memberships[0].Subject != "sub-admin" || !spec.Memberships[0].Admin {
		t.Fatalf("first membership should be the embedded admin: %+v", spec.Memberships[0])
	}
	if spec.Memberships[1].Admin {
		t.Fatalf("second membership should be a non-admin member: %+v", spec.Memberships[1])
	}
	if spec.Route == nil || spec.Route.ProviderName != "inference" || spec.Route.ModelAlias != "claude-sonnet-4-5" {
		t.Fatalf("inference route not resolved from the embedded payload: %+v", spec.Route)
	}
}

func strPtr(s string) *string { return &s }
