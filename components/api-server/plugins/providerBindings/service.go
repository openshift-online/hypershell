package providerBindings

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type ProviderBindingService interface {
	Get(ctx context.Context, id string) (*ProviderBinding, *errors.ServiceError)
	Create(ctx context.Context, providerBinding *ProviderBinding) (*ProviderBinding, *errors.ServiceError)
	Replace(ctx context.Context, providerBinding *ProviderBinding) (*ProviderBinding, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (ProviderBindingList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (ProviderBindingList, *errors.ServiceError)
}

func NewProviderBindingService(dao ProviderBindingDao) ProviderBindingService {
	return &sqlProviderBindingService{dao: dao}
}

var _ ProviderBindingService = &sqlProviderBindingService{}

type sqlProviderBindingService struct {
	dao ProviderBindingDao
}

func (s *sqlProviderBindingService) Get(ctx context.Context, id string) (*ProviderBinding, *errors.ServiceError) {
	item, err := s.dao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("ProviderBinding", "id", id, err)
	}
	return item, nil
}

func (s *sqlProviderBindingService) Create(ctx context.Context, providerBinding *ProviderBinding) (*ProviderBinding, *errors.ServiceError) {
	item, err := s.dao.Create(ctx, providerBinding)
	if err != nil {
		return nil, errors.GeneralError("Unable to create provider binding: %s", err)
	}
	return item, nil
}

func (s *sqlProviderBindingService) Replace(ctx context.Context, providerBinding *ProviderBinding) (*ProviderBinding, *errors.ServiceError) {
	item, err := s.dao.Replace(ctx, providerBinding)
	if err != nil {
		return nil, errors.GeneralError("Unable to update provider binding: %s", err)
	}
	return item, nil
}

func (s *sqlProviderBindingService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("Unable to delete provider binding: %s", err)
	}
	return nil
}

func (s *sqlProviderBindingService) All(ctx context.Context) (ProviderBindingList, *errors.ServiceError) {
	items, err := s.dao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all provider bindings: %s", err)
	}
	return items, nil
}

func (s *sqlProviderBindingService) FindByIDs(ctx context.Context, ids []string) (ProviderBindingList, *errors.ServiceError) {
	items, err := s.dao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get provider bindings: %s", err)
	}
	return items, nil
}
