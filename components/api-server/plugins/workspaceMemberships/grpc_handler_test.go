package workspaceMemberships

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

type stubService struct {
	got *WorkspaceMembership
	ret *WorkspaceMembership
	err *errors.ServiceError
}

func (s *stubService) Get(ctx context.Context, id string) (*WorkspaceMembership, *errors.ServiceError) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubService) Create(ctx context.Context, wm *WorkspaceMembership) (*WorkspaceMembership, *errors.ServiceError) {
	s.got = wm
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubService) Replace(ctx context.Context, wm *WorkspaceMembership) (*WorkspaceMembership, *errors.ServiceError) {
	if s.err != nil {
		return nil, s.err
	}
	return wm, nil
}
func (s *stubService) Delete(ctx context.Context, id string) *errors.ServiceError { return s.err }
func (s *stubService) All(ctx context.Context) (WorkspaceMembershipList, *errors.ServiceError) {
	return nil, s.err
}
func (s *stubService) FindByIDs(ctx context.Context, ids []string) (WorkspaceMembershipList, *errors.ServiceError) {
	return nil, s.err
}

func sampleMembership() *WorkspaceMembership {
	wm := &WorkspaceMembership{WorkspaceId: "ws-1", Subject: "sub-1", Role: RoleAdmin}
	wm.ID = "wm-1"
	return wm
}

func TestWorkspaceMembershipToProto(t *testing.T) {
	got := workspaceMembershipToProto(sampleMembership())
	if got.GetMetadata().GetId() != "wm-1" || got.GetMetadata().GetKind() != "WorkspaceMembership" {
		t.Fatalf("metadata wrong: %+v", got.GetMetadata())
	}
	if got.GetWorkspaceId() != "ws-1" || got.GetSubject() != "sub-1" || got.GetRole() != RoleAdmin {
		t.Fatalf("field mapping wrong: %+v", got)
	}
}

func TestCreateWorkspaceMembership_ValidatesRequiredFields(t *testing.T) {
	h := &workspaceMembershipGRPCHandler{service: &stubService{}}
	cases := []*pb.CreateWorkspaceMembershipRequest{
		{Subject: "s", Role: "admin"},
		{WorkspaceId: "ws-1", Role: "admin"},
		{WorkspaceId: "ws-1", Subject: "s"},
	}
	for i, req := range cases {
		if _, err := h.CreateWorkspaceMembership(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("case %d: code = %s, want InvalidArgument", i, status.Code(err))
		}
	}
}

func TestCreateWorkspaceMembership_Success(t *testing.T) {
	svc := &stubService{ret: sampleMembership()}
	h := &workspaceMembershipGRPCHandler{service: svc}
	resp, err := h.CreateWorkspaceMembership(context.Background(), &pb.CreateWorkspaceMembershipRequest{
		WorkspaceId: "ws-1", Subject: "sub-1", Role: "admin",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceMembership error = %v", err)
	}
	if resp.GetWorkspaceMembership().GetMetadata().GetId() != "wm-1" {
		t.Fatalf("id = %q, want wm-1", resp.GetWorkspaceMembership().GetMetadata().GetId())
	}
	if svc.got.Role != "admin" {
		t.Fatalf("service received wrong membership: %+v", svc.got)
	}
}
