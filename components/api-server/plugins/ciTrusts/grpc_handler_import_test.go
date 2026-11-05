package ciTrusts

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	pkgrbac "github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// importCtx returns a context carrying the reconciled-Repository import binding, the
// trusted metadata the server reads ownership from.
func importCtx(repositoryID string) context.Context {
	md := metadata.Pairs(pkgrbac.ImportRepositoryMetadataKey, repositoryID)
	return metadata.NewIncomingContext(context.Background(), md)
}

// captureCITrustService records writes and serves canned reads, plus a configurable
// gateway-ownership answer for the SEC-RBAC-26 gate, so the import handlers can be
// tested in isolation.
type captureCITrustService struct {
	getResult *CITrust
	getErr    *errors.ServiceError
	created   *CITrust
	replaced  *CITrust
	deletedID string

	gatewayOwner      *string
	gatewayFound      bool
	gatewayOwnerErr   *errors.ServiceError
	lastGatewayLookup string
}

func (s *captureCITrustService) Get(context.Context, string) (*CITrust, *errors.ServiceError) {
	return s.getResult, s.getErr
}
func (s *captureCITrustService) Create(_ context.Context, ct *CITrust) (*CITrust, *errors.ServiceError) {
	s.created = ct
	ct.ID = "ct-new"
	return ct, nil
}
func (s *captureCITrustService) Replace(_ context.Context, ct *CITrust) (*CITrust, *errors.ServiceError) {
	s.replaced = ct
	return ct, nil
}
func (s *captureCITrustService) Delete(_ context.Context, id string) *errors.ServiceError {
	s.deletedID = id
	return nil
}
func (s *captureCITrustService) All(context.Context) (CITrustList, *errors.ServiceError) {
	return nil, nil
}
func (s *captureCITrustService) FindByIDs(context.Context, []string) (CITrustList, *errors.ServiceError) {
	return nil, nil
}
func (s *captureCITrustService) GatewayRepositoryID(_ context.Context, gatewayID string) (*string, bool, *errors.ServiceError) {
	s.lastGatewayLookup = gatewayID
	return s.gatewayOwner, s.gatewayFound, s.gatewayOwnerErr
}

func importCreateReq() *pb.CreateCITrustRequest {
	return &pb.CreateCITrustRequest{
		Name:         strPtr("github-actions-hypershell"),
		CiProviderId: "cp-1",
		GatewayId:    "gw-1",
		Role:         "openshell-user",
	}
}

// An import create on a gateway the SAME Repository owns is authorized, and the server
// sets repository_id from the trusted metadata (never the request body).
// Verifies: SEC-RBAC-26.01, PLAT-DATA-25
func TestCreateCITrust_ImportSetsOwnershipOnOwnedGateway(t *testing.T) {
	svc := &captureCITrustService{gatewayOwner: strPtr("repo-1"), gatewayFound: true}
	h := &ciTrustGRPCHandler{service: svc}

	resp, err := h.CreateCITrust(importCtx("repo-1"), importCreateReq())
	if err != nil {
		t.Fatalf("CreateCITrust error = %v", err)
	}
	if svc.created == nil || svc.created.RepositoryID == nil || *svc.created.RepositoryID != "repo-1" {
		t.Fatalf("created repository_id = %v, want repo-1 set from import metadata", svc.created.RepositoryID)
	}
	if svc.created.Name != "github-actions-hypershell" {
		t.Errorf("created name = %q, want the manifest metadata.name preserved", svc.created.Name)
	}
	if resp.GetCiTrust() == nil {
		t.Error("expected a CITrust in the response")
	}
}

// An import create on a gateway owned by ANOTHER Repository (or unowned / nonexistent)
// is refused.
// Verifies: SEC-RBAC-26.02
func TestCreateCITrust_ImportRejectedOnForeignGateway(t *testing.T) {
	cases := map[string]*captureCITrustService{
		"owned by another repository": {gatewayOwner: strPtr("repo-2"), gatewayFound: true},
		"unowned (REST-created)":      {gatewayOwner: nil, gatewayFound: true},
		"nonexistent gateway":         {gatewayOwner: nil, gatewayFound: false},
	}
	for name, svc := range cases {
		t.Run(name, func(t *testing.T) {
			h := &ciTrustGRPCHandler{service: svc}
			_, err := h.CreateCITrust(importCtx("repo-1"), importCreateReq())
			if status.Code(err) != codes.PermissionDenied {
				t.Fatalf("code = %s, want PermissionDenied", status.Code(err))
			}
			if svc.created != nil {
				t.Fatalf("a create was issued despite a foreign gateway: %+v", svc.created)
			}
		})
	}
}

