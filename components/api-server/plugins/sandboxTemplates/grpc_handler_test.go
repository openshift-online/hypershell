package sandboxTemplates

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"google.golang.org/grpc/metadata"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	pkgrbac "github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// stubSandboxTemplateService is a minimal SandboxTemplateService for gRPC handler tests.
type stubSandboxTemplateService struct {
	got       *SandboxTemplate
	ret       *SandboxTemplate
	err       *errors.ServiceError
	deletedID string
}

func (s *stubSandboxTemplateService) Get(ctx context.Context, id string) (*SandboxTemplate, *errors.ServiceError) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubSandboxTemplateService) Create(ctx context.Context, st *SandboxTemplate) (*SandboxTemplate, *errors.ServiceError) {
	s.got = st
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubSandboxTemplateService) Replace(ctx context.Context, st *SandboxTemplate) (*SandboxTemplate, *errors.ServiceError) {
	s.got = st
	if s.err != nil {
		return nil, s.err
	}
	return s.ret, nil
}
func (s *stubSandboxTemplateService) Delete(ctx context.Context, id string) *errors.ServiceError {
	s.deletedID = id
	return s.err
}
func (s *stubSandboxTemplateService) All(ctx context.Context) (SandboxTemplateList, *errors.ServiceError) {
	return SandboxTemplateList{}, s.err
}
func (s *stubSandboxTemplateService) FindByIDs(ctx context.Context, ids []string) (SandboxTemplateList, *errors.ServiceError) {
	return SandboxTemplateList{}, s.err
}

func sampleSandboxTemplate() *SandboxTemplate {
	st := &SandboxTemplate{Name: "default", Image: "quay.io/example/sandbox:latest"}
	st.ID = "st-1"
	return st
}

func TestSandboxTemplateToProto(t *testing.T) {
	st := sampleSandboxTemplate()
	got := sandboxTemplateToProto(st)
	if got.GetMetadata().GetId() != "st-1" {
		t.Fatalf("id = %q, want st-1", got.GetMetadata().GetId())
	}
	if got.GetMetadata().GetKind() != "SandboxTemplate" {
		t.Fatalf("kind = %q, want SandboxTemplate", got.GetMetadata().GetKind())
	}
	if got.GetMetadata().GetHref() != "/api/hypershell/ext/sandbox_templates/st-1" {
		t.Fatalf("href = %q", got.GetMetadata().GetHref())
	}
	if got.GetName() != "default" || got.GetImage() != "quay.io/example/sandbox:latest" {
		t.Fatalf("field mapping wrong: %+v", got)
	}
}

func TestCreateSandboxTemplate_ValidatesRequiredFields(t *testing.T) {
	h := &sandboxTemplateGRPCHandler{service: &stubSandboxTemplateService{}}
	cases := []*pb.CreateSandboxTemplateRequest{
		{Image: "img"},
		{Name: "n"},
	}
	for i, req := range cases {
		if _, err := h.CreateSandboxTemplate(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("case %d: code = %s, want InvalidArgument", i, status.Code(err))
		}
	}
}

func TestCreateSandboxTemplate_Success(t *testing.T) {
	svc := &stubSandboxTemplateService{ret: sampleSandboxTemplate()}
	h := &sandboxTemplateGRPCHandler{service: svc}
	resp, err := h.CreateSandboxTemplate(context.Background(), &pb.CreateSandboxTemplateRequest{
		Name: "default", Image: "quay.io/example/sandbox:latest",
	})
	if err != nil {
		t.Fatalf("CreateSandboxTemplate error = %v", err)
	}
	if resp.GetSandboxTemplate().GetMetadata().GetId() != "st-1" {
		t.Fatalf("id = %q, want st-1", resp.GetSandboxTemplate().GetMetadata().GetId())
	}
	if svc.got.Name != "default" || svc.got.Image != "quay.io/example/sandbox:latest" {
		t.Fatalf("service received wrong model: %+v", svc.got)
	}
}

func TestGetSandboxTemplate_NotFound(t *testing.T) {
	h := &sandboxTemplateGRPCHandler{service: &stubSandboxTemplateService{err: errors.NotFound("missing")}}
	if _, err := h.GetSandboxTemplate(context.Background(), &pb.GetSandboxTemplateRequest{Id: "st-x"}); status.Code(err) != codes.NotFound {
		t.Fatalf("code = %s, want NotFound", status.Code(err))
	}
}

func TestGetSandboxTemplate_ValidatesID(t *testing.T) {
	h := &sandboxTemplateGRPCHandler{service: &stubSandboxTemplateService{}}
	if _, err := h.GetSandboxTemplate(context.Background(), &pb.GetSandboxTemplateRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %s, want InvalidArgument", status.Code(err))
	}
}

