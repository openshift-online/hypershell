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

	got := agentRuntimeToProto(ar, sources)

	if got.GetStatusMessage() != "boom" {
		t.Fatalf("status_message = %q, want boom", got.GetStatusMessage())
	}
	if len(got.GetSecretSources()) != 2 {
		t.Fatalf("secret_sources count = %d, want 2", len(got.GetSecretSources()))
	}
	if got.GetSecretSources()[0].GetPurpose() != "gateway-identity" {
		t.Fatalf("first source purpose = %q", got.GetSecretSources()[0].GetPurpose())
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
