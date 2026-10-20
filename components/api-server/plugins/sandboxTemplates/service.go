package sandboxTemplates

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// sandboxTemplateEventSource is the event broker Source string for
// SandboxTemplate mutations. The gRPC watch handler filters the shared broker
// fan-out on this value, so it MUST match the Source written here on
// create/update/delete.
const sandboxTemplateEventSource = "SandboxTemplates"

type SandboxTemplateService interface {
	Get(ctx context.Context, id string) (*SandboxTemplate, *errors.ServiceError)
	Create(ctx context.Context, sandboxTemplate *SandboxTemplate) (*SandboxTemplate, *errors.ServiceError)
	Replace(ctx context.Context, sandboxTemplate *SandboxTemplate) (*SandboxTemplate, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (SandboxTemplateList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (SandboxTemplateList, *errors.ServiceError)
}

func NewSandboxTemplateService(dao SandboxTemplateDao, events services.EventService) SandboxTemplateService {
	return &sqlSandboxTemplateService{dao: dao, events: events}
}

var _ SandboxTemplateService = &sqlSandboxTemplateService{}

type sqlSandboxTemplateService struct {
	dao    SandboxTemplateDao
	events services.EventService
}

func (s *sqlSandboxTemplateService) Get(ctx context.Context, id string) (*SandboxTemplate, *errors.ServiceError) {
	item, err := s.dao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("SandboxTemplate", "id", id, err)
	}
	return item, nil
}

func (s *sqlSandboxTemplateService) Create(ctx context.Context, sandboxTemplate *SandboxTemplate) (*SandboxTemplate, *errors.ServiceError) {
	item, err := s.dao.Create(ctx, sandboxTemplate)
	if err != nil {
		return nil, errors.GeneralError("Unable to create sandbox template: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    sandboxTemplateEventSource,
		SourceID:  item.ID,
		EventType: api.CreateEventType,
	}); evErr != nil {
		return nil, services.HandleCreateError("SandboxTemplate", evErr)
	}
	return item, nil
}

func (s *sqlSandboxTemplateService) Replace(ctx context.Context, sandboxTemplate *SandboxTemplate) (*SandboxTemplate, *errors.ServiceError) {
	item, err := s.dao.Replace(ctx, sandboxTemplate)
	if err != nil {
		return nil, errors.GeneralError("Unable to update sandbox template: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    sandboxTemplateEventSource,
		SourceID:  item.ID,
		EventType: api.UpdateEventType,
	}); evErr != nil {
		return nil, services.HandleUpdateError("SandboxTemplate", evErr)
	}
	return item, nil
}

func (s *sqlSandboxTemplateService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("Unable to delete sandbox template: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    sandboxTemplateEventSource,
		SourceID:  id,
		EventType: api.DeleteEventType,
	}); evErr != nil {
		return services.HandleDeleteError("SandboxTemplate", evErr)
	}
	return nil
}

func (s *sqlSandboxTemplateService) All(ctx context.Context) (SandboxTemplateList, *errors.ServiceError) {
	items, err := s.dao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all sandbox templates: %s", err)
	}
	return items, nil
}

func (s *sqlSandboxTemplateService) FindByIDs(ctx context.Context, ids []string) (SandboxTemplateList, *errors.ServiceError) {
	items, err := s.dao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get sandbox templates: %s", err)
	}
	return items, nil
}