// With no import context the gateway-ownership gate is skipped (the human HTTP tenancy
// path, SEC-RBAC-22, is unchanged) and no ownership mark is set.
// Verifies: SEC-RBAC-26.03
func TestCreateCITrust_NoImportContextSkipsGateAndOwnership(t *testing.T) {
	svc := &captureCITrustService{}
	h := &ciTrustGRPCHandler{service: svc}

	_, err := h.CreateCITrust(context.Background(), importCreateReq())
	if err != nil {
		t.Fatalf("CreateCITrust error = %v", err)
	}
	if svc.lastGatewayLookup != "" {
		t.Errorf("gateway ownership was looked up without an import context: %q", svc.lastGatewayLookup)
	}
	if svc.created.RepositoryID != nil {
		t.Errorf("repository_id = %v, want nil without an import context", svc.created.RepositoryID)
	}
}

// An import reconciler may not update a CITrust owned by another Repository or created
// directly over REST.
// Verifies: SEC-RBAC-26
func TestUpdateCITrust_EnforcesOwnership(t *testing.T) {
	otherRepo := "repo-2"
	found := &CITrust{Name: "ct", RepositoryID: &otherRepo}
	found.ID = "ct-1"
	svc := &captureCITrustService{getResult: found}
	h := &ciTrustGRPCHandler{service: svc}

	if _, err := h.UpdateCITrust(importCtx("repo-1"), &pb.UpdateCITrustRequest{Id: "ct-1"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("cross-repository update: code = %s, want PermissionDenied", status.Code(err))
	}
}

// An import reconciler owning the trust may reconcile a changed subject predicate,
// including subject_repository and subject_project_path, so a manifest subject change
// is applied rather than silently skipped (reconcile, don't create-or-skip; FINDING 2).
// Before the fix UpdateCITrustRequest carried neither field, so the gRPC update path
// could not persist a changed subject.repository/project_path.
func TestUpdateCITrust_AppliesSubjectRepositoryAndProjectPath(t *testing.T) {
	oldRepo, oldPath := "acme/old", "grp/old"
	owned := &CITrust{Name: "ct", RepositoryID: strPtr("repo-1"), Role: "openshell-user", SubjectRepository: &oldRepo, SubjectProjectPath: &oldPath}
	owned.ID = "ct-1"
	svc := &captureCITrustService{getResult: owned}
	h := &ciTrustGRPCHandler{service: svc}

	newRepo, newPath := "acme/new", "grp/new"
	role := "openshell-user"
	resp, err := h.UpdateCITrust(importCtx("repo-1"), &pb.UpdateCITrustRequest{
		Id:                 "ct-1",
		Role:               &role,
		SubjectRepository:  &newRepo,
		SubjectProjectPath: &newPath,
	})
	if err != nil {
		t.Fatalf("UpdateCITrust error = %v", err)
	}
	if got := resp.GetCiTrust().GetSubjectRepository(); got != newRepo {
		t.Errorf("subject_repository = %q, want %q", got, newRepo)
	}
	if got := resp.GetCiTrust().GetSubjectProjectPath(); got != newPath {
		t.Errorf("subject_project_path = %q, want %q", got, newPath)
	}
}

// An import is DESIRED STATE: a manifest that omits a previously-set subject field
// clears it. The import update path REPLACES the mutable subject predicate wholesale,
// so a nil field in the request clears the stored value rather than leaving it stale
// (FINDING 1). Before the fix the import path applied subject fields only when
// non-nil (the human sparse-PATCH rule), so a removed subject field was silently kept
// -> false convergence and churn.
func TestUpdateCITrust_ImportClearsOmittedSubjectField(t *testing.T) {
	oldRef, oldEnv := "refs/heads/main", "prod"
	owned := &CITrust{
		Name:               "ct",
		RepositoryID:       strPtr("repo-1"),
		Role:               "openshell-user",
		SubjectRepository:  strPtr("acme/repo"),
		SubjectRef:         &oldRef,
		SubjectEnvironment: &oldEnv,
	}
	owned.ID = "ct-1"
	svc := &captureCITrustService{getResult: owned}
	h := &ciTrustGRPCHandler{service: svc}

	// The manifest now declares only subject.repository and role; ref and environment
	// are gone.
	keepRepo := "acme/repo"
	role := "openshell-user"
	resp, err := h.UpdateCITrust(importCtx("repo-1"), &pb.UpdateCITrustRequest{
		Id:                "ct-1",
		Role:              &role,
		SubjectRepository: &keepRepo,
	})
	if err != nil {
		t.Fatalf("UpdateCITrust error = %v", err)
	}
	if got := resp.GetCiTrust().GetSubjectRef(); got != "" {
		t.Errorf("subject_ref = %q, want cleared (import replace semantics)", got)
	}
	if got := resp.GetCiTrust().GetSubjectEnvironment(); got != "" {
		t.Errorf("subject_environment = %q, want cleared (import replace semantics)", got)
	}
	if got := resp.GetCiTrust().GetSubjectRepository(); got != keepRepo {
		t.Errorf("subject_repository = %q, want %q retained", got, keepRepo)
	}
}

// role is required on the import update path too, symmetric with CreateCITrust
// (FINDING 1). Because the import path REPLACES role wholesale, a manifest that drops
// role reaches here as Role="" and would persist an empty role, silently breaking the
// Keycloak token-exchange role mapping. The update must be rejected, not recorded as
// convergence. Before the fix the import path set found.Role="" with no validation and
// the Replace succeeded.
func TestUpdateCITrust_ImportRejectsEmptyRole(t *testing.T) {
	owned := &CITrust{Name: "ct", RepositoryID: strPtr("repo-1"), Role: "openshell-user"}
	owned.ID = "ct-1"
	svc := &captureCITrustService{getResult: owned}
	h := &ciTrustGRPCHandler{service: svc}

	emptyRole := ""
	_, err := h.UpdateCITrust(importCtx("repo-1"), &pb.UpdateCITrustRequest{
		Id:   "ct-1",
		Role: &emptyRole,
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("import update with empty role: code = %s, want InvalidArgument", status.Code(err))
	}
}

// The human HTTP partial-PATCH path is unchanged (SEC-RBAC-22): a nil field means
// "unchanged", so updating one subject field must NOT clear the others. This guards the
// FINDING 1 fix from regressing the human sparse-PATCH contract.
func TestUpdateCITrust_HumanPatchLeavesOmittedSubjectFieldUnchanged(t *testing.T) {
	oldRef := "refs/heads/main"
	found := &CITrust{
		Name:              "ct",
		SubjectRepository: strPtr("acme/repo"),
		SubjectRef:        &oldRef,
	}
	found.ID = "ct-1"
	svc := &captureCITrustService{getResult: found}
	h := &ciTrustGRPCHandler{service: svc}

	// No import context: human PATCH that touches only subject.repository.
	newRepo := "acme/renamed"
	resp, err := h.UpdateCITrust(context.Background(), &pb.UpdateCITrustRequest{
		Id:                "ct-1",
		SubjectRepository: &newRepo,
	})
	if err != nil {
		t.Fatalf("UpdateCITrust error = %v", err)
	}
	if got := resp.GetCiTrust().GetSubjectRef(); got != oldRef {
		t.Errorf("subject_ref = %q, want %q unchanged under human PATCH", got, oldRef)
	}
	if got := resp.GetCiTrust().GetSubjectRepository(); got != newRepo {
		t.Errorf("subject_repository = %q, want %q", got, newRepo)
	}
}

// An import reconciler may not prune a CITrust it does not own, so one Repository's
// import never deletes another's or a REST-created trust.
// Verifies: SEC-RBAC-26
func TestDeleteCITrust_EnforcesOwnership(t *testing.T) {
	restCreated := &CITrust{Name: "ct"} // no ownership mark
	restCreated.ID = "ct-1"
	svc := &captureCITrustService{getResult: restCreated}
	h := &ciTrustGRPCHandler{service: svc}

	if _, err := h.DeleteCITrust(importCtx("repo-1"), &pb.DeleteCITrustRequest{Id: "ct-1"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("prune of an unowned trust: code = %s, want PermissionDenied", status.Code(err))
	}
	if svc.deletedID != "" {
		t.Fatalf("an unowned trust was deleted: %q", svc.deletedID)
	}
}

// An import reconciler owning the trust may prune it.
func TestDeleteCITrust_AllowsOwnedPrune(t *testing.T) {
	owned := &CITrust{Name: "ct", RepositoryID: strPtr("repo-1")}
	owned.ID = "ct-1"
	svc := &captureCITrustService{getResult: owned}
	h := &ciTrustGRPCHandler{service: svc}

	if _, err := h.DeleteCITrust(importCtx("repo-1"), &pb.DeleteCITrustRequest{Id: "ct-1"}); err != nil {
		t.Fatalf("owned prune error = %v", err)
	}
	if svc.deletedID != "ct-1" {
		t.Fatalf("deleted id = %q, want ct-1", svc.deletedID)
	}
}
