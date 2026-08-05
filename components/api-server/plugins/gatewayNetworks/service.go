package gatewayNetworks

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/logger"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

const gatewayNetworksLockType db.LockType = "gateway_networks"

type GatewayNetworkService interface {
	Get(ctx context.Context, id string) (*GatewayNetwork, *errors.ServiceError)
	Create(ctx context.Context, gatewayNetwork *GatewayNetwork) (*GatewayNetwork, *errors.ServiceError)
	Replace(ctx context.Context, gatewayNetwork *GatewayNetwork) (*GatewayNetwork, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (GatewayNetworkList, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (GatewayNetworkList, *errors.ServiceError)

	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewGatewayNetworkService(lockFactory db.LockFactory, gatewayNetworkDao GatewayNetworkDao, events services.EventService) GatewayNetworkService {
	return &sqlGatewayNetworkService{
		lockFactory:       lockFactory,
		gatewayNetworkDao: gatewayNetworkDao,
		events:            events,
	}
}

var _ GatewayNetworkService = &sqlGatewayNetworkService{}

type sqlGatewayNetworkService struct {
	lockFactory       db.LockFactory
	gatewayNetworkDao GatewayNetworkDao
	events            services.EventService
}

func (s *sqlGatewayNetworkService) OnUpsert(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)

	gatewayNetwork, err := s.gatewayNetworkDao.Get(ctx, id)
	if err != nil {
		return err
	}

	logger.Infof("Do idempotent somethings with this gatewayNetwork: %s", gatewayNetwork.ID)

	return nil
}

func (s *sqlGatewayNetworkService) OnDelete(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)
	logger.Infof("This gatewayNetwork has been deleted: %s", id)
	return nil
}

func (s *sqlGatewayNetworkService) Get(ctx context.Context, id string) (*GatewayNetwork, *errors.ServiceError) {
	gatewayNetwork, err := s.gatewayNetworkDao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("GatewayNetwork", "id", id, err)
	}
	return gatewayNetwork, nil
}

func (s *sqlGatewayNetworkService) Create(ctx context.Context, gatewayNetwork *GatewayNetwork) (*GatewayNetwork, *errors.ServiceError) {
	gatewayNetwork, err := s.gatewayNetworkDao.Create(ctx, gatewayNetwork)
	if err != nil {
		return nil, services.HandleCreateError("GatewayNetwork", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "GatewayNetworks",
		SourceID:  gatewayNetwork.ID,
		EventType: api.CreateEventType,
	})
	if evErr != nil {
		return nil, services.HandleCreateError("GatewayNetwork", evErr)
	}

	return gatewayNetwork, nil
}

func (s *sqlGatewayNetworkService) Replace(ctx context.Context, gatewayNetwork *GatewayNetwork) (*GatewayNetwork, *errors.ServiceError) {
	lockOwnerID, err := s.lockFactory.NewAdvisoryLock(ctx, gatewayNetwork.ID, gatewayNetworksLockType)
	if err != nil {
		return nil, errors.DatabaseAdvisoryLock(err)
	}
	defer s.lockFactory.Unlock(ctx, lockOwnerID)

	gatewayNetwork, err = s.gatewayNetworkDao.Replace(ctx, gatewayNetwork)
	if err != nil {
		return nil, services.HandleUpdateError("GatewayNetwork", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "GatewayNetworks",
		SourceID:  gatewayNetwork.ID,
		EventType: api.UpdateEventType,
	})
	if evErr != nil {
		return nil, services.HandleUpdateError("GatewayNetwork", evErr)
	}

	return gatewayNetwork, nil
}

func (s *sqlGatewayNetworkService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if _, svcErr := s.Get(ctx, id); svcErr != nil {
		return svcErr
	}

	if err := s.gatewayNetworkDao.Delete(ctx, id); err != nil {
		return services.HandleDeleteError("GatewayNetwork", errors.GeneralError("Unable to delete gatewayNetwork: %s", err))
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "GatewayNetworks",
		SourceID:  id,
		EventType: api.DeleteEventType,
	})
	if evErr != nil {
		return services.HandleDeleteError("GatewayNetwork", evErr)
	}

	return nil
}

func (s *sqlGatewayNetworkService) FindByIDs(ctx context.Context, ids []string) (GatewayNetworkList, *errors.ServiceError) {
	gatewayNetworks, err := s.gatewayNetworkDao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all gatewayNetworks: %s", err)
	}
	return gatewayNetworks, nil
}

func (s *sqlGatewayNetworkService) All(ctx context.Context) (GatewayNetworkList, *errors.ServiceError) {
	gatewayNetworks, err := s.gatewayNetworkDao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all gatewayNetworks: %s", err)
	}
	return gatewayNetworks, nil
}
