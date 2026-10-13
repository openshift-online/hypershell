package agentRuntimes

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// agentRuntimeEventSource is the event broker Source string for AgentRuntime
// mutations. The gRPC watch handler filters the shared broker fan-out on this
// value, so it MUST match the Source written here on create/update/delete.
const agentRuntimeEventSource = "AgentRuntimes"

type AgentRuntimeService interface {
	Get(ctx context.Context, id string) (*AgentRuntime, *errors.ServiceError)
	// GetUnscoped returns an AgentRuntime including soft-deleted rows, for the
	// watch handler to resolve a deleted runtime's effective cluster.
	GetUnscoped(ctx context.Context, id string) (*AgentRuntime, *errors.ServiceError)
	Create(ctx context.Context, agentRuntime *AgentRuntime) (*AgentRuntime, *errors.ServiceError)
	Replace(ctx context.Context, agentRuntime *AgentRuntime) (*AgentRuntime, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (AgentRuntimeList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (AgentRuntimeList, *errors.ServiceError)
}

func NewAgentRuntimeService(dao AgentRuntimeDao, events services.EventService) AgentRuntimeService {
	return &sqlAgentRuntimeService{dao: dao, events: events}
}

var _ AgentRuntimeService = &sqlAgentRuntimeService{}

type sqlAgentRuntimeService struct {
	dao    AgentRuntimeDao
	events services.EventService
}

func (s *sqlAgentRuntimeService) Get(ctx context.Context, id string) (*AgentRuntime, *errors.ServiceError) {
	agentRuntime, err := s.dao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("AgentRuntime", "id", id, err)
	}
	return agentRuntime, nil
}

func (s *sqlAgentRuntimeService) GetUnscoped(ctx context.Context, id string) (*AgentRuntime, *errors.ServiceError) {
	agentRuntime, err := s.dao.GetUnscoped(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("AgentRuntime", "id", id, err)
	}
	return agentRuntime, nil
}

// validate enforces the repository selector contract: a non-empty selector
// requires a repository_id (422), and a set repository_id must reference an
// existing Repository.
func (s *sqlAgentRuntimeService) validate(ctx context.Context, agentRuntime *AgentRuntime) *errors.ServiceError {
	hasRepository := agentRuntime.RepositoryId != nil && *agentRuntime.RepositoryId != ""
	if len(agentRuntime.Selector) > 0 && !hasRepository {
		return errors.UnprocessableEntity("selector requires repository_id to be set")
	}
	if hasRepository {
		exists, err := s.dao.RepositoryExists(ctx, *agentRuntime.RepositoryId)
		if err != nil {
			return errors.GeneralError("Unable to validate repository_id: %s", err)
		}
		if !exists {
			return errors.Validation("repository_id %q does not reference an existing Repository", *agentRuntime.RepositoryId)
		}
	}
	return nil
}

func (s *sqlAgentRuntimeService) Create(ctx context.Context, agentRuntime *AgentRuntime) (*AgentRuntime, *errors.ServiceError) {
	if svcErr := s.validate(ctx, agentRuntime); svcErr != nil {
		return nil, svcErr
	}
	agentRuntime, err := s.dao.Create(ctx, agentRuntime)
	if err != nil {
		return nil, errors.GeneralError("Unable to create agent runtime: %s", err)
	}

	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    agentRuntimeEventSource,
		SourceID:  agentRuntime.ID,
		EventType: api.CreateEventType,
	}); evErr != nil {
		return nil, services.HandleCreateError("AgentRuntime", evErr)
	}

	return agentRuntime, nil
}

func (s *sqlAgentRuntimeService) Replace(ctx context.Context, agentRuntime *AgentRuntime) (*AgentRuntime, *errors.ServiceError) {
	if svcErr := s.validate(ctx, agentRuntime); svcErr != nil {
		return nil, svcErr
	}
	agentRuntime, err := s.dao.Replace(ctx, agentRuntime)
	if err != nil {
		return nil, errors.GeneralError("Unable to update agent runtime: %s", err)
	}

	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    agentRuntimeEventSource,
		SourceID:  agentRuntime.ID,
		EventType: api.UpdateEventType,
	}); evErr != nil {
		return nil, services.HandleUpdateError("AgentRuntime", evErr)
	}

	return agentRuntime, nil
}

func (s *sqlAgentRuntimeService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("Unable to delete agent runtime: %s", err)
	}

	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    agentRuntimeEventSource,
		SourceID:  id,
		EventType: api.DeleteEventType,
	}); evErr != nil {
		return services.HandleDeleteError("AgentRuntime", evErr)
	}

	return nil
}

func (s *sqlAgentRuntimeService) All(ctx context.Context) (AgentRuntimeList, *errors.ServiceError) {
	agentRuntimes, err := s.dao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all agent runtimes: %s", err)
	}
	return agentRuntimes, nil
}

func (s *sqlAgentRuntimeService) FindByIDs(ctx context.Context, ids []string) (AgentRuntimeList, *errors.ServiceError) {
	agentRuntimes, err := s.dao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get agent runtimes: %s", err)
	}
	return agentRuntimes, nil
}
