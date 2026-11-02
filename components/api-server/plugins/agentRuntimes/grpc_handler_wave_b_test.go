package agentRuntimes

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

func TestAgentRuntimeToProto_EmbedsSecretSourcesAndStatusMessage(t *testing.T) {
	ar := &AgentRuntime{Name: "amber", Status: strPtrLocal("Degraded"), StatusMessage: strPtrLocal("boom")}
	ar.ID = "ar-1"
	sources := []SecretSourceView{
		{Name: "gw", Purpose: "gateway-identity", Backend: "vault", Path: "secret/gw"},
		{Name: "key", Purpose: "api-key", Backend: "kubernetes", Path: "ns/secret"},
	}

	got := agentRuntimeToProto(ar, sources, nil)

	if got.GetStatusMessage() != "boom" {
		t.Fatalf("status_message = %q, want boom", got.GetStatusMessage())
	}
	if len(got.GetSecretSources()) != 2 {
		t.Fatalf("secret_sources count = %d, want 2", len(got.GetSecretSources()))
	}
	if got.GetSecretSources()[0].GetPurpose() != "gateway-identity" {
		t.Fatalf("first source purpose = %q", got.GetSecretSources()[0].GetPurpose())
	}
	// A nil workspace leaves the workspace-derived fields unset.
	if got.WorkspaceName != nil || len(got.GetProviderBindings()) != 0 || len(got.GetWorkspaceMemberships()) != 0 || got.GetInferenceRoute() != nil {
		t.Fatalf("nil workspace should leave workspace fields unset, got %+v", got)
	}
}

// Verifies: PLAT-DATA-14.01
func TestAgentRuntimeToProto_EmbedsWorkspacePayload(t *testing.T) {
	ar := &AgentRuntime{Name: "amber"}
	ar.ID = "ar-1"
	ref := "ss-1"
	refresh := `{"type":"app-installation-token"}`
	ws := &WorkspaceView{
		ID:        "ws-1",
		Name:      "amber-reviews",
		GatewayID: "gw-1",
		ProviderBindings: []ProviderBindingView{
			{ID: "pb-1", Name: "github", WorkspaceID: "ws-1", ProviderSpecID: "spec-1", SecretSourceID: &ref, RefreshStrategy: &refresh},
		},
		Memberships: []WorkspaceMembershipView{
			{ID: "wm-1", WorkspaceID: "ws-1", Subject: "sub-1", Role: "admin"},
		},
		Route: &InferenceRouteView{ID: "ir-1", WorkspaceID: "ws-1", ProviderBindingID: "pb-1", Model: "claude-sonnet-4-5"},
	}

	got := agentRuntimeToProto(ar, nil, ws)

	if got.GetWorkspaceName() != "amber-reviews" {
		t.Fatalf("workspace_name = %q, want amber-reviews", got.GetWorkspaceName())
	}
	if len(got.GetProviderBindings()) != 1 || got.GetProviderBindings()[0].GetRefreshStrategy() != refresh {
		t.Fatalf("provider_bindings embedding wrong: %+v", got.GetProviderBindings())
	}
	if got.GetProviderBindings()[0].GetSecretSourceId() != ref {
		t.Fatalf("secret_source_id reference not carried: %+v", got.GetProviderBindings()[0])
	}
	if len(got.GetWorkspaceMemberships()) != 1 || got.GetWorkspaceMemberships()[0].GetRole() != "admin" {
		t.Fatalf("workspace_memberships embedding wrong: %+v", got.GetWorkspaceMemberships())
	}
	if got.GetInferenceRoute().GetModel() != "claude-sonnet-4-5" {
		t.Fatalf("inference_route embedding wrong: %+v", got.GetInferenceRoute())
	}
}

func TestResolveSecretSources_NilLookupReturnsNil(t *testing.T) {
	h := &agentRuntimeGRPCHandler{}
	sources, err := h.resolveSecretSources(context.Background(), &AgentRuntime{})
	if err != nil {
		t.Fatalf("resolveSecretSources error = %v", err)
	}
	if sources != nil {
		t.Fatalf("sources = %v, want nil when lookup is unavailable", sources)
	}
}