func TestUpdateSandboxTemplate_AppliesFields(t *testing.T) {
	svc := &stubSandboxTemplateService{ret: sampleSandboxTemplate()}
	h := &sandboxTemplateGRPCHandler{service: svc}
	newImage := "quay.io/example/sandbox:v2"
	if _, err := h.UpdateSandboxTemplate(context.Background(), &pb.UpdateSandboxTemplateRequest{Id: "st-1", Image: &newImage}); err != nil {
		t.Fatalf("UpdateSandboxTemplate error = %v", err)
	}
	if svc.got.Image != "quay.io/example/sandbox:v2" {
		t.Fatalf("image not applied: %+v", svc.got)
	}
}

func TestDeleteSandboxTemplate_Success(t *testing.T) {
	svc := &stubSandboxTemplateService{ret: sampleSandboxTemplate()}
	h := &sandboxTemplateGRPCHandler{service: svc}
	if _, err := h.DeleteSandboxTemplate(context.Background(), &pb.DeleteSandboxTemplateRequest{Id: "st-1"}); err != nil {
		t.Fatalf("DeleteSandboxTemplate error = %v", err)
	}
	if svc.deletedID != "st-1" {
		t.Fatalf("deleted id = %q, want st-1", svc.deletedID)
	}
}

// TestCreateSandboxTemplate_ImportOwnershipBinding proves the server sets the
// repository_id ownership mark from the reconciled-Repository metadata and ignores a
// client-supplied repository_id, so ownership cannot be forged (rbac-enforcement.spec.md
// "Import reconciler writes an owned ext resource and ownership cannot be forged").
// Verifies: SEC-RBAC-09.06, CTRL-CORE-06.08
func TestCreateSandboxTemplate_ImportOwnershipBinding(t *testing.T) {
	svc := &stubSandboxTemplateService{ret: sampleSandboxTemplate()}
	h := &sandboxTemplateGRPCHandler{service: svc}
	forged := "repo-2"
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs(pkgrbac.ImportRepositoryMetadataKey, "repo-1"))

	_, err := h.CreateSandboxTemplate(ctx, &pb.CreateSandboxTemplateRequest{
		Name: "n", Image: "img", RepositoryId: &forged,
	})
	if err != nil {
		t.Fatalf("CreateSandboxTemplate: %v", err)
	}
	if svc.got.RepositoryId == nil || *svc.got.RepositoryId != "repo-1" {
		t.Fatalf("repository_id = %v, want repo-1 from metadata (client value ignored)", svc.got.RepositoryId)
	}
}

// TestCreateSandboxTemplate_NoImportContextLeavesOwnershipUnset proves a direct
// create (no import metadata) persists no ownership mark, so it is treated as created
// directly and never pruned by the import.
func TestCreateSandboxTemplate_NoImportContextLeavesOwnershipUnset(t *testing.T) {
	svc := &stubSandboxTemplateService{ret: sampleSandboxTemplate()}
	h := &sandboxTemplateGRPCHandler{service: svc}
	forged := "repo-2"

	if _, err := h.CreateSandboxTemplate(context.Background(), &pb.CreateSandboxTemplateRequest{
		Name: "n", Image: "img", RepositoryId: &forged,
	}); err != nil {
		t.Fatalf("CreateSandboxTemplate: %v", err)
	}
	if svc.got.RepositoryId != nil {
		t.Fatalf("repository_id = %v, want nil for a direct create", svc.got.RepositoryId)
	}
}

// TestWriteSandboxTemplate_RefusedOutsideOwningRepository proves update and delete of
// a resource owned by a different Repository (or unowned) are refused in the import
// context (rbac-enforcement.spec.md "Import write is refused outside the owning
// Repository").
// Verifies: SEC-RBAC-09.07
func TestWriteSandboxTemplate_RefusedOutsideOwningRepository(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(),
		metadata.Pairs(pkgrbac.ImportRepositoryMetadataKey, "repo-1"))
	otherOwner := "repo-2"
	owned := sampleSandboxTemplate()
	owned.RepositoryId = &otherOwner

	h := &sandboxTemplateGRPCHandler{service: &stubSandboxTemplateService{ret: owned}}

	name := "n"
	if _, err := h.UpdateSandboxTemplate(ctx, &pb.UpdateSandboxTemplateRequest{Id: "st-1", Name: &name}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("update: code = %s, want PermissionDenied", status.Code(err))
	}
	if _, err := h.DeleteSandboxTemplate(ctx, &pb.DeleteSandboxTemplateRequest{Id: "st-1"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("delete: code = %s, want PermissionDenied", status.Code(err))
	}
}
