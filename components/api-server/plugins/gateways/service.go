package gateways

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/logger"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

const gatewaysLockType db.LockType = "gateways"

type ClusterFleetResolver interface {
	FleetIDForCluster(ctx context.Context, clusterID string) (fleetID string, err error)
}

type FleetDatabaseFinder interface {
	FindSoleInFleet(ctx context.Context, fleetID string) (databaseID string, err error)
	FindSole(ctx context.Context) (databaseID string, fleetID string, err error)
}

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

	FindByIDs(ctx context.Context, ids []string) (GatewayList, *errors.ServiceError)

	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewGatewayService(
	lockFactory db.LockFactory,
	gatewayDao GatewayDao,
	events services.EventService,
	dbFinder FleetDatabaseFinder,
	clusterResolver ClusterFleetResolver,
) GatewayService {
	return &sqlGatewayService{
		lockFactory:     lockFactory,
		gatewayDao:      gatewayDao,
		events:          events,
		dbFinder:        dbFinder,
		clusterResolver: clusterResolver,
	}
}

var _ GatewayService = &sqlGatewayService{}

type sqlGatewayService struct {
	lockFactory     db.LockFactory
	gatewayDao      GatewayDao
	events          services.EventService
	dbFinder        FleetDatabaseFinder
	clusterResolver ClusterFleetResolver
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
	if gateway.FleetId == "" && gateway.ClusterId != "" && s.clusterResolver != nil {
		fleetID, err := s.clusterResolver.FleetIDForCluster(ctx, gateway.ClusterId)
		if err != nil {
			return nil, errors.GeneralError("resolve fleet from cluster %s: %s", gateway.ClusterId, err)
		}
		if fleetID == "" {
			return nil, errors.Validation("cluster %s does not belong to a fleet", gateway.ClusterId)
		}
		gateway.FleetId = fleetID
	}

	if gateway.DatabaseId == "" {
		if s.dbFinder == nil {
			return nil, errors.Validation("database_id is required")
		}
		if gateway.FleetId != "" {
			dbID, findErr := s.dbFinder.FindSoleInFleet(ctx, gateway.FleetId)
			if findErr != nil {
				return nil, errors.GeneralError("resolve fleet database: %s", findErr)
			}
			if dbID == "" {
				return nil, errors.Validation("database_id is required: fleet %s has zero or multiple ManagedDatabases", gateway.FleetId)
			}
			gateway.DatabaseId = dbID
		} else {
			dbID, fleetID, findErr := s.dbFinder.FindSole(ctx)
			if findErr != nil {
				return nil, errors.GeneralError("resolve database: %s", findErr)
			}
			if dbID == "" {
				return nil, errors.Validation("database_id is required: zero or multiple ManagedDatabases exist")
			}
			gateway.DatabaseId = dbID
			gateway.FleetId = fleetID
		}
	}

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

	if err := s.gatewayDao.Delete(ctx, id); err != nil {
		return services.HandleDeleteError("Gateway", errors.GeneralError("Unable to delete gateway: %s", err))
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
