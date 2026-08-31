package gatewayReleases

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/logger"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

const gatewayReleasesLockType db.LockType = "gateway_releases"

type GatewayReleaseService interface {
	Get(ctx context.Context, id string) (*GatewayRelease, *errors.ServiceError)
	Create(ctx context.Context, gatewayRelease *GatewayRelease) (*GatewayRelease, *errors.ServiceError)
	Replace(ctx context.Context, gatewayRelease *GatewayRelease) (*GatewayRelease, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (GatewayReleaseList, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (GatewayReleaseList, *errors.ServiceError)

	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewGatewayReleaseService(lockFactory db.LockFactory, gatewayReleaseDao GatewayReleaseDao, events services.EventService) GatewayReleaseService {
	return &sqlGatewayReleaseService{
		lockFactory:       lockFactory,
		gatewayReleaseDao: gatewayReleaseDao,
		events:            events,
	}
}

var _ GatewayReleaseService = &sqlGatewayReleaseService{}

type sqlGatewayReleaseService struct {
	lockFactory       db.LockFactory
	gatewayReleaseDao GatewayReleaseDao
	events            services.EventService
}

func (s *sqlGatewayReleaseService) OnUpsert(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)

	gatewayRelease, err := s.gatewayReleaseDao.Get(ctx, id)
	if err != nil {
		return err
	}

	logger.Infof("Do idempotent somethings with this gatewayRelease: %s", gatewayRelease.ID)

	return nil
}

func (s *sqlGatewayReleaseService) OnDelete(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)
	logger.Infof("This gatewayRelease has been deleted: %s", id)
	return nil
}

func (s *sqlGatewayReleaseService) Get(ctx context.Context, id string) (*GatewayRelease, *errors.ServiceError) {
	gatewayRelease, err := s.gatewayReleaseDao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("GatewayRelease", "id", id, err)
	}
	return gatewayRelease, nil
}

func (s *sqlGatewayReleaseService) Create(ctx context.Context, gatewayRelease *GatewayRelease) (*GatewayRelease, *errors.ServiceError) {
	gatewayRelease.CaptureTraceContext(ctx)
	gatewayRelease, err := s.gatewayReleaseDao.Create(ctx, gatewayRelease)
	if err != nil {
		return nil, services.HandleCreateError("GatewayRelease", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "GatewayReleases",
		SourceID:  gatewayRelease.ID,
		EventType: api.CreateEventType,
	})
	if evErr != nil {
		return nil, services.HandleCreateError("GatewayRelease", evErr)
	}

	return gatewayRelease, nil
}

func (s *sqlGatewayReleaseService) Replace(ctx context.Context, gatewayRelease *GatewayRelease) (*GatewayRelease, *errors.ServiceError) {
	lockOwnerID, err := s.lockFactory.NewAdvisoryLock(ctx, gatewayRelease.ID, gatewayReleasesLockType)
	if err != nil {
		return nil, errors.DatabaseAdvisoryLock(err)
	}
	defer s.lockFactory.Unlock(ctx, lockOwnerID)

	gatewayRelease.CaptureTraceContext(ctx)
	gatewayRelease, err = s.gatewayReleaseDao.Replace(ctx, gatewayRelease)
	if err != nil {
		return nil, services.HandleUpdateError("GatewayRelease", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "GatewayReleases",
		SourceID:  gatewayRelease.ID,
		EventType: api.UpdateEventType,
	})
	if evErr != nil {
		return nil, services.HandleUpdateError("GatewayRelease", evErr)
	}

	return gatewayRelease, nil
}

func (s *sqlGatewayReleaseService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if _, svcErr := s.Get(ctx, id); svcErr != nil {
		return svcErr
	}

	if err := s.gatewayReleaseDao.Delete(ctx, id); err != nil {
		return services.HandleDeleteError("GatewayRelease", errors.GeneralError("Unable to delete gatewayRelease: %s", err))
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "GatewayReleases",
		SourceID:  id,
		EventType: api.DeleteEventType,
	})
	if evErr != nil {
		return services.HandleDeleteError("GatewayRelease", evErr)
	}

	return nil
}

func (s *sqlGatewayReleaseService) FindByIDs(ctx context.Context, ids []string) (GatewayReleaseList, *errors.ServiceError) {
	gatewayReleases, err := s.gatewayReleaseDao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all gatewayReleases: %s", err)
	}
	return gatewayReleases, nil
}

func (s *sqlGatewayReleaseService) All(ctx context.Context) (GatewayReleaseList, *errors.ServiceError) {
	gatewayReleases, err := s.gatewayReleaseDao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all gatewayReleases: %s", err)
	}
	return gatewayReleases, nil
}
