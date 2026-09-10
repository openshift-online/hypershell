package managedClusters

import (
	"context"
	stderrors "errors"
	"time"

	"gorm.io/gorm"

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
	// Register upserts a ManagedCluster by (oidcSubject, name). Returns the cluster,
	// whether it was newly created (true=201, false=200), and any error.
	Register(ctx context.Context, name, description, oidcSubject string) (*ManagedCluster, bool, *errors.ServiceError)

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
	managedCluster.CaptureTraceContext(ctx)
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

	managedCluster.CaptureTraceContext(ctx)
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

func (s *sqlManagedClusterService) Register(ctx context.Context, name, description, oidcSubject string) (*ManagedCluster, bool, *errors.ServiceError) {
	lockOwnerID, lockErr := s.lockFactory.NewAdvisoryLock(ctx, oidcSubject, managedClustersLockType)
	if lockErr != nil {
		return nil, false, errors.DatabaseAdvisoryLock(lockErr)
	}
	defer s.lockFactory.Unlock(ctx, lockOwnerID)

	existing, err := s.managedClusterDao.FindByOIDCSubject(ctx, oidcSubject)
	if err != nil && !stderrors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, errors.GeneralError("registration lookup failed: %s", err)
	}

	if existing != nil {
		if existing.Name != name {
			return nil, false, errors.Conflict("managed cluster already registered under a different name %q", existing.Name)
		}
		now := time.Now()
		existing.LastSeenAt = &now
		updated, replaceErr := s.managedClusterDao.Replace(ctx, existing)
		if replaceErr != nil {
			return nil, false, services.HandleUpdateError("ManagedCluster", replaceErr)
		}
		return updated, false, nil
	}

	now := time.Now()
	cluster := &ManagedCluster{
		Name:        name,
		OIDCSubject: oidcSubject,
		LastSeenAt:  &now,
	}
	cluster.CaptureTraceContext(ctx)
	created, createErr := s.managedClusterDao.Create(ctx, cluster)
	if createErr != nil {
		return nil, false, services.HandleCreateError("ManagedCluster", createErr)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "ManagedClusters",
		SourceID:  created.ID,
		EventType: api.CreateEventType,
	})
	if evErr != nil {
		return nil, false, services.HandleCreateError("ManagedCluster", evErr)
	}

	return created, true, nil
}
