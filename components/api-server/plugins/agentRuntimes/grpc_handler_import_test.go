package agentRuntimes

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

// captureAgentRuntimeService records the model passed to Create/Replace and serves a
// canned Get, so the import handlers can be tested in isolation.
type captureAgentRuntimeService struct {
	getResult  *AgentRuntime
	getErr     *errors.ServiceError
	created    *AgentRuntime
	replaced   *AgentRuntime
	deletedID  string
	deleteErr  *errors.ServiceError
	createErr  *errors.ServiceError
	replaceErr *errors.ServiceError
}

func (s *captureAgentRuntimeService) Get(context.Context, string) (*AgentRuntime, *errors.ServiceError) {
	return s.getResult, s.getErr
}
func (s *captureAgentRuntimeService) GetUnscoped(context.Context, string) (*AgentRuntime, *errors.ServiceError) {
	return s.getResult, s.getErr
}
func (s *captureAgentRuntimeService) Create(_ context.Context, ar *AgentRuntime) (*AgentRuntime, *errors.ServiceError) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	s.created = ar
	ar.ID = "ar-new"
	return ar, nil
}
func (s *captureAgentRuntimeService) Replace(_ context.Context, ar *AgentRuntime) (*AgentRuntime, *errors.ServiceError) {
	if s.replaceErr != nil {
		return nil, s.replaceErr
	}
	s.replaced = ar
	return ar, nil
}
func (s *captureAgentRuntimeService) Delete(_ context.Context, id string) *errors.ServiceError {
	s.deletedID = id
	return s.deleteErr
}
func (s *captureAgentRuntimeService) All(context.Context) (AgentRuntimeList, *errors.ServiceError) {
	return nil, nil
}
func (s *captureAgentRuntimeService) FindByIDs(context.Context, []string) (AgentRuntimeList, *errors.ServiceError) {
	return nil, nil
}
func (s *captureAgentRuntimeService) UpdateStatus(context.Context, string, string, *string) (*AgentRuntime, *errors.ServiceError) {
	return nil, nil
}

func TestCreateAgentRuntime_SetsOwnershipFromMetadata(t *testing.T) {
	svc := &captureAgentRuntimeService{}
	h := &agentRuntimeGRPCHandler{service: svc}

	_, err := h.CreateAgentRuntime(importCtx("repo-1"), &pb.CreateAgentRuntimeRequest{
		Name:      "amber",
		GatewayId: "gw-1",
	})
	if err != nil {
		t.Fatalf("CreateAgentRuntime error = %v", err)
	}
	if svc.created == nil || svc.created.RepositoryId == nil || *svc.created.RepositoryId != "repo-1" {
		t.Fatalf("created repository_id = %v, want repo-1 set from import metadata", svc.created.RepositoryId)
	}
}

func TestCreateAgentRuntime_IgnoresNoImportContext(t *testing.T) {
	svc := &captureAgentRuntimeService{}
	h := &agentRuntimeGRPCHandler{service: svc}

	_, err := h.CreateAgentRuntime(context.Background(), &pb.CreateAgentRuntimeRequest{Name: "amber", GatewayId: "gw-1"})
	if err != nil {
		t.Fatalf("CreateAgentRuntime error = %v", err)
	}
	if svc.created.RepositoryId != nil {
		t.Fatalf("repository_id = %v, want nil without an import context", svc.created.RepositoryId)
	}
}

