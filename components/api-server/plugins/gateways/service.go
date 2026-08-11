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

type GatewayService interface {
	Get(ctx context.Context, id string) (*Gateway, *errors.ServiceError)
	GetUnscoped(ctx context.Context, id string) (*Gateway, *errors.ServiceError)
	Create(ctx context.Context, gateway *Gateway) (*Gateway, *errors.ServiceError)
	Replace(ctx context.Context, gateway *Gateway) (*Gateway, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (GatewayList, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (GatewayList, *errors.ServiceError)

	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewGatewayService(lockFactory db.LockFactory, gatewayDao GatewayDao, events services.EventService) GatewayService {
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
