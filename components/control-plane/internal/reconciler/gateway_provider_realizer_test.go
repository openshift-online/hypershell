package reconciler

import (
	"context"
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

func realizerAR() *pb.AgentRuntime {
	return &pb.AgentRuntime{
		Metadata:  &pb.ObjectReference{Id: "ar-1"},
		Name:      "amber-reviews",
		GatewayId: "gw-1",
	}
}

func TestGatewayProviderRealizer_MapsMaterialToWorkspaceSpec(t *testing.T) {
	port := &recordingPort{}
	realizer, err := NewGatewayProviderRealizer(port)
	if err != nil {
		t.Fatalf("NewGatewayProviderRealizer: %v", err)
	}
	materials := []ProviderMaterial{{
		Source:   &pb.SecretSource{Name: "github-key", Purpose: "app-private-key"},
		Material: "PEM",
	}}

	if err := realizer.RealizeProviders(context.Background(), realizerAR(), materials); err != nil {
		t.Fatalf("RealizeProviders: %v", err)
	}
	spec := port.realized
	if spec.GatewayID != "gw-1" || spec.AgentRuntimeID != "ar-1" || spec.WorkspaceName != "amber-reviews" {
		t.Fatalf("workspace identity not mapped: %+v", spec)
	}
	if len(spec.Providers) != 1 || spec.Providers[0].Name != "github-key" {
		t.Fatalf("provider not mapped: %+v", spec.Providers)
	}
	if spec.Providers[0].RefreshType != "app-installation-token" {
		t.Fatalf("refresh type = %q, want app-installation-token", spec.Providers[0].RefreshType)
	}
	if string(spec.Providers[0].RefreshSecretMaterial["app-private-key"]) != "PEM" {
		t.Fatalf("durable material not carried into the provider config")
	}
	// A refresh-bearing provider requires an admin membership.
	if len(spec.Memberships) != 1 || !spec.Memberships[0].Admin {
		t.Fatalf("admin membership must be derived for a refresh-bearing workspace: %+v", spec.Memberships)
	}
}

func TestGatewayProviderRealizer_StaticProviderNeedsNoAdmin(t *testing.T) {
	port := &recordingPort{}
	realizer, _ := NewGatewayProviderRealizer(port)
	materials := []ProviderMaterial{{
		Source:   &pb.SecretSource{Name: "wiki", Purpose: "unknown-static"},
		Material: "token",
	}}

	if err := realizer.RealizeProviders(context.Background(), realizerAR(), materials); err != nil {
		t.Fatalf("RealizeProviders: %v", err)
	}
	if port.realized.Providers[0].RefreshType != "" {
		t.Fatalf("unknown purpose must yield a static provider with no refresh strategy")
	}
	if len(port.realized.Memberships) != 0 {
		t.Fatalf("a static provider must not force an admin membership")
	}
}

func TestGatewayProviderRealizer_EmptyMaterialIsRefused(t *testing.T) {
	port := &recordingPort{}
	realizer, _ := NewGatewayProviderRealizer(port)
	materials := []ProviderMaterial{{
		Source:   &pb.SecretSource{Name: "github-key", Purpose: "app-private-key"},
		Material: "",
	}}

	if err := realizer.RealizeProviders(context.Background(), realizerAR(), materials); err == nil {
		t.Fatalf("expected a refusal to realize a provider with empty material")
	}
}

func TestGatewayProviderRealizer_DeleteCascades(t *testing.T) {
	port := &recordingPort{}
	realizer, _ := NewGatewayProviderRealizer(port)

	if err := realizer.DeleteProviders(context.Background(), realizerAR()); err != nil {
		t.Fatalf("DeleteProviders: %v", err)
	}
	if port.deleted.gatewayID != "gw-1" || port.deleted.agentRuntimeID != "ar-1" || port.deleted.workspaceName != "amber-reviews" {
		t.Fatalf("delete not driven with the workspace identity: %+v", port.deleted)
	}
}

func TestGatewayProviderRealizer_MissingGatewayIsError(t *testing.T) {
	port := &recordingPort{}
	realizer, _ := NewGatewayProviderRealizer(port)
	ar := realizerAR()
	ar.GatewayId = ""

	if err := realizer.RealizeProviders(context.Background(), ar, nil); err == nil {
		t.Fatalf("expected an error when the AgentRuntime has no gateway_id")
	}
}

func TestNewGatewayProviderRealizer_RequiresPort(t *testing.T) {
	if _, err := NewGatewayProviderRealizer(nil); err == nil {
		t.Fatalf("expected an error for a nil port")
	}
}
