package secretSources

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// secretSourceEventSource is the event broker Source string for SecretSource
// mutations. The gRPC watch handler filters the shared broker fan-out on this
// value, so it MUST match the Source written here on create/update/delete.
const secretSourceEventSource = "SecretSources"

type SecretSourceService interface {
	Get(ctx context.Context, id string) (*SecretSource, *errors.ServiceError)
	Create(ctx context.Context, secretSource *SecretSource) (*SecretSource, *errors.ServiceError)
	Replace(ctx context.Context, secretSource *SecretSource) (*SecretSource, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (SecretSourceList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (SecretSourceList, *errors.ServiceError)
	// FindByAgentRuntimeID returns the SecretSources owned by an AgentRuntime, used
	// to embed them in the AgentRuntime gRPC watch/list payload.
	FindByAgentRuntimeID(ctx context.Context, agentRuntimeID string) (SecretSourceList, *errors.ServiceError)
}

func NewSecretSourceService(dao SecretSourceDao, events services.EventService) SecretSourceService {
	return &sqlSecretSourceService{dao: dao, events: events}
}

var _ SecretSourceService = &sqlSecretSourceService{}

type sqlSecretSourceService struct {
	dao    SecretSourceDao
	events services.EventService
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
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    secretSourceEventSource,
		SourceID:  item.ID,
		EventType: api.CreateEventType,
	}); evErr != nil {
		return nil, services.HandleCreateError("SecretSource", evErr)
	}
	return item, nil
}

func (s *sqlSecretSourceService) Replace(ctx context.Context, secretSource *SecretSource) (*SecretSource, *errors.ServiceError) {
	item, err := s.dao.Replace(ctx, secretSource)
	if err != nil {
		return nil, errors.GeneralError("Unable to update secret source: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    secretSourceEventSource,
		SourceID:  item.ID,
		EventType: api.UpdateEventType,
	}); evErr != nil {
		return nil, services.HandleUpdateError("SecretSource", evErr)
	}
	return item, nil
}

func (s *sqlSecretSourceService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("Unable to delete secret source: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    secretSourceEventSource,
		SourceID:  id,
		EventType: api.DeleteEventType,
	}); evErr != nil {
		return services.HandleDeleteError("SecretSource", evErr)
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

func (s *sqlSecretSourceService) FindByAgentRuntimeID(ctx context.Context, agentRuntimeID string) (SecretSourceList, *errors.ServiceError) {
	items, err := s.dao.FindByAgentRuntimeID(ctx, agentRuntimeID)
	if err != nil {
		return nil, errors.GeneralError("Unable to get secret sources for agent runtime %s: %s", agentRuntimeID, err)
	}
	return items, nil
}
