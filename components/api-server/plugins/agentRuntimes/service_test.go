package agentRuntimes_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/plugins/agentRuntimes"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// fakeEventService is a no-op services.EventService for validation tests: the
// service emits a broker event on every successful create/update/delete, so the
// constructor needs an event service even though these tests do not assert on
// the emitted events.
type fakeEventService struct{}

func (f *fakeEventService) Get(ctx context.Context, id string) (*api.Event, *errors.ServiceError) {
	return &api.Event{}, nil
}
func (f *fakeEventService) Create(ctx context.Context, event *api.Event) (*api.Event, *errors.ServiceError) {
	return event, nil
}
func (f *fakeEventService) Replace(ctx context.Context, event *api.Event) (*api.Event, *errors.ServiceError) {
	return event, nil
}
func (f *fakeEventService) Delete(ctx context.Context, id string) *errors.ServiceError { return nil }
func (f *fakeEventService) All(ctx context.Context) (api.EventList, *errors.ServiceError) {
	return api.EventList{}, nil
}
func (f *fakeEventService) FindByIDs(ctx context.Context, ids []string) (api.EventList, *errors.ServiceError) {
	return api.EventList{}, nil
}
func (f *fakeEventService) FindUnreconciled(ctx context.Context, olderThan time.Duration) (api.EventList, *errors.ServiceError) {
	return api.EventList{}, nil
}
func (f *fakeEventService) FindBySourceAndType(ctx context.Context, source string, eventType api.EventType) (api.EventList, *errors.ServiceError) {
	return api.EventList{}, nil
}

// fakeAgentRuntimeDao is an in-memory AgentRuntimeDao for validation tests.
type fakeAgentRuntimeDao struct {
	repositoryExists bool
}

func (f *fakeAgentRuntimeDao) Get(ctx context.Context, id string) (*agentRuntimes.AgentRuntime, error) {
	return &agentRuntimes.AgentRuntime{}, nil
}

func (f *fakeAgentRuntimeDao) GetUnscoped(ctx context.Context, id string) (*agentRuntimes.AgentRuntime, error) {
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
	svc := agentRuntimes.NewAgentRuntimeService(&fakeAgentRuntimeDao{}, &fakeEventService{})
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
	svc := agentRuntimes.NewAgentRuntimeService(&fakeAgentRuntimeDao{repositoryExists: true}, &fakeEventService{})
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
	svc := agentRuntimes.NewAgentRuntimeService(&fakeAgentRuntimeDao{repositoryExists: false}, &fakeEventService{})
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
	svc := agentRuntimes.NewAgentRuntimeService(&fakeAgentRuntimeDao{}, &fakeEventService{})
	ar := &agentRuntimes.AgentRuntime{
		Name:      "plain",
		ClusterId: "c-1",
		GatewayId: "g-1",
	}
	if _, err := svc.Create(context.Background(), ar); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// cluster_id is optional: the scheduling cluster is derived from the referenced
// Gateway's placement, so a fully declarable agent omits it entirely.
func TestCreate_NoClusterID_OK(t *testing.T) {
	svc := agentRuntimes.NewAgentRuntimeService(&fakeAgentRuntimeDao{}, &fakeEventService{})
	ar := &agentRuntimes.AgentRuntime{
		Name:      "cluster-free",
		GatewayId: "g-1",
	}
	if _, err := svc.Create(context.Background(), ar); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
