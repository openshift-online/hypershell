package repositories

import (
	"context"
	"strings"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// supportedProviders is the set of provider types a Repository credential may target.
var supportedProviders = map[string]bool{
	"github": true,
}

// defaultImportPath is applied when a Repository is registered without an explicit import_path.
const defaultImportPath = ".hypershell"

type RepositoryService interface {
	Get(ctx context.Context, id string) (*Repository, *errors.ServiceError)
	Create(ctx context.Context, repository *Repository) (*Repository, *errors.ServiceError)
	Replace(ctx context.Context, repository *Repository) (*Repository, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (RepositoryList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (RepositoryList, *errors.ServiceError)
}

func NewRepositoryService(dao RepositoryDao) RepositoryService {
	return &sqlRepositoryService{dao: dao}
}

var _ RepositoryService = &sqlRepositoryService{}

type sqlRepositoryService struct {
	dao RepositoryDao
}

func (s *sqlRepositoryService) Get(ctx context.Context, id string) (*Repository, *errors.ServiceError) {
	item, err := s.dao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("Repository", "id", id, err)
	}
	return item, nil
}

// validate enforces the provider vocabulary and the SecretSource foreign key.
func (s *sqlRepositoryService) validate(ctx context.Context, repository *Repository) *errors.ServiceError {
	if !supportedProviders[repository.Provider] {
		return errors.Validation("unsupported provider %q; supported providers: github", repository.Provider)
	}
	if repository.SecretSourceId == "" {
		return errors.Validation("secret_source_id is required")
	}
	exists, err := s.dao.SecretSourceExists(ctx, repository.SecretSourceId)
	if err != nil {
		return errors.GeneralError("Unable to validate secret_source_id: %s", err)
	}
	if !exists {
		return errors.Validation("secret_source_id %q does not reference an existing SecretSource", repository.SecretSourceId)
	}
	return nil
}

func (s *sqlRepositoryService) Create(ctx context.Context, repository *Repository) (*Repository, *errors.ServiceError) {
	if repository.ImportPath == nil || *repository.ImportPath == "" {
		p := defaultImportPath
		repository.ImportPath = &p
	}
	if svcErr := s.validate(ctx, repository); svcErr != nil {
		return nil, svcErr
	}
	item, err := s.dao.Create(ctx, repository)
	if err != nil {
		return nil, errors.GeneralError("Unable to create repository: %s", err)
	}
	return item, nil
}

func (s *sqlRepositoryService) Replace(ctx context.Context, repository *Repository) (*Repository, *errors.ServiceError) {
	if repository.ImportPath == nil || *repository.ImportPath == "" {
		p := defaultImportPath
		repository.ImportPath = &p
	}
	if svcErr := s.validate(ctx, repository); svcErr != nil {
		return nil, svcErr
	}
	item, err := s.dao.Replace(ctx, repository)
	if err != nil {
		return nil, errors.GeneralError("Unable to update repository: %s", err)
	}
	return item, nil
}

func (s *sqlRepositoryService) Delete(ctx context.Context, id string) *errors.ServiceError {
	refs, err := s.dao.ReferencingAgentRuntimeIDs(ctx, id)
	if err != nil {
		return errors.GeneralError("Unable to check repository references: %s", err)
	}
	if len(refs) > 0 {
		return errors.Conflict("Repository %s is still referenced by AgentRuntime(s): %s", id, strings.Join(refs, ", "))
	}
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("Unable to delete repository: %s", err)
	}
	return nil
}

func (s *sqlRepositoryService) All(ctx context.Context) (RepositoryList, *errors.ServiceError) {
	items, err := s.dao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all repositories: %s", err)
	}
	return items, nil
}

func (s *sqlRepositoryService) FindByIDs(ctx context.Context, ids []string) (RepositoryList, *errors.ServiceError) {
	items, err := s.dao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get repositories: %s", err)
	}
	return items, nil
}