func TestCreateAgentRuntime_Validates(t *testing.T) {
	h := &agentRuntimeGRPCHandler{service: &captureAgentRuntimeService{}}
	if _, err := h.CreateAgentRuntime(context.Background(), &pb.CreateAgentRuntimeRequest{GatewayId: "gw-1"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty name: code = %s, want InvalidArgument", status.Code(err))
	}
	if _, err := h.CreateAgentRuntime(context.Background(), &pb.CreateAgentRuntimeRequest{Name: "amber"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty gateway_id: code = %s, want InvalidArgument", status.Code(err))
	}
}

func TestUpdateAgentRuntime_EnforcesOwnership(t *testing.T) {
	otherRepo := "repo-2"
	found := &AgentRuntime{Name: "amber", RepositoryId: &otherRepo}
	found.ID = "ar-1"
	svc := &captureAgentRuntimeService{getResult: found}
	h := &agentRuntimeGRPCHandler{service: svc}

	// Import reconciler for repo-1 must not update a runtime owned by repo-2.
	if _, err := h.UpdateAgentRuntime(importCtx("repo-1"), &pb.UpdateAgentRuntimeRequest{Id: "ar-1"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("cross-repository update: code = %s, want PermissionDenied", status.Code(err))
	}
	if svc.replaced != nil {
		t.Fatal("a cross-repository update reached Replace")
	}
}

func TestUpdateAgentRuntime_SameOwnerApplies(t *testing.T) {
	repo1 := "repo-1"
	found := &AgentRuntime{Name: "amber", GatewayId: "gw-old", RepositoryId: &repo1}
	found.ID = "ar-1"
	svc := &captureAgentRuntimeService{getResult: found}
	h := &agentRuntimeGRPCHandler{service: svc}

	newGateway := "gw-new"
	_, err := h.UpdateAgentRuntime(importCtx("repo-1"), &pb.UpdateAgentRuntimeRequest{Id: "ar-1", GatewayId: &newGateway})
	if err != nil {
		t.Fatalf("UpdateAgentRuntime error = %v", err)
	}
	if svc.replaced == nil || svc.replaced.GatewayId != "gw-new" {
		t.Fatalf("replaced gateway_id = %v, want gw-new", svc.replaced)
	}
}

func TestDeleteAgentRuntime_NilFoundGuard(t *testing.T) {
	svc := &captureAgentRuntimeService{getResult: nil, getErr: nil}
	h := &agentRuntimeGRPCHandler{service: svc}

	if _, err := h.DeleteAgentRuntime(context.Background(), &pb.DeleteAgentRuntimeRequest{Id: "ar-x"}); status.Code(err) != codes.NotFound {
		t.Fatalf("nil found: code = %s, want NotFound", status.Code(err))
	}
	if svc.deletedID != "" {
		t.Fatal("Delete was called despite a nil-found guard")
	}
}

func TestDeleteAgentRuntime_EnforcesOwnership(t *testing.T) {
	otherRepo := "repo-2"
	found := &AgentRuntime{Name: "amber", RepositoryId: &otherRepo}
	found.ID = "ar-1"
	svc := &captureAgentRuntimeService{getResult: found}
	h := &agentRuntimeGRPCHandler{service: svc}

	if _, err := h.DeleteAgentRuntime(importCtx("repo-1"), &pb.DeleteAgentRuntimeRequest{Id: "ar-1"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("cross-repository delete: code = %s, want PermissionDenied", status.Code(err))
	}
	if svc.deletedID != "" {
		t.Fatal("a cross-repository delete reached Delete")
	}
}

func TestDeleteAgentRuntime_OwnedSucceeds(t *testing.T) {
	repo1 := "repo-1"
	found := &AgentRuntime{Name: "amber", RepositoryId: &repo1}
	found.ID = "ar-1"
	svc := &captureAgentRuntimeService{getResult: found}
	h := &agentRuntimeGRPCHandler{service: svc}

	if _, err := h.DeleteAgentRuntime(importCtx("repo-1"), &pb.DeleteAgentRuntimeRequest{Id: "ar-1"}); err != nil {
		t.Fatalf("DeleteAgentRuntime error = %v", err)
	}
	if svc.deletedID != "ar-1" {
		t.Fatalf("deleted id = %q, want ar-1", svc.deletedID)
	}
}

func TestListAgentRuntimes_RepositoryFilter(t *testing.T) {
	repo1 := "repo-1"
	repo2 := "repo-2"
	ar1 := &AgentRuntime{Name: "ar-1", RepositoryId: &repo1}
	ar1.ID = "a1"
	ar2 := &AgentRuntime{Name: "ar-2", RepositoryId: &repo2}
	ar2.ID = "a2"
	ar3 := &AgentRuntime{Name: "ar-3"} // unowned
	ar3.ID = "a3"
	svc := &listAgentRuntimeService{all: AgentRuntimeList{ar1, ar2, ar3}}
	h := &agentRuntimeGRPCHandler{service: svc}

	resp, err := h.ListAgentRuntimes(context.Background(), &pb.ListAgentRuntimesRequest{RepositoryId: &repo1})
	if err != nil {
		t.Fatalf("ListAgentRuntimes error = %v", err)
	}
	if len(resp.GetItems()) != 1 || resp.GetItems()[0].GetName() != "ar-1" {
		t.Fatalf("items = %+v, want only ar-1", resp.GetItems())
	}
}

// listAgentRuntimeService serves a fixed All() for the list filter test.
type listAgentRuntimeService struct {
	captureAgentRuntimeService
	all AgentRuntimeList
}

func (s *listAgentRuntimeService) All(context.Context) (AgentRuntimeList, *errors.ServiceError) {
	return s.all, nil
}
