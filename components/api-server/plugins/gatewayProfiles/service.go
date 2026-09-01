package gatewayProfiles

import (
	"context"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/logger"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

const gatewayProfilesLockType db.LockType = "gateway_profiles"

type GatewayProfileService interface {
	Get(ctx context.Context, id string) (*GatewayProfile, *errors.ServiceError)
	Create(ctx context.Context, gatewayProfile *GatewayProfile) (*GatewayProfile, *errors.ServiceError)
	Replace(ctx context.Context, gatewayProfile *GatewayProfile) (*GatewayProfile, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (GatewayProfileList, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (GatewayProfileList, *errors.ServiceError)

	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewGatewayProfileService(lockFactory db.LockFactory, gatewayProfileDao GatewayProfileDao, events services.EventService) GatewayProfileService {
	return &sqlGatewayProfileService{
		lockFactory:       lockFactory,
		gatewayProfileDao: gatewayProfileDao,
		events:            events,
	}
}

var _ GatewayProfileService = &sqlGatewayProfileService{}

type sqlGatewayProfileService struct {
	lockFactory       db.LockFactory
	gatewayProfileDao GatewayProfileDao
	events            services.EventService
}

func (s *sqlGatewayProfileService) OnUpsert(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)

	gatewayProfile, err := s.gatewayProfileDao.Get(ctx, id)
	if err != nil {
		return err
	}

	logger.Infof("GatewayProfile upserted: %s (name=%s)", gatewayProfile.ID, gatewayProfile.Name)

	return nil
}

func (s *sqlGatewayProfileService) OnDelete(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)
	logger.Infof("This gatewayProfile has been deleted: %s", id)
	return nil
}

func (s *sqlGatewayProfileService) Get(ctx context.Context, id string) (*GatewayProfile, *errors.ServiceError) {
	gatewayProfile, err := s.gatewayProfileDao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("GatewayProfile", "id", id, err)
	}
	return gatewayProfile, nil
}

func (s *sqlGatewayProfileService) Create(ctx context.Context, gatewayProfile *GatewayProfile) (*GatewayProfile, *errors.ServiceError) {
	if svcErr := validateProfileFields(gatewayProfile); svcErr != nil {
		return nil, svcErr
	}

	gatewayProfile, err := s.gatewayProfileDao.Create(ctx, gatewayProfile)
	if err != nil {
		return nil, services.HandleCreateError("GatewayProfile", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "GatewayProfiles",
		SourceID:  gatewayProfile.ID,
		EventType: api.CreateEventType,
	})
	if evErr != nil {
		return nil, services.HandleCreateError("GatewayProfile", evErr)
	}

	return gatewayProfile, nil
}

func (s *sqlGatewayProfileService) Replace(ctx context.Context, gatewayProfile *GatewayProfile) (*GatewayProfile, *errors.ServiceError) {
	if svcErr := validateProfileFields(gatewayProfile); svcErr != nil {
		return nil, svcErr
	}

	lockOwnerID, err := s.lockFactory.NewAdvisoryLock(ctx, gatewayProfile.ID, gatewayProfilesLockType)
	if err != nil {
		return nil, errors.DatabaseAdvisoryLock(err)
	}
	defer s.lockFactory.Unlock(ctx, lockOwnerID)

	gatewayProfile, err = s.gatewayProfileDao.Replace(ctx, gatewayProfile)
	if err != nil {
		return nil, services.HandleUpdateError("GatewayProfile", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "GatewayProfiles",
		SourceID:  gatewayProfile.ID,
		EventType: api.UpdateEventType,
	})
	if evErr != nil {
		return nil, services.HandleUpdateError("GatewayProfile", evErr)
	}

	return gatewayProfile, nil
}

func (s *sqlGatewayProfileService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if _, svcErr := s.Get(ctx, id); svcErr != nil {
		return svcErr
	}

	// Deletion protection: a profile referenced by a cluster default or by any
	// gateway must not be deleted, or those referrers would carry a dangling
	// profile_id that blocks gateway provisioning.
	clusterRef, refErr := s.gatewayProfileDao.ExistsByClusterProfileID(ctx, id)
	if refErr != nil {
		return errors.GeneralError("check cluster references: %s", refErr)
	}
	if clusterRef {
		return errors.Conflict("GatewayProfile %s is the default profile for one or more clusters and cannot be deleted", id)
	}

	gatewayRef, refErr := s.gatewayProfileDao.ExistsByGatewayProfileID(ctx, id)
	if refErr != nil {
		return errors.GeneralError("check gateway references: %s", refErr)
	}
	if gatewayRef {
		return errors.Conflict("GatewayProfile %s is referenced by one or more gateways and cannot be deleted", id)
	}

	if err := s.gatewayProfileDao.Delete(ctx, id); err != nil {
		return services.HandleDeleteError("GatewayProfile", errors.GeneralError("Unable to delete gatewayProfile: %s", err))
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "GatewayProfiles",
		SourceID:  id,
		EventType: api.DeleteEventType,
	})
	if evErr != nil {
		return services.HandleDeleteError("GatewayProfile", evErr)
	}

	return nil
}

func (s *sqlGatewayProfileService) FindByIDs(ctx context.Context, ids []string) (GatewayProfileList, *errors.ServiceError) {
	gatewayProfiles, err := s.gatewayProfileDao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all gatewayProfiles: %s", err)
	}
	return gatewayProfiles, nil
}

func (s *sqlGatewayProfileService) All(ctx context.Context) (GatewayProfileList, *errors.ServiceError) {
	gatewayProfiles, err := s.gatewayProfileDao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all gatewayProfiles: %s", err)
	}
	return gatewayProfiles, nil
}
