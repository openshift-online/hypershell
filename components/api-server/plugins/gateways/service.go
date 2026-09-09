package gateways

import (
	"context"
	stderrors "errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"gorm.io/gorm"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/logger"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

const gatewaysLockType db.LockType = "gateways"

type GatewayService interface {
	DeletionStatus(ctx context.Context, reference string) (*Gateway, *errors.ServiceError)
	PendingDeletions(ctx context.Context, after, clusterID string, limit int) (GatewayList, *errors.ServiceError)
	CompleteDeletion(ctx context.Context, id string) *errors.ServiceError
	FindByExternalReference(ctx context.Context, reference string) (*Gateway, *errors.ServiceError)
	Get(ctx context.Context, id string) (*Gateway, *errors.ServiceError)
	GetUnscoped(ctx context.Context, id string) (*Gateway, *errors.ServiceError)
	Create(ctx context.Context, gateway *Gateway) (*Gateway, *errors.ServiceError)
	Replace(ctx context.Context, gateway *Gateway) (*Gateway, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (GatewayList, *errors.ServiceError)

	// AdjustActiveSandboxCount applies a relative delta to the
	// active_sandbox_count of the gateway backing the given namespace and returns
	// the resulting count. It is a no-op returning 0 when no live gateway backs
	// the namespace.
	AdjustActiveSandboxCount(ctx context.Context, namespace string, delta int) (int, *errors.ServiceError)

	// SetActiveSandboxCount sets the active_sandbox_count of the gateway backing
	// the given namespace to an absolute value and returns it (self-heal path).
	SetActiveSandboxCount(ctx context.Context, namespace string, count int) (int, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (GatewayList, *errors.ServiceError)

	// CountByPhase returns the number of gateways in each phase.
	CountByPhase(ctx context.Context) (map[string]int64, *errors.ServiceError)

	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewGatewayService(
	lockFactory db.LockFactory,
	gatewayDao GatewayDao,
	events services.EventService,
	placement PlacementResolver,
) GatewayService {
	return &sqlGatewayService{
		lockFactory: lockFactory,
		gatewayDao:  gatewayDao,
		events:      events,
		placement:   placement,
	}
}

var _ GatewayService = &sqlGatewayService{}

type sqlGatewayService struct {
	lockFactory db.LockFactory
	gatewayDao  GatewayDao
	events      services.EventService
	placement   PlacementResolver
}

func (s *sqlGatewayService) OnUpsert(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)

	gateway, err := s.gatewayDao.Get(ctx, id)
	if err != nil {
		return err
	}

	logger.Infof("Do idempotent somethings with this gateway: %s", gateway.ID)

	return nil
}

func (s *sqlGatewayService) OnDelete(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)
	logger.Infof("This gateway has been deleted: %s", id)
	return nil
}

func (s *sqlGatewayService) Get(ctx context.Context, id string) (*Gateway, *errors.ServiceError) {
	gateway, err := s.gatewayDao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("Gateway", "id", id, err)
	}
	return gateway, nil
}

func (s *sqlGatewayService) GetUnscoped(ctx context.Context, id string) (*Gateway, *errors.ServiceError) {
	gateway, err := s.gatewayDao.GetUnscoped(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("Gateway", "id", id, err)
	}
	return gateway, nil
}

func (s *sqlGatewayService) Create(ctx context.Context, gateway *Gateway) (*Gateway, *errors.ServiceError) {
	gateway.ExternalReferenceOwner = nil
	if gateway.ExternalReference != nil {
		ctx = context.WithValue(ctx, atomicCreationKey{}, true)
		if err := validateExternalReference(*gateway.ExternalReference); err != nil {
			return nil, err
		}
		owner := rbac.GetUserIDFromContext(ctx)
		if owner == "" {
			return nil, errors.Forbidden("external_reference requires an authenticated caller")
		}
		gateway.ExternalReferenceOwner = &owner
		if err := s.gatewayDao.LockExternalReference(ctx, owner, *gateway.ExternalReference); err != nil {
			return nil, errors.GeneralError("cannot lock gateway reference: %s", err)
		}
		existing, err := s.gatewayDao.FindByExternalReference(ctx, owner, *gateway.ExternalReference)
		if err == nil {
			if existing.DeletedAt.Valid {
				return nil, errors.Conflict("external_reference belongs to a deleted gateway; use a new reference")
			}
			copy := *existing
			copy.replayed = true
			return &copy, nil
		}
		if !stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.GeneralError("cannot find gateway reference: %s", err)
		}
	}
	// database_id is server-owned. Clear any value that reached the business
	// layer from an API client before selecting the configured placement strategy.
	gateway.DatabaseId = ""
	if s.placement != nil {
		if err := s.placement.Resolve(ctx, gateway); err != nil {
			if IsPlacementValidationError(err) {
				return nil, errors.Validation("gateway placement is invalid: %s", err)
			}
			return nil, errors.GeneralError("gateway placement failed: %s", err)
		}
	}
	if gateway.DatabaseId == "" {
		return nil, errors.GeneralError("gateway placement did not assign database_id")
	}

	gateway.CaptureTraceContext(ctx)
	gateway, err := s.gatewayDao.Create(ctx, gateway)
	if err != nil {
		return nil, services.HandleCreateError("Gateway", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "Gateways",
		SourceID:  gateway.ID,
		EventType: api.CreateEventType,
	})
	if evErr != nil {
		db.MarkForRollback(ctx, evErr)
		return nil, services.HandleCreateError("Gateway", evErr)
	}

	return gateway, nil
}

