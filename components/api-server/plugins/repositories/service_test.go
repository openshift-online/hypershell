package repositories_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/plugins/repositories"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
)

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
	svc := repositories.NewRepositoryService(&fakeRepositoryDao{secretSourceExists: true})
	repo := validRepo()
	repo.Provider = "gitlab"
	_, err := svc.Create(context.Background(), repo)
	if err == nil || err.Code != errors.ErrorValidation {
		t.Fatalf("expected validation error for unsupported provider, got %v", err)
	}
}

func TestCreate_MissingSecretSource(t *testing.T) {
	svc := repositories.NewRepositoryService(&fakeRepositoryDao{secretSourceExists: false})
	_, err := svc.Create(context.Background(), validRepo())
	if err == nil || err.Code != errors.ErrorValidation {
		t.Fatalf("expected validation error for missing secret source, got %v", err)
	}
}

func TestCreate_DefaultsImportPath(t *testing.T) {
	dao := &fakeRepositoryDao{secretSourceExists: true}
	svc := repositories.NewRepositoryService(dao)
	if _, err := svc.Create(context.Background(), validRepo()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dao.created.ImportPath == nil || *dao.created.ImportPath != ".hypershell" {
		t.Fatalf("expected import_path defaulted to .hypershell, got %v", dao.created.ImportPath)
	}
}

func TestDelete_BlockedByReferencingAgentRuntime(t *testing.T) {
	svc := repositories.NewRepositoryService(&fakeRepositoryDao{referencingRuntimes: []string{"ar-1"}})
	err := svc.Delete(context.Background(), "repo-1")
	if err == nil || err.Code != errors.ErrorConflict {
		t.Fatalf("expected 409 conflict, got %v", err)
	}
	if err.HttpCode != http.StatusConflict {
		t.Fatalf("expected HTTP 409, got %d", err.HttpCode)
	}
}

func TestDelete_Unreferenced(t *testing.T) {
	svc := repositories.NewRepositoryService(&fakeRepositoryDao{})
	if err := svc.Delete(context.Background(), "repo-1"); err != nil {
		t.Fatalf("unexpected error deleting unreferenced repository: %v", err)
	}
}
