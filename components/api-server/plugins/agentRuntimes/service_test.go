package agentRuntimes_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/plugins/agentRuntimes"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// fakeAgentRuntimeDao is an in-memory AgentRuntimeDao for validation tests.
type fakeAgentRuntimeDao struct {
	repositoryExists bool
}

func (f *fakeAgentRuntimeDao) Get(ctx context.Context, id string) (*agentRuntimes.AgentRuntime, error) {
	return &agentRuntimes.AgentRuntime{}, nil
}

func (f *fakeAgentRuntimeDao) Create(ctx context.Context, ar *agentRuntimes.AgentRuntime) (*agentRuntimes.AgentRuntime, error) {
	return ar, nil
}

func (f *fakeAgentRuntimeDao) Replace(ctx context.Context, ar *agentRuntimes.AgentRuntime) (*agentRuntimes.AgentRuntime, error) {
	return ar, nil
}

func (f *fakeAgentRuntimeDao) Delete(ctx context.Context, id string) error { return nil }

func (f *fakeAgentRuntimeDao) FindByIDs(ctx context.Context, ids []string) (agentRuntimes.AgentRuntimeList, error) {
	return agentRuntimes.AgentRuntimeList{}, nil
}

func (f *fakeAgentRuntimeDao) All(ctx context.Context) (agentRuntimes.AgentRuntimeList, error) {
	return agentRuntimes.AgentRuntimeList{}, nil
}

func (f *fakeAgentRuntimeDao) RepositoryExists(ctx context.Context, id string) (bool, error) {
	return f.repositoryExists, nil
}

func strPtr(s string) *string { return &s }

func TestCreate_SelectorWithoutRepository_422(t *testing.T) {
	svc := agentRuntimes.NewAgentRuntimeService(&fakeAgentRuntimeDao{})
	ar := &agentRuntimes.AgentRuntime{
		Name:      "reviewer",
		ClusterId: "c-1",
		GatewayId: "g-1",
		Selector:  []string{"agent/reviewable"},
	}
	_, err := svc.Create(context.Background(), ar)
	if err == nil || err.Code != errors.ErrorUnprocessableEntity {
		t.Fatalf("expected 422 unprocessable entity, got %v", err)
	}
	if err.HttpCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected HTTP 422, got %d", err.HttpCode)
	}
}

func TestCreate_SelectorWithRepository_OK(t *testing.T) {
	svc := agentRuntimes.NewAgentRuntimeService(&fakeAgentRuntimeDao{repositoryExists: true})
	ar := &agentRuntimes.AgentRuntime{
		Name:         "reviewer",
		ClusterId:    "c-1",
		GatewayId:    "g-1",
		RepositoryId: strPtr("repo-1"),
		Selector:     []string{"agent/reviewable", "agent/ready"},
	}
	if _, err := svc.Create(context.Background(), ar); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreate_RepositoryDoesNotExist(t *testing.T) {
	svc := agentRuntimes.NewAgentRuntimeService(&fakeAgentRuntimeDao{repositoryExists: false})
	ar := &agentRuntimes.AgentRuntime{
		Name:         "reviewer",
		ClusterId:    "c-1",
		GatewayId:    "g-1",
		RepositoryId: strPtr("missing"),
	}
	_, err := svc.Create(context.Background(), ar)
	if err == nil || err.Code != errors.ErrorValidation {
		t.Fatalf("expected validation error for missing repository, got %v", err)
	}
}

func TestCreate_NoSelectorNoRepository_OK(t *testing.T) {
	svc := agentRuntimes.NewAgentRuntimeService(&fakeAgentRuntimeDao{})
	ar := &agentRuntimes.AgentRuntime{
		Name:      "plain",
		ClusterId: "c-1",
		GatewayId: "g-1",
	}
	if _, err := svc.Create(context.Background(), ar); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
