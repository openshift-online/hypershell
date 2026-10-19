package repositories

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// stubRepositoryService is a minimal RepositoryService for gRPC handler tests.
type stubRepositoryService struct {
	got       *Repository
	ret       *Repository
	err       *errors.ServiceError
	deletedID string
}

func (s *stubRepositoryService) Get(ctx context.Context, id string) (*Repository, *errors.ServiceError) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubRepositoryService) Create(ctx context.Context, r *Repository) (*Repository, *errors.ServiceError) {
	s.got = r
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubRepositoryService) Replace(ctx context.Context, r *Repository) (*Repository, *errors.ServiceError) {
	s.got = r
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubRepositoryService) Delete(ctx context.Context, id string) *errors.ServiceError {
	s.deletedID = id
	return s.err
}
func (s *stubRepositoryService) All(ctx context.Context) (RepositoryList, *errors.ServiceError) {
	return RepositoryList{}, s.err
}
func (s *stubRepositoryService) FindByIDs(ctx context.Context, ids []string) (RepositoryList, *errors.ServiceError) {
	return RepositoryList{}, s.err
}

func sampleRepository() *Repository {
	r := &Repository{Name: "hypershell", Url: "https://example.com/r", Provider: "github", SecretSourceId: "ss-1"}
	r.ID = "repo-1"
	return r
}

func TestRepositoryToProto(t *testing.T) {
	r := sampleRepository()
	branch := "main"
	r.DefaultBranch = &branch
	got := repositoryToProto(r)
	if got.GetMetadata().GetId() != "repo-1" {
		t.Fatalf("id = %q, want repo-1", got.GetMetadata().GetId())
	}
	if got.GetMetadata().GetKind() != "Repository" {
		t.Fatalf("kind = %q, want Repository", got.GetMetadata().GetKind())
	}
	if got.GetMetadata().GetHref() != "/api/hypershell/ext/repositories/repo-1" {
		t.Fatalf("href = %q", got.GetMetadata().GetHref())
	}
	if got.GetName() != "hypershell" || got.GetProvider() != "github" || got.GetDefaultBranch() != "main" {
		t.Fatalf("field mapping wrong: %+v", got)
	}
}

func TestCreateRepository_ValidatesRequiredFields(t *testing.T) {
	h := &repositoryGRPCHandler{service: &stubRepositoryService{}}
	cases := []*pb.CreateRepositoryRequest{
		{Url: "u", Provider: "github", SecretSourceId: "ss-1"},
		{Name: "n", Provider: "github", SecretSourceId: "ss-1"},
		{Name: "n", Url: "u", SecretSourceId: "ss-1"},
		{Name: "n", Url: "u", Provider: "github"},
	}
	for i, req := range cases {
		if _, err := h.CreateRepository(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("case %d: code = %s, want InvalidArgument", i, status.Code(err))
		}
	}
}

func TestCreateRepository_Success(t *testing.T) {
	svc := &stubRepositoryService{ret: sampleRepository()}
	h := &repositoryGRPCHandler{service: svc}
	resp, err := h.CreateRepository(context.Background(), &pb.CreateRepositoryRequest{
		Name: "hypershell", Url: "https://example.com/r", Provider: "github", SecretSourceId: "ss-1",
	})
	if err != nil {
		t.Fatalf("CreateRepository error = %v", err)
	}
	if resp.GetRepository().GetMetadata().GetId() != "repo-1" {
		t.Fatalf("id = %q, want repo-1", resp.GetRepository().GetMetadata().GetId())
	}
	if svc.got.Name != "hypershell" || svc.got.SecretSourceId != "ss-1" {
		t.Fatalf("service received wrong model: %+v", svc.got)
	}
}

func TestGetRepository_NotFound(t *testing.T) {
	h := &repositoryGRPCHandler{service: &stubRepositoryService{err: errors.NotFound("missing")}}
	if _, err := h.GetRepository(context.Background(), &pb.GetRepositoryRequest{Id: "repo-x"}); status.Code(err) != codes.NotFound {
		t.Fatalf("code = %s, want NotFound", status.Code(err))
	}
}

func TestGetRepository_ValidatesID(t *testing.T) {
	h := &repositoryGRPCHandler{service: &stubRepositoryService{}}
	if _, err := h.GetRepository(context.Background(), &pb.GetRepositoryRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %s, want InvalidArgument", status.Code(err))
	}
}

func TestUpdateRepository_AppliesFields(t *testing.T) {
	svc := &stubRepositoryService{ret: sampleRepository()}
	svc.ret.Status = nil
	// Get returns the same stub.ret; Replace records the applied model.
	h := &repositoryGRPCHandler{service: svc}
	newBranch := "develop"
	if _, err := h.UpdateRepository(context.Background(), &pb.UpdateRepositoryRequest{Id: "repo-1", DefaultBranch: &newBranch}); err != nil {
		t.Fatalf("UpdateRepository error = %v", err)
	}
	if svc.got.DefaultBranch == nil || *svc.got.DefaultBranch != "develop" {
		t.Fatalf("default_branch not applied: %+v", svc.got)
	}
}

func TestDeleteRepository_Success(t *testing.T) {
	svc := &stubRepositoryService{}
	h := &repositoryGRPCHandler{service: svc}
	if _, err := h.DeleteRepository(context.Background(), &pb.DeleteRepositoryRequest{Id: "repo-1"}); err != nil {
		t.Fatalf("DeleteRepository error = %v", err)
	}
	if svc.deletedID != "repo-1" {
		t.Fatalf("deleted id = %q, want repo-1", svc.deletedID)
	}
}
