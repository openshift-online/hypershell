package ciProviders

import (
	"context"
	"strings"

	"github.com/openshift-online/hypershell/components/api-server/pkg/dberr"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

const ciProviderEventSource = "CIProviders"

var supportedForges = map[string]bool{
	"github":  true,
	"gitlab":  true,
	"forgejo": true,
}

type CIProviderService interface {
	Get(ctx context.Context, id string) (*CIProvider, *errors.ServiceError)
	Create(ctx context.Context, ciProvider *CIProvider) (*CIProvider, *errors.ServiceError)
	Replace(ctx context.Context, ciProvider *CIProvider) (*CIProvider, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (CIProviderList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (CIProviderList, *errors.ServiceError)
}

func NewCIProviderService(dao CIProviderDao, events services.EventService) CIProviderService {
	return &sqlCIProviderService{dao: dao, events: events}
}

var _ CIProviderService = &sqlCIProviderService{}

type sqlCIProviderService struct {
	dao    CIProviderDao
	events services.EventService
}

func (s *sqlCIProviderService) Get(ctx context.Context, id string) (*CIProvider, *errors.ServiceError) {
	item, err := s.dao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("CIProvider", "id", id, err)
	}
	return item, nil
}

// conflictForUnique maps a PostgreSQL unique-violation DAO error to a 409 that
// names the ACTUAL conflicting field. CIProvider has two unique indexes (name and
// issuer); the prior code reported "issuer" for every unique violation, so a
// duplicate name was reported against the wrong field (FINDING 2). The violation is
// detected by SQLSTATE 23505 through the shared dberr classifier, which covers BOTH
// driver error types - pgx (*pgconn.PgError) in production and lib/pq in the test
// session - not by substring-matching the message text (FINDING 5). The field is
// attributed by the violated index name, which both drivers expose via
// dberr.ConstraintName (with a message fallback in case a wrapper strips the
// structured field). It returns nil when err is not a unique violation, so the
// caller falls through to its generic error.
func conflictForUnique(err error, ciProvider *CIProvider) *errors.ServiceError {
	if !dberr.IsUniqueViolation(err) {
		return nil
	}
	constraint := dberr.ConstraintName(err)
	switch {
	case constraint == "idx_ci_providers_name" || strings.Contains(err.Error(), "idx_ci_providers_name"):
		return errors.Conflict("a CIProvider named %q already exists", ciProvider.Name)
	case constraint == "idx_ci_providers_issuer" || strings.Contains(err.Error(), "idx_ci_providers_issuer"):
		return errors.Conflict("a CIProvider with issuer %q already exists", ciProvider.Issuer)
	default:
		// Unique violation whose index could not be attributed: report a conflict
		// without naming a field rather than guessing the wrong one.
		return errors.Conflict("a CIProvider with the same unique field already exists")
	}
}

func (s *sqlCIProviderService) validate(ciProvider *CIProvider) *errors.ServiceError {
	if !supportedForges[ciProvider.Forge] {
		return errors.Validation("unsupported forge %q; supported forges: github, gitlab, forgejo", ciProvider.Forge)
	}
	if ciProvider.Issuer == "" {
		return errors.Validation("issuer is required")
	}
	if !strings.HasPrefix(ciProvider.Issuer, "https://") {
		return errors.Validation("issuer must be an HTTPS URL")
	}
	if ciProvider.Audience == "" {
		return errors.Validation("audience is required")
	}
	return nil
}

func (s *sqlCIProviderService) Create(ctx context.Context, ciProvider *CIProvider) (*CIProvider, *errors.ServiceError) {
	if svcErr := s.validate(ciProvider); svcErr != nil {
		return nil, svcErr
	}
	item, err := s.dao.Create(ctx, ciProvider)
	if err != nil {
		if svcErr := conflictForUnique(err, ciProvider); svcErr != nil {
			return nil, svcErr
		}
		return nil, errors.GeneralError("unable to create CIProvider: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    ciProviderEventSource,
		SourceID:  item.ID,
		EventType: api.CreateEventType,
	}); evErr != nil {
		return nil, services.HandleCreateError("CIProvider", evErr)
	}
	return item, nil
}

func (s *sqlCIProviderService) Replace(ctx context.Context, ciProvider *CIProvider) (*CIProvider, *errors.ServiceError) {
	item, err := s.dao.Replace(ctx, ciProvider)
	if err != nil {
		if svcErr := conflictForUnique(err, ciProvider); svcErr != nil {
			return nil, svcErr
		}
		return nil, errors.GeneralError("unable to update CIProvider: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    ciProviderEventSource,
		SourceID:  item.ID,
		EventType: api.UpdateEventType,
	}); evErr != nil {
		return nil, services.HandleUpdateError("CIProvider", evErr)
	}
	return item, nil
}

func (s *sqlCIProviderService) Delete(ctx context.Context, id string) *errors.ServiceError {
	refs, err := s.dao.ReferencingCITrustIDs(ctx, id)
	if err != nil {
		return errors.GeneralError("unable to check CIProvider references: %s", err)
	}
	if len(refs) > 0 {
		return errors.Conflict("CIProvider %s is still referenced by CITrust(s): %s", id, strings.Join(refs, ", "))
	}
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("unable to delete CIProvider: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    ciProviderEventSource,
		SourceID:  id,
		EventType: api.DeleteEventType,
	}); evErr != nil {
		return services.HandleDeleteError("CIProvider", evErr)
	}
	return nil
}

func (s *sqlCIProviderService) All(ctx context.Context) (CIProviderList, *errors.ServiceError) {
	items, err := s.dao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("unable to get all CIProviders: %s", err)
	}
	return items, nil
}

func (s *sqlCIProviderService) FindByIDs(ctx context.Context, ids []string) (CIProviderList, *errors.ServiceError) {
	items, err := s.dao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("unable to get CIProviders: %s", err)
	}
	return items, nil
}
