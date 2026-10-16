package providerSpecs

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// providerSpecEventSource is the event broker Source string for ProviderSpec
// mutations. The gRPC watch handler filters the shared broker fan-out on this
// value, so it MUST match the Source written here on create/update/delete.
const providerSpecEventSource = "ProviderSpecs"

type ProviderSpecService interface {
	Get(ctx context.Context, id string) (*ProviderSpec, *errors.ServiceError)
	Create(ctx context.Context, providerSpec *ProviderSpec) (*ProviderSpec, *errors.ServiceError)
	Replace(ctx context.Context, providerSpec *ProviderSpec) (*ProviderSpec, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (ProviderSpecList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (ProviderSpecList, *errors.ServiceError)
}

func NewProviderSpecService(dao ProviderSpecDao, events services.EventService) ProviderSpecService {
	return &sqlProviderSpecService{dao: dao, events: events}
}

var _ ProviderSpecService = &sqlProviderSpecService{}

type sqlProviderSpecService struct {
	dao    ProviderSpecDao
	events services.EventService
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
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    providerSpecEventSource,
		SourceID:  item.ID,
		EventType: api.CreateEventType,
	}); evErr != nil {
		return nil, services.HandleCreateError("ProviderSpec", evErr)
	}
	return item, nil
}

func (s *sqlProviderSpecService) Replace(ctx context.Context, providerSpec *ProviderSpec) (*ProviderSpec, *errors.ServiceError) {
	item, err := s.dao.Replace(ctx, providerSpec)
	if err != nil {
		return nil, errors.GeneralError("Unable to update provider spec: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    providerSpecEventSource,
		SourceID:  item.ID,
		EventType: api.UpdateEventType,
	}); evErr != nil {
		return nil, services.HandleUpdateError("ProviderSpec", evErr)
	}
	return item, nil
}

func (s *sqlProviderSpecService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("Unable to delete provider spec: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    providerSpecEventSource,
		SourceID:  id,
		EventType: api.DeleteEventType,
	}); evErr != nil {
		return services.HandleDeleteError("ProviderSpec", evErr)
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
