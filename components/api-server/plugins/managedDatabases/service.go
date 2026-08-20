package managedDatabases

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/logger"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

const managedDatabasesLockType db.LockType = "managed_databases"

type ManagedDatabaseService interface {
	Get(ctx context.Context, id string) (*ManagedDatabase, *errors.ServiceError)
	Create(ctx context.Context, managedDatabase *ManagedDatabase) (*ManagedDatabase, *errors.ServiceError)
	Replace(ctx context.Context, managedDatabase *ManagedDatabase) (*ManagedDatabase, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (ManagedDatabaseList, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (ManagedDatabaseList, *errors.ServiceError)
	FindSoleInFleet(ctx context.Context, fleetID string) (*ManagedDatabase, *errors.ServiceError)

	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewManagedDatabaseService(lockFactory db.LockFactory, managedDatabaseDao ManagedDatabaseDao, events services.EventService) ManagedDatabaseService {
	return &sqlManagedDatabaseService{
		lockFactory:        lockFactory,
		managedDatabaseDao: managedDatabaseDao,
		events:             events,
	}
}

var _ ManagedDatabaseService = &sqlManagedDatabaseService{}

type sqlManagedDatabaseService struct {
	lockFactory        db.LockFactory
	managedDatabaseDao ManagedDatabaseDao
	events             services.EventService
}

func (s *sqlManagedDatabaseService) OnUpsert(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)

	managedDatabase, err := s.managedDatabaseDao.Get(ctx, id)
	if err != nil {
		return err
	}

	logger.Infof("Do idempotent somethings with this managedDatabase: %s", managedDatabase.ID)

	return nil
}

func (s *sqlManagedDatabaseService) OnDelete(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)
	logger.Infof("This managedDatabase has been deleted: %s", id)
	return nil
}

func (s *sqlManagedDatabaseService) Get(ctx context.Context, id string) (*ManagedDatabase, *errors.ServiceError) {
	managedDatabase, err := s.managedDatabaseDao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("ManagedDatabase", "id", id, err)
	}
	return managedDatabase, nil
}

func (s *sqlManagedDatabaseService) Create(ctx context.Context, managedDatabase *ManagedDatabase) (*ManagedDatabase, *errors.ServiceError) {
	if managedDatabase.Provider != "cnpg" {
		return nil, errors.Validation("unsupported provider %q: only \"cnpg\" is supported", managedDatabase.Provider)
	}

	managedDatabase, err := s.managedDatabaseDao.Create(ctx, managedDatabase)
	if err != nil {
		return nil, services.HandleCreateError("ManagedDatabase", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "ManagedDatabases",
		SourceID:  managedDatabase.ID,
		EventType: api.CreateEventType,
	})
	if evErr != nil {
		return nil, services.HandleCreateError("ManagedDatabase", evErr)
	}

	return managedDatabase, nil
}

func (s *sqlManagedDatabaseService) Replace(ctx context.Context, managedDatabase *ManagedDatabase) (*ManagedDatabase, *errors.ServiceError) {
	lockOwnerID, err := s.lockFactory.NewAdvisoryLock(ctx, managedDatabase.ID, managedDatabasesLockType)
	if err != nil {
		return nil, errors.DatabaseAdvisoryLock(err)
	}
	defer s.lockFactory.Unlock(ctx, lockOwnerID)

	managedDatabase, err = s.managedDatabaseDao.Replace(ctx, managedDatabase)
	if err != nil {
		return nil, services.HandleUpdateError("ManagedDatabase", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "ManagedDatabases",
		SourceID:  managedDatabase.ID,
		EventType: api.UpdateEventType,
	})
	if evErr != nil {
		return nil, services.HandleUpdateError("ManagedDatabase", evErr)
	}

	return managedDatabase, nil
}

func (s *sqlManagedDatabaseService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if _, svcErr := s.Get(ctx, id); svcErr != nil {
		return svcErr
	}

	referenced, refErr := s.managedDatabaseDao.ExistsByDatabaseID(ctx, id)
	if refErr != nil {
		return errors.GeneralError("check gateway references: %s", refErr)
	}
	if referenced {
		return errors.Conflict("ManagedDatabase %s is referenced by one or more gateways and cannot be deleted", id)
	}

	if err := s.managedDatabaseDao.Delete(ctx, id); err != nil {
		return services.HandleDeleteError("ManagedDatabase", errors.GeneralError("Unable to delete managedDatabase: %s", err))
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "ManagedDatabases",
		SourceID:  id,
		EventType: api.DeleteEventType,
	})
	if evErr != nil {
		return services.HandleDeleteError("ManagedDatabase", evErr)
	}

	return nil
}

func (s *sqlManagedDatabaseService) FindByIDs(ctx context.Context, ids []string) (ManagedDatabaseList, *errors.ServiceError) {
	managedDatabases, err := s.managedDatabaseDao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all managedDatabases: %s", err)
	}
	return managedDatabases, nil
}

func (s *sqlManagedDatabaseService) All(ctx context.Context) (ManagedDatabaseList, *errors.ServiceError) {
	managedDatabases, err := s.managedDatabaseDao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all managedDatabases: %s", err)
	}
	return managedDatabases, nil
}

func (s *sqlManagedDatabaseService) FindSoleInFleet(ctx context.Context, fleetID string) (*ManagedDatabase, *errors.ServiceError) {
	managedDatabase, err := s.managedDatabaseDao.FindSoleInFleet(ctx, fleetID)
	if err != nil {
		return nil, errors.GeneralError("Unable to find sole managed database in fleet: %s", err)
	}
	return managedDatabase, nil
}
