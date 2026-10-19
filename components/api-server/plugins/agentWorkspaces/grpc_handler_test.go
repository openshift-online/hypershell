package agentWorkspaces

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

type stubService struct {
	got *AgentWorkspace
	ret *AgentWorkspace
	err *errors.ServiceError
}

func (s *stubService) Get(ctx context.Context, id string) (*AgentWorkspace, *errors.ServiceError) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubService) Create(ctx context.Context, ws *AgentWorkspace) (*AgentWorkspace, *errors.ServiceError) {
	s.got = ws
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubService) Replace(ctx context.Context, ws *AgentWorkspace) (*AgentWorkspace, *errors.ServiceError) {
	if s.err != nil {
		return nil, s.err
	}
	return ws, nil
}
func (s *stubService) Delete(ctx context.Context, id string) *errors.ServiceError { return s.err }
func (s *stubService) All(ctx context.Context) (AgentWorkspaceList, *errors.ServiceError) {
	return nil, s.err
}
func (s *stubService) FindByIDs(ctx context.Context, ids []string) (AgentWorkspaceList, *errors.ServiceError) {
	return nil, s.err
}
func (s *stubService) ResolveView(ctx context.Context, agentRuntimeID string) (*WorkspaceViewData, *errors.ServiceError) {
	return nil, s.err
}

func sampleWorkspace() *AgentWorkspace {
	ws := &AgentWorkspace{AgentRuntimeId: "ar-1", GatewayId: "gw-1", Name: "amber-reviews"}
	ws.ID = "ws-1"
	return ws
}

func TestAgentWorkspaceToProto(t *testing.T) {
	got := agentWorkspaceToProto(sampleWorkspace())
	if got.GetMetadata().GetId() != "ws-1" || got.GetMetadata().GetKind() != "AgentWorkspace" {
		t.Fatalf("metadata wrong: %+v", got.GetMetadata())
	}
	if got.GetAgentRuntimeId() != "ar-1" || got.GetGatewayId() != "gw-1" || got.GetName() != "amber-reviews" {
		t.Fatalf("field mapping wrong: %+v", got)
	}
}

func TestCreateAgentWorkspace_ValidatesRequiredFields(t *testing.T) {
	h := &agentWorkspaceGRPCHandler{service: &stubService{}}
	cases := []*pb.CreateAgentWorkspaceRequest{
		{GatewayId: "gw-1", Name: "n"},
		{AgentRuntimeId: "ar-1", Name: "n"},
		{AgentRuntimeId: "ar-1", GatewayId: "gw-1"},
	}
	for i, req := range cases {
		if _, err := h.CreateAgentWorkspace(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("case %d: code = %s, want InvalidArgument", i, status.Code(err))
		}
	}
}

func TestCreateAgentWorkspace_Success(t *testing.T) {
	svc := &stubService{ret: sampleWorkspace()}
	h := &agentWorkspaceGRPCHandler{service: svc}
	resp, err := h.CreateAgentWorkspace(context.Background(), &pb.CreateAgentWorkspaceRequest{
		AgentRuntimeId: "ar-1", GatewayId: "gw-1", Name: "amber-reviews",
	})
	if err != nil {
		t.Fatalf("CreateAgentWorkspace error = %v", err)
	}
	if resp.GetAgentWorkspace().GetMetadata().GetId() != "ws-1" {
		t.Fatalf("id = %q, want ws-1", resp.GetAgentWorkspace().GetMetadata().GetId())
	}
	if svc.got.AgentRuntimeId != "ar-1" {
		t.Fatalf("service received wrong workspace: %+v", svc.got)
	}
}
