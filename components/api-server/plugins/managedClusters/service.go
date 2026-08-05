package managedClusters

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/logger"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

const managedClustersLockType db.LockType = "managed_clusters"

type ManagedClusterService interface {
	Get(ctx context.Context, id string) (*ManagedCluster, *errors.ServiceError)
	Create(ctx context.Context, managedCluster *ManagedCluster) (*ManagedCluster, *errors.ServiceError)
	Replace(ctx context.Context, managedCluster *ManagedCluster) (*ManagedCluster, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (ManagedClusterList, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (ManagedClusterList, *errors.ServiceError)

	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewManagedClusterService(lockFactory db.LockFactory, managedClusterDao ManagedClusterDao, events services.EventService) ManagedClusterService {
	return &sqlManagedClusterService{
		lockFactory:       lockFactory,
		managedClusterDao: managedClusterDao,
		events:            events,
	}
}

var _ ManagedClusterService = &sqlManagedClusterService{}

type sqlManagedClusterService struct {
	lockFactory       db.LockFactory
	managedClusterDao ManagedClusterDao
	events            services.EventService
}

func (s *sqlManagedClusterService) OnUpsert(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)

	managedCluster, err := s.managedClusterDao.Get(ctx, id)
	if err != nil {
		return err
	}

	logger.Infof("Do idempotent somethings with this managedCluster: %s", managedCluster.ID)

	return nil
}

func (s *sqlManagedClusterService) OnDelete(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)
	logger.Infof("This managedCluster has been deleted: %s", id)
	return nil
}

func (s *sqlManagedClusterService) Get(ctx context.Context, id string) (*ManagedCluster, *errors.ServiceError) {
	managedCluster, err := s.managedClusterDao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("ManagedCluster", "id", id, err)
	}
	return managedCluster, nil
}

func (s *sqlManagedClusterService) Create(ctx context.Context, managedCluster *ManagedCluster) (*ManagedCluster, *errors.ServiceError) {
	managedCluster, err := s.managedClusterDao.Create(ctx, managedCluster)
	if err != nil {
		return nil, services.HandleCreateError("ManagedCluster", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "ManagedClusters",
		SourceID:  managedCluster.ID,
		EventType: api.CreateEventType,
	})
	if evErr != nil {
		return nil, services.HandleCreateError("ManagedCluster", evErr)
	}

	return managedCluster, nil
}

func (s *sqlManagedClusterService) Replace(ctx context.Context, managedCluster *ManagedCluster) (*ManagedCluster, *errors.ServiceError) {
	lockOwnerID, err := s.lockFactory.NewAdvisoryLock(ctx, managedCluster.ID, managedClustersLockType)
	if err != nil {
		return nil, errors.DatabaseAdvisoryLock(err)
	}
	defer s.lockFactory.Unlock(ctx, lockOwnerID)

	managedCluster, err = s.managedClusterDao.Replace(ctx, managedCluster)
	if err != nil {
		return nil, services.HandleUpdateError("ManagedCluster", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "ManagedClusters",
		SourceID:  managedCluster.ID,
		EventType: api.UpdateEventType,
	})
	if evErr != nil {
		return nil, services.HandleUpdateError("ManagedCluster", evErr)
	}

	return managedCluster, nil
}

func (s *sqlManagedClusterService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if _, svcErr := s.Get(ctx, id); svcErr != nil {
		return svcErr
	}

	if err := s.managedClusterDao.Delete(ctx, id); err != nil {
		return services.HandleDeleteError("ManagedCluster", errors.GeneralError("Unable to delete managedCluster: %s", err))
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "ManagedClusters",
		SourceID:  id,
		EventType: api.DeleteEventType,
	})
	if evErr != nil {
		return services.HandleDeleteError("ManagedCluster", evErr)
	}

	return nil
}

func (s *sqlManagedClusterService) FindByIDs(ctx context.Context, ids []string) (ManagedClusterList, *errors.ServiceError) {
	managedClusters, err := s.managedClusterDao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all managedClusters: %s", err)
	}
	return managedClusters, nil
}

func (s *sqlManagedClusterService) All(ctx context.Context) (ManagedClusterList, *errors.ServiceError) {
	managedClusters, err := s.managedClusterDao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all managedClusters: %s", err)
	}
	return managedClusters, nil
}
