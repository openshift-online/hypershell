package repositories_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/plugins/repositories"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

// fakeEventService is a no-op services.EventService: these tests cover service
// validation and the broker events are exercised by the gRPC/integration tests.
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

// fakeRepositoryDao is an in-memory RepositoryDao for service-level validation tests.
type fakeRepositoryDao struct {
	created             *repositories.Repository
	secretSourceExists  bool
	referencingRuntimes []string
}

func (f *fakeRepositoryDao) Get(ctx context.Context, id string) (*repositories.Repository, error) {
	return &repositories.Repository{}, nil
}

func (f *fakeRepositoryDao) Create(ctx context.Context, r *repositories.Repository) (*repositories.Repository, error) {
	f.created = r
	return r, nil
}

func (f *fakeRepositoryDao) Replace(ctx context.Context, r *repositories.Repository) (*repositories.Repository, error) {
	return r, nil
}

func (f *fakeRepositoryDao) Delete(ctx context.Context, id string) error { return nil }

func (f *fakeRepositoryDao) FindByIDs(ctx context.Context, ids []string) (repositories.RepositoryList, error) {
	return repositories.RepositoryList{}, nil
}

func (f *fakeRepositoryDao) All(ctx context.Context) (repositories.RepositoryList, error) {
	return repositories.RepositoryList{}, nil
}

func (f *fakeRepositoryDao) SecretSourceExists(ctx context.Context, id string) (bool, error) {
	return f.secretSourceExists, nil
}

func (f *fakeRepositoryDao) ReferencingAgentRuntimeIDs(ctx context.Context, id string) ([]string, error) {
	return f.referencingRuntimes, nil
}

func validRepo() *repositories.Repository {
	return &repositories.Repository{
		Name:           "hypershell",
		Url:            "https://github.com/openshift-online/hypershell",
		Provider:       "github",
		SecretSourceId: "ss-1",
	}
}

func TestCreate_UnsupportedProvider(t *testing.T) {
	svc := repositories.NewRepositoryService(&fakeRepositoryDao{secretSourceExists: true}, &fakeEventService{})
	repo := validRepo()
	repo.Provider = "gitlab"
	_, err := svc.Create(context.Background(), repo)
	if err == nil || err.Code != errors.ErrorValidation {
		t.Fatalf("expected validation error for unsupported provider, got %v", err)
	}
}

func TestCreate_MissingSecretSource(t *testing.T) {
	svc := repositories.NewRepositoryService(&fakeRepositoryDao{secretSourceExists: false}, &fakeEventService{})
	_, err := svc.Create(context.Background(), validRepo())
	if err == nil || err.Code != errors.ErrorValidation {
		t.Fatalf("expected validation error for missing secret source, got %v", err)
	}
}

func TestCreate_DefaultsImportPath(t *testing.T) {
	dao := &fakeRepositoryDao{secretSourceExists: true}
	svc := repositories.NewRepositoryService(dao, &fakeEventService{})
	if _, err := svc.Create(context.Background(), validRepo()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dao.created.ImportPath == nil || *dao.created.ImportPath != ".hypershell" {
		t.Fatalf("expected import_path defaulted to .hypershell, got %v", dao.created.ImportPath)
	}
}

func TestDelete_BlockedByReferencingAgentRuntime(t *testing.T) {
	svc := repositories.NewRepositoryService(&fakeRepositoryDao{referencingRuntimes: []string{"ar-1"}}, &fakeEventService{})
	err := svc.Delete(context.Background(), "repo-1")
	if err == nil || err.Code != errors.ErrorConflict {
		t.Fatalf("expected 409 conflict, got %v", err)
	}
	if err.HttpCode != http.StatusConflict {
		t.Fatalf("expected HTTP 409, got %d", err.HttpCode)
	}
}

func TestDelete_Unreferenced(t *testing.T) {
	svc := repositories.NewRepositoryService(&fakeRepositoryDao{}, &fakeEventService{})
	if err := svc.Delete(context.Background(), "repo-1"); err != nil {
		t.Fatalf("unexpected error deleting unreferenced repository: %v", err)
	}
}
