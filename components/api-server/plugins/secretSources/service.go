package secretSources

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type SecretSourceService interface {
	Get(ctx context.Context, id string) (*SecretSource, *errors.ServiceError)
	Create(ctx context.Context, secretSource *SecretSource) (*SecretSource, *errors.ServiceError)
	Replace(ctx context.Context, secretSource *SecretSource) (*SecretSource, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (SecretSourceList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (SecretSourceList, *errors.ServiceError)
}

func NewSecretSourceService(dao SecretSourceDao) SecretSourceService {
	return &sqlSecretSourceService{dao: dao}
}

var _ SecretSourceService = &sqlSecretSourceService{}

type sqlSecretSourceService struct {
	dao SecretSourceDao
}

func (s *sqlSecretSourceService) Get(ctx context.Context, id string) (*SecretSource, *errors.ServiceError) {
	item, err := s.dao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("SecretSource", "id", id, err)
	}
	return item, nil
}

func (s *sqlSecretSourceService) Create(ctx context.Context, secretSource *SecretSource) (*SecretSource, *errors.ServiceError) {
	item, err := s.dao.Create(ctx, secretSource)
	if err != nil {
		return nil, errors.GeneralError("Unable to create secret source: %s", err)
	}
	return item, nil
}

func (s *sqlSecretSourceService) Replace(ctx context.Context, secretSource *SecretSource) (*SecretSource, *errors.ServiceError) {
	item, err := s.dao.Replace(ctx, secretSource)
	if err != nil {
		return nil, errors.GeneralError("Unable to update secret source: %s", err)
	}
	return item, nil
}

func (s *sqlSecretSourceService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("Unable to delete secret source: %s", err)
	}
	return nil
}

func (s *sqlSecretSourceService) All(ctx context.Context) (SecretSourceList, *errors.ServiceError) {
	items, err := s.dao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all secret sources: %s", err)
	}
	return items, nil
}

func (s *sqlSecretSourceService) FindByIDs(ctx context.Context, ids []string) (SecretSourceList, *errors.ServiceError) {
	items, err := s.dao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get secret sources: %s", err)
	}
	return items, nil
}
