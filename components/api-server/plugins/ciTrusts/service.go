package ciTrusts

import (
	"context"

	"github.com/openshift-online/hypershell/components/api-server/pkg/dberr"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// conflictForDuplicateName maps a PostgreSQL unique-violation on the name index to
// a 409 that names the conflicting field (SDLC-CONF-13, FINDING 2). name is the
// only unique index on a CITrust, so a unique violation is always a duplicate
// find-by-name key. The violation is detected by SQLSTATE 23505 through the shared
// dberr classifier, which covers BOTH driver error types - pgx (*pgconn.PgError) in
// production and lib/pq in the test session - not by substring-matching the message
// text, which is fragile against locale and wording changes (FINDING 5). It returns
// nil for any other error so the caller falls through to its generic error.
func conflictForDuplicateName(err error, name string) *errors.ServiceError {
	if dberr.IsUniqueViolation(err) {
		return errors.Conflict("a CITrust named %q already exists", name)
	}
	return nil
}

const ciTrustEventSource = "CITrusts"

type CITrustService interface {
	Get(ctx context.Context, id string) (*CITrust, *errors.ServiceError)
	Create(ctx context.Context, ciTrust *CITrust) (*CITrust, *errors.ServiceError)
	Replace(ctx context.Context, ciTrust *CITrust) (*CITrust, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (CITrustList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (CITrustList, *errors.ServiceError)
	// GatewayRepositoryID returns the ownership mark of the referenced gateway (nil
	// when the gateway is unowned, found=false when no live gateway has that id). It
	// backs the imported CITrust tenancy gate (SEC-RBAC-25).
	GatewayRepositoryID(ctx context.Context, gatewayID string) (repositoryID *string, found bool, svcErr *errors.ServiceError)
}

func NewCITrustService(dao CITrustDao, events services.EventService) CITrustService {
	return &sqlCITrustService{dao: dao, events: events}
}

var _ CITrustService = &sqlCITrustService{}

type sqlCITrustService struct {
	dao    CITrustDao
	events services.EventService
}

func (s *sqlCITrustService) Get(ctx context.Context, id string) (*CITrust, *errors.ServiceError) {
	item, err := s.dao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("CITrust", "id", id, err)
	}
	return item, nil
}

func (s *sqlCITrustService) validate(ctx context.Context, ciTrust *CITrust) *errors.ServiceError {
	if ciTrust.CIProviderID == "" {
		return errors.Validation("ci_provider_id is required")
	}
	if ciTrust.GatewayID == "" {
		return errors.Validation("gateway_id is required")
	}
	if ciTrust.Role == "" {
		return errors.Validation("role is required")
	}
	exists, err := s.dao.CIProviderExists(ctx, ciTrust.CIProviderID)
	if err != nil {
		return errors.GeneralError("unable to validate ci_provider_id: %s", err)
	}
	if !exists {
		return errors.Validation("ci_provider_id %q does not reference an existing CIProvider", ciTrust.CIProviderID)
	}
	exists, err = s.dao.GatewayExists(ctx, ciTrust.GatewayID)
	if err != nil {
		return errors.GeneralError("unable to validate gateway_id: %s", err)
	}
	if !exists {
		return errors.Validation("gateway_id %q does not reference an existing Gateway", ciTrust.GatewayID)
	}
	return nil
}

func (s *sqlCITrustService) Create(ctx context.Context, ciTrust *CITrust) (*CITrust, *errors.ServiceError) {
	if svcErr := s.validate(ctx, ciTrust); svcErr != nil {
		return nil, svcErr
	}
	item, err := s.dao.Create(ctx, ciTrust)
	if err != nil {
		if svcErr := conflictForDuplicateName(err, ciTrust.Name); svcErr != nil {
			return nil, svcErr
		}
		return nil, errors.GeneralError("unable to create CITrust: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    ciTrustEventSource,
		SourceID:  item.ID,
		EventType: api.CreateEventType,
	}); evErr != nil {
		return nil, services.HandleCreateError("CITrust", evErr)
	}
	return item, nil
}

func (s *sqlCITrustService) Replace(ctx context.Context, ciTrust *CITrust) (*CITrust, *errors.ServiceError) {
	item, err := s.dao.Replace(ctx, ciTrust)
	if err != nil {
		if svcErr := conflictForDuplicateName(err, ciTrust.Name); svcErr != nil {
			return nil, svcErr
		}
		return nil, errors.GeneralError("unable to update CITrust: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    ciTrustEventSource,
		SourceID:  item.ID,
		EventType: api.UpdateEventType,
	}); evErr != nil {
		return nil, services.HandleUpdateError("CITrust", evErr)
	}
	return item, nil
}

func (s *sqlCITrustService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("unable to delete CITrust: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    ciTrustEventSource,
		SourceID:  id,
		EventType: api.DeleteEventType,
	}); evErr != nil {
		return services.HandleDeleteError("CITrust", evErr)
	}
	return nil
}

func (s *sqlCITrustService) All(ctx context.Context) (CITrustList, *errors.ServiceError) {
	items, err := s.dao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("unable to get all CITrusts: %s", err)
	}
	return items, nil
}

func (s *sqlCITrustService) FindByIDs(ctx context.Context, ids []string) (CITrustList, *errors.ServiceError) {
	items, err := s.dao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("unable to get CITrusts: %s", err)
	}
	return items, nil
}

func (s *sqlCITrustService) GatewayRepositoryID(ctx context.Context, gatewayID string) (*string, bool, *errors.ServiceError) {
	repositoryID, found, err := s.dao.GatewayRepositoryID(ctx, gatewayID)
	if err != nil {
		return nil, false, errors.GeneralError("unable to resolve gateway ownership: %s", err)
	}
	return repositoryID, found, nil
}