func TestUpdateAgentRuntimeStatus_ValidatesID(t *testing.T) {
	h := &agentRuntimeGRPCHandler{}
	if _, err := h.UpdateAgentRuntimeStatus(context.Background(), &pb.UpdateAgentRuntimeStatusRequest{Status: "Running"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty id: code = %s, want InvalidArgument", status.Code(err))
	}
	if _, err := h.UpdateAgentRuntimeStatus(context.Background(), &pb.UpdateAgentRuntimeStatusRequest{Id: "ar-1"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty status: code = %s, want InvalidArgument", status.Code(err))
	}
}

func TestUpdateAgentRuntimeStatus_Success(t *testing.T) {
	svc := &stubAgentRuntimeService{updated: &AgentRuntime{Name: "amber", Status: strPtrLocal("Running")}}
	svc.updated.ID = "ar-1"
	h := &agentRuntimeGRPCHandler{service: svc}

	got, err := h.UpdateAgentRuntimeStatus(context.Background(), &pb.UpdateAgentRuntimeStatusRequest{Id: "ar-1", Status: "Running"})
	if err != nil {
		t.Fatalf("UpdateAgentRuntimeStatus error = %v", err)
	}
	if got.GetStatus() != "Running" {
		t.Fatalf("status = %q, want Running", got.GetStatus())
	}
	if svc.gotID != "ar-1" || svc.gotStatus != "Running" {
		t.Fatalf("service called with (%q,%q), want (ar-1,Running)", svc.gotID, svc.gotStatus)
	}
}

func TestUpdateAgentRuntimeStatus_NotFound(t *testing.T) {
	svc := &stubAgentRuntimeService{updateErr: errors.NotFound("missing")}
	h := &agentRuntimeGRPCHandler{service: svc}

	if _, err := h.UpdateAgentRuntimeStatus(context.Background(), &pb.UpdateAgentRuntimeStatusRequest{Id: "ar-x", Status: "Running"}); status.Code(err) != codes.NotFound {
		t.Fatalf("code = %s, want NotFound", status.Code(err))
	}
}

func strPtrLocal(s string) *string { return &s }

// stubAgentRuntimeService is a minimal AgentRuntimeService for handler tests. Only
// UpdateStatus is exercised; the rest satisfy the interface.
type stubAgentRuntimeService struct {
	updated   *AgentRuntime
	updateErr *errors.ServiceError
	gotID     string
	gotStatus string
}

func (s *stubAgentRuntimeService) UpdateStatus(_ context.Context, id, statusValue string, _ *string) (*AgentRuntime, *errors.ServiceError) {
	s.gotID = id
	s.gotStatus = statusValue
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	return s.updated, nil
}

func (s *stubAgentRuntimeService) Get(context.Context, string) (*AgentRuntime, *errors.ServiceError) {
	return nil, nil
}
func (s *stubAgentRuntimeService) GetUnscoped(context.Context, string) (*AgentRuntime, *errors.ServiceError) {
	return nil, nil
}
func (s *stubAgentRuntimeService) Create(context.Context, *AgentRuntime) (*AgentRuntime, *errors.ServiceError) {
	return nil, nil
}
func (s *stubAgentRuntimeService) Replace(context.Context, *AgentRuntime) (*AgentRuntime, *errors.ServiceError) {
	return nil, nil
}
func (s *stubAgentRuntimeService) Delete(context.Context, string) *errors.ServiceError { return nil }
func (s *stubAgentRuntimeService) All(context.Context) (AgentRuntimeList, *errors.ServiceError) {
	return nil, nil
}
func (s *stubAgentRuntimeService) FindByIDs(context.Context, []string) (AgentRuntimeList, *errors.ServiceError) {
	return nil, nil
}
