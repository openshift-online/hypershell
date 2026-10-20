package providerBindings

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// providerBindingEventSource is the event broker Source string for
// ProviderBinding mutations. The gRPC watch handler filters the shared broker
// fan-out on this value, so it MUST match the Source written here on
// create/update/delete.
const providerBindingEventSource = "ProviderBindings"

type ProviderBindingService interface {
	Get(ctx context.Context, id string) (*ProviderBinding, *errors.ServiceError)
	Create(ctx context.Context, providerBinding *ProviderBinding) (*ProviderBinding, *errors.ServiceError)
	Replace(ctx context.Context, providerBinding *ProviderBinding) (*ProviderBinding, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (ProviderBindingList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (ProviderBindingList, *errors.ServiceError)
}

func NewProviderBindingService(dao ProviderBindingDao, events services.EventService) ProviderBindingService {
	return &sqlProviderBindingService{dao: dao, events: events}
}

var _ ProviderBindingService = &sqlProviderBindingService{}

type sqlProviderBindingService struct {
	dao    ProviderBindingDao
	events services.EventService
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
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    providerBindingEventSource,
		SourceID:  item.ID,
		EventType: api.CreateEventType,
	}); evErr != nil {
		return nil, services.HandleCreateError("ProviderBinding", evErr)
	}
	return item, nil
}

func (s *sqlProviderBindingService) Replace(ctx context.Context, providerBinding *ProviderBinding) (*ProviderBinding, *errors.ServiceError) {
	item, err := s.dao.Replace(ctx, providerBinding)
	if err != nil {
		return nil, errors.GeneralError("Unable to update provider binding: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    providerBindingEventSource,
		SourceID:  item.ID,
		EventType: api.UpdateEventType,
	}); evErr != nil {
		return nil, services.HandleUpdateError("ProviderBinding", evErr)
	}
	return item, nil
}

func (s *sqlProviderBindingService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("Unable to delete provider binding: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    providerBindingEventSource,
		SourceID:  id,
		EventType: api.DeleteEventType,
	}); evErr != nil {
		return services.HandleDeleteError("ProviderBinding", evErr)
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