func (s *sqlGatewayService) Replace(ctx context.Context, gateway *Gateway) (*Gateway, *errors.ServiceError) {
	lockOwnerID, err := s.lockFactory.NewAdvisoryLock(ctx, gateway.ID, gatewaysLockType)
	if err != nil {
		return nil, errors.DatabaseAdvisoryLock(err)
	}
	defer s.lockFactory.Unlock(ctx, lockOwnerID)

	gateway.CaptureTraceContext(ctx)
	gateway, err = s.gatewayDao.Replace(ctx, gateway)
	if err != nil {
		return nil, services.HandleUpdateError("Gateway", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "Gateways",
		SourceID:  gateway.ID,
		EventType: api.UpdateEventType,
	})
	if evErr != nil {
		return nil, services.HandleUpdateError("Gateway", evErr)
	}

	return gateway, nil
}

func (s *sqlGatewayService) AdjustActiveSandboxCount(ctx context.Context, namespace string, delta int) (int, *errors.ServiceError) {
	// The DAO emits the Gateway update Event in the same transaction as the count
	// mutation (transactional outbox), so there is no separate event write here to
	// drift from the persisted value.
	count, err := s.gatewayDao.AdjustActiveSandboxCount(ctx, namespace, delta)
	if err != nil {
		return 0, services.HandleUpdateError("Gateway", err)
	}
	return count, nil
}

func (s *sqlGatewayService) SetActiveSandboxCount(ctx context.Context, namespace string, count int) (int, *errors.ServiceError) {
	resulting, err := s.gatewayDao.SetActiveSandboxCount(ctx, namespace, count)
	if err != nil {
		return 0, services.HandleUpdateError("Gateway", err)
	}
	return resulting, nil
}

