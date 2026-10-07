package providerSpecs

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type ProviderSpecService interface {
	Get(ctx context.Context, id string) (*ProviderSpec, *errors.ServiceError)
	Create(ctx context.Context, providerSpec *ProviderSpec) (*ProviderSpec, *errors.ServiceError)
	Replace(ctx context.Context, providerSpec *ProviderSpec) (*ProviderSpec, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (ProviderSpecList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (ProviderSpecList, *errors.ServiceError)
}

func NewProviderSpecService(dao ProviderSpecDao) ProviderSpecService {
	return &sqlProviderSpecService{dao: dao}
}

var _ ProviderSpecService = &sqlProviderSpecService{}

type sqlProviderSpecService struct {
	dao ProviderSpecDao
}

func (s *sqlProviderSpecService) Get(ctx context.Context, id string) (*ProviderSpec, *errors.ServiceError) {
	item, err := s.dao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("ProviderSpec", "id", id, err)
	}
	return item, nil
}

func (s *sqlProviderSpecService) Create(ctx context.Context, providerSpec *ProviderSpec) (*ProviderSpec, *errors.ServiceError) {
	item, err := s.dao.Create(ctx, providerSpec)
	if err != nil {
		return nil, errors.GeneralError("Unable to create provider spec: %s", err)
	}
	return item, nil
}

func (s *sqlProviderSpecService) Replace(ctx context.Context, providerSpec *ProviderSpec) (*ProviderSpec, *errors.ServiceError) {
	item, err := s.dao.Replace(ctx, providerSpec)
	if err != nil {
		return nil, errors.GeneralError("Unable to update provider spec: %s", err)
	}
	return item, nil
}

func (s *sqlProviderSpecService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("Unable to delete provider spec: %s", err)
	}
	return nil
}

func (s *sqlProviderSpecService) All(ctx context.Context) (ProviderSpecList, *errors.ServiceError) {
	items, err := s.dao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all provider specs: %s", err)
	}
	return items, nil
}

func (s *sqlProviderSpecService) FindByIDs(ctx context.Context, ids []string) (ProviderSpecList, *errors.ServiceError) {
	items, err := s.dao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get provider specs: %s", err)
	}
	return items, nil
}
