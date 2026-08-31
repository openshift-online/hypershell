package fleets

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/logger"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

const fleetsLockType db.LockType = "fleets"

type FleetService interface {
	Get(ctx context.Context, id string) (*Fleet, *errors.ServiceError)
	Create(ctx context.Context, fleet *Fleet) (*Fleet, *errors.ServiceError)
	Replace(ctx context.Context, fleet *Fleet) (*Fleet, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (FleetList, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (FleetList, *errors.ServiceError)

	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewFleetService(lockFactory db.LockFactory, fleetDao FleetDao, events services.EventService) FleetService {
	return &sqlFleetService{
		lockFactory: lockFactory,
		fleetDao:    fleetDao,
		events:      events,
	}
}

var _ FleetService = &sqlFleetService{}

type sqlFleetService struct {
	lockFactory db.LockFactory
	fleetDao    FleetDao
	events      services.EventService
}

func (s *sqlFleetService) OnUpsert(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)

	fleet, err := s.fleetDao.Get(ctx, id)
	if err != nil {
		return err
	}

	logger.Infof("Do idempotent somethings with this fleet: %s", fleet.ID)

	return nil
}

func (s *sqlFleetService) OnDelete(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)
	logger.Infof("This fleet has been deleted: %s", id)
	return nil
}

func (s *sqlFleetService) Get(ctx context.Context, id string) (*Fleet, *errors.ServiceError) {
	fleet, err := s.fleetDao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("Fleet", "id", id, err)
	}
	return fleet, nil
}

func (s *sqlFleetService) Create(ctx context.Context, fleet *Fleet) (*Fleet, *errors.ServiceError) {
	fleet.CaptureTraceContext(ctx)
	fleet, err := s.fleetDao.Create(ctx, fleet)
	if err != nil {
		return nil, services.HandleCreateError("Fleet", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "Fleets",
		SourceID:  fleet.ID,
		EventType: api.CreateEventType,
	})
	if evErr != nil {
		return nil, services.HandleCreateError("Fleet", evErr)
	}

	return fleet, nil
}

func (s *sqlFleetService) Replace(ctx context.Context, fleet *Fleet) (*Fleet, *errors.ServiceError) {
	lockOwnerID, err := s.lockFactory.NewAdvisoryLock(ctx, fleet.ID, fleetsLockType)
	if err != nil {
		return nil, errors.DatabaseAdvisoryLock(err)
	}
	defer s.lockFactory.Unlock(ctx, lockOwnerID)

	fleet.CaptureTraceContext(ctx)
	fleet, err = s.fleetDao.Replace(ctx, fleet)
	if err != nil {
		return nil, services.HandleUpdateError("Fleet", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "Fleets",
		SourceID:  fleet.ID,
		EventType: api.UpdateEventType,
	})
	if evErr != nil {
		return nil, services.HandleUpdateError("Fleet", evErr)
	}

	return fleet, nil
}

func (s *sqlFleetService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if _, svcErr := s.Get(ctx, id); svcErr != nil {
		return svcErr
	}

	if err := s.fleetDao.Delete(ctx, id); err != nil {
		return services.HandleDeleteError("Fleet", errors.GeneralError("Unable to delete fleet: %s", err))
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "Fleets",
		SourceID:  id,
		EventType: api.DeleteEventType,
	})
	if evErr != nil {
		return services.HandleDeleteError("Fleet", evErr)
	}

	return nil
}

func (s *sqlFleetService) FindByIDs(ctx context.Context, ids []string) (FleetList, *errors.ServiceError) {
	fleets, err := s.fleetDao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all fleets: %s", err)
	}
	return fleets, nil
}

func (s *sqlFleetService) All(ctx context.Context) (FleetList, *errors.ServiceError) {
	fleets, err := s.fleetDao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all fleets: %s", err)
	}
	return fleets, nil
}