func (s *sqlGatewayService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if _, svcErr := s.Get(ctx, id); svcErr != nil {
		return svcErr
	}

	// Delete the gateway row through the cleanup barrier so the row disappears
	// while the service-account lifecycle lock is still held. finalize captures
	// its own error so a row-deletion failure is reported distinctly from a
	// cleanup-unavailable failure.
	var deleteErr error
	finalize := func(ctx context.Context) error {
		deleteErr = s.gatewayDao.Delete(ctx, id)
		return deleteErr
	}
	if err := cleanBeforeDeletion(ctx, id, finalize); err != nil {
		if deleteErr != nil {
			return services.HandleDeleteError("Gateway", errors.GeneralError("Unable to delete gateway: %s", deleteErr))
		}
		serviceErr := errors.GeneralError("gateway service-account cleanup is unavailable")
		serviceErr.HttpCode = http.StatusServiceUnavailable
		return serviceErr
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "Gateways",
		SourceID:  id,
		EventType: api.DeleteEventType,
	})
	if evErr != nil {
		return services.HandleDeleteError("Gateway", evErr)
	}

	return nil
}

func (s *sqlGatewayService) FindByIDs(ctx context.Context, ids []string) (GatewayList, *errors.ServiceError) {
	gateways, err := s.gatewayDao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all gateways: %s", err)
	}
	return gateways, nil
}

func (s *sqlGatewayService) All(ctx context.Context) (GatewayList, *errors.ServiceError) {
	gateways, err := s.gatewayDao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all gateways: %s", err)
	}
	return gateways, nil
}

func (s *sqlGatewayService) CountByPhase(ctx context.Context) (map[string]int64, *errors.ServiceError) {
	counts, err := s.gatewayDao.CountByPhase(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to count gateways by phase: %s", err)
	}
	return counts, nil
}

func validateExternalReference(reference string) *errors.ServiceError {
	if len(reference) == 0 || len(reference) > 255 || strings.TrimSpace(reference) != reference || strings.IndexFunc(reference, unicode.IsControl) >= 0 {
		return errors.Validation("external_reference must contain 1 to 255 bytes without control characters or surrounding whitespace")
	}
	return nil
}
func (s *sqlGatewayService) FindByExternalReference(ctx context.Context, reference string) (*Gateway, *errors.ServiceError) {
	if err := validateExternalReference(reference); err != nil {
		return nil, err
	}
	owner := rbac.GetUserIDFromContext(ctx)
	if owner == "" {
		return nil, errors.Forbidden("external_reference requires an authenticated caller")
	}
	gateway, err := s.gatewayDao.FindByExternalReference(ctx, owner, reference)
	if err != nil {
		return nil, services.HandleGetError("Gateway", "external_reference", reference, err)
	}
	if gateway.DeletedAt.Valid {
		return nil, errors.NotFound("Gateway with this external_reference does not exist")
	}
	return gateway, nil
}

// DeletionStatus uses the immutable creator scope even after gateway access is removed.
func (s *sqlGatewayService) DeletionStatus(ctx context.Context, reference string) (*Gateway, *errors.ServiceError) {
	if err := validateExternalReference(reference); err != nil {
		return nil, err
	}
	owner := rbac.GetUserIDFromContext(ctx)
	if owner == "" {
		return nil, errors.Forbidden("deletion status requires an authenticated caller")
	}
	gateway, err := s.gatewayDao.FindByExternalReference(ctx, owner, reference)
	if err != nil {
		return nil, services.HandleGetError("Gateway", "external_reference", reference, err)
	}
	return gateway, nil
}
func (s *sqlGatewayService) PendingDeletions(ctx context.Context, after, clusterID string, limit int) (GatewayList, *errors.ServiceError) {
	gateways, err := s.gatewayDao.PendingDeletions(ctx, after, clusterID, limit)
	if err != nil {
		return nil, errors.GeneralError("cannot load pending gateway deletions: %s", err)
	}
	return gateways, nil
}
func (s *sqlGatewayService) CompleteDeletion(ctx context.Context, id string) *errors.ServiceError {
	gateway, err := s.GetUnscoped(ctx, id)
	if err != nil {
		return err
	}
	if !gateway.DeletedAt.Valid {
		return errors.Conflict("gateway deletion has not been requested")
	}
	if err := s.gatewayDao.CompleteDeletion(ctx, id); err != nil {
		return errors.GeneralError("cannot complete gateway deletion: %s", err)
	}
	return nil
}
