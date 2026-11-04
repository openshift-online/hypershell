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
// gateway-ownership answer for the SEC-RBAC-25 gate, so the import handlers can be
// tested in isolation.
type captureCITrustService struct {
	getResult *CITrust
	getErr    *errors.ServiceError
	created   *CITrust
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
// Verifies: SEC-RBAC-25.01, PLAT-DATA-25
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
// Verifies: SEC-RBAC-25.02
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
// Verifies: SEC-RBAC-25.03
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
// Verifies: SEC-RBAC-25
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

// An import reconciler may not prune a CITrust it does not own, so one Repository's
// import never deletes another's or a REST-created trust.
// Verifies: SEC-RBAC-25
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
