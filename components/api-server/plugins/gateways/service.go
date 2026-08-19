package gateways

import (
	"context"
	stderrors "errors"
	"net/http"

	"gorm.io/gorm"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/logger"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

const gatewaysLockType db.LockType = "gateways"

type GatewayService interface {
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

	// SetGatewayVersion sets the last runtime version that the health reconciler
	// observed. It does not change any other gateway field.
	SetGatewayVersion(ctx context.Context, id, version string) (string, *errors.ServiceError)

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
) GatewayService {
	return &sqlGatewayService{
		lockFactory: lockFactory,
		gatewayDao:  gatewayDao,
		events:      events,
	}
}

var _ GatewayService = &sqlGatewayService{}

type sqlGatewayService struct {
	lockFactory db.LockFactory
	gatewayDao  GatewayDao
	events      services.EventService
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

	// generation is API-server-owned: increment it iff a desired-spec field
	// changed, and never trust a client-supplied value. observed_generation is a
	// monotonic convergence latch written only by the control plane. See
	// data-model.spec.md § Gateway Generation Tracking.
	current, getErr := s.gatewayDao.Get(ctx, gateway.ID)
	if getErr != nil {
		return nil, services.HandleGetError("Gateway", "id", gateway.ID, getErr)
	}
	newGeneration := current.Generation
	if desiredStateChanged(current, gateway) {
		newGeneration = current.Generation + 1
	}
	gateway.Generation = newGeneration
	if gateway.ObservedGeneration != current.ObservedGeneration {
		if gateway.ObservedGeneration < current.ObservedGeneration || gateway.ObservedGeneration > newGeneration {
			return nil, errors.BadRequest(
				"observed_generation %d out of range [%d, %d]",
				gateway.ObservedGeneration, current.ObservedGeneration, newGeneration)
		}
	}

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

func (s *sqlGatewayService) SetGatewayVersion(ctx context.Context, id, version string) (string, *errors.ServiceError) {
	resulting, err := s.gatewayDao.SetGatewayVersion(ctx, id, version)
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return "", services.HandleGetError("Gateway", "id", id, err)
		}
		return "", services.HandleUpdateError("Gateway", err)
	}
	return resulting, nil
}

// desiredStateChanged reports whether any workload-altering (desired-spec) field
// differs between the persisted Gateway and the incoming update. Observed fields
// (status, phase, route_address, generation, observed_generation) and identity
// fields (name, namespace) are excluded: they do not alter the live
// workload and must not advance generation. See data-model.spec.md.
func desiredStateChanged(current, next *Gateway) bool {
	return current.ClusterId != next.ClusterId ||
		current.ReleaseId != next.ReleaseId ||
		current.DatabaseId != next.DatabaseId ||
		!strEq(current.ExternalDns, next.ExternalDns) ||
		!strEq(current.TlsMode, next.TlsMode) ||
		!strEq(current.ServiceType, next.ServiceType) ||
		!strEq(current.Image, next.Image) ||
		!strEq(current.SupervisorImage, next.SupervisorImage) ||
		!strEq(current.ServerDnsNames, next.ServerDnsNames) ||
		!strEq(current.Oidc, next.Oidc) ||
		!strEq(current.Route, next.Route) ||
		!strEq(current.CredentialDriver, next.CredentialDriver)
}

func strEq(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
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
