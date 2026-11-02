package inferenceRoutes

import (
	"context"
	"strings"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// conflictForDuplicateName maps a PostgreSQL unique-violation on the name index to
// a 409 that names the conflicting field (SDLC-CONF-13, FINDING 2). name is the
// only unique index on an InferenceRoute, so a unique violation is always a
// duplicate find-by-name key. It returns nil for any other error so the caller
// falls through to its generic error.
func conflictForDuplicateName(err error, name string) *errors.ServiceError {
	msg := err.Error()
	if strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique") {
		return errors.Conflict("an InferenceRoute named %q already exists", name)
	}
	return nil
}

// inferenceRouteEventSource is the event broker Source string for InferenceRoute
// mutations. The gRPC watch handler filters the shared broker fan-out on this
// value, so it MUST match the Source written here on create/delete.
const inferenceRouteEventSource = "InferenceRoutes"

type InferenceRouteService interface {
	Get(ctx context.Context, id string) (*InferenceRoute, *errors.ServiceError)
	Create(ctx context.Context, inferenceRoute *InferenceRoute) (*InferenceRoute, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (InferenceRouteList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (InferenceRouteList, *errors.ServiceError)
}

func NewInferenceRouteService(dao InferenceRouteDao, events services.EventService) InferenceRouteService {
	return &sqlInferenceRouteService{dao: dao, events: events}
}

var _ InferenceRouteService = &sqlInferenceRouteService{}

type sqlInferenceRouteService struct {
	dao    InferenceRouteDao
	events services.EventService
}

func (s *sqlInferenceRouteService) Get(ctx context.Context, id string) (*InferenceRoute, *errors.ServiceError) {
	item, err := s.dao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("InferenceRoute", "id", id, err)
	}
	return item, nil
}

func (s *sqlInferenceRouteService) Create(ctx context.Context, inferenceRoute *InferenceRoute) (*InferenceRoute, *errors.ServiceError) {
	item, err := s.dao.Create(ctx, inferenceRoute)
	if err != nil {
		if svcErr := conflictForDuplicateName(err, inferenceRoute.Name); svcErr != nil {
			return nil, svcErr
		}
		return nil, errors.GeneralError("Unable to create inference route: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    inferenceRouteEventSource,
		SourceID:  item.ID,
		EventType: api.CreateEventType,
	}); evErr != nil {
		return nil, services.HandleCreateError("InferenceRoute", evErr)
	}
	return item, nil
}

func (s *sqlInferenceRouteService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("Unable to delete inference route: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    inferenceRouteEventSource,
		SourceID:  id,
		EventType: api.DeleteEventType,
	}); evErr != nil {
		return services.HandleDeleteError("InferenceRoute", evErr)
	}
	return nil
}

func (s *sqlInferenceRouteService) All(ctx context.Context) (InferenceRouteList, *errors.ServiceError) {
	items, err := s.dao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all inference routes: %s", err)
	}
	return items, nil
}

func (s *sqlInferenceRouteService) FindByIDs(ctx context.Context, ids []string) (InferenceRouteList, *errors.ServiceError) {
	items, err := s.dao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get inference routes: %s", err)
	}
	return items, nil
}
