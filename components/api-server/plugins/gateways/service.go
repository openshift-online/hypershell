package gateways

import (
	"context"
	"fmt"
	"net/http"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/logger"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

const gatewaysLockType db.LockType = "gateways"

// ProfileResolver supplies the gateway service with the two GatewayProfile
// facts it needs without depending on the gatewayProfiles package directly:
// the cluster-level default profile used as a create-time fallback, and an
// existence check used to reject assignments the control plane could not fetch.
type ProfileResolver interface {
	// ClusterDefaultProfileID returns the profile_id configured as the default
	// for the given cluster, or "" if the cluster has no default profile.
	ClusterDefaultProfileID(ctx context.Context, clusterID string) (string, error)
	// ProfileExists reports whether a GatewayProfile with the given id exists.
	ProfileExists(ctx context.Context, profileID string) (bool, error)
	// ClusterExists reports whether a ManagedCluster with the given id exists.
	ClusterExists(ctx context.Context, clusterID string) (bool, error)
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

	// ProfileExists reports whether the referenced GatewayProfile exists. It is
	// used by the PATCH path to reject reassigning a gateway to a profile_id the
	// control plane could not fetch. Returns true when no resolver is wired.
	ProfileExists(ctx context.Context, profileID string) (bool, *errors.ServiceError)

	// ClusterExists reports whether the referenced ManagedCluster exists. It is
	// used by the PATCH path to reject assigning a gateway to a cluster_id that
	// does not exist. Returns true when no resolver is wired.
	ClusterExists(ctx context.Context, clusterID string) (bool, *errors.ServiceError)

	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewGatewayService(
	lockFactory db.LockFactory,
	gatewayDao GatewayDao,
	events services.EventService,
	placement PlacementResolver,
	profiles ProfileResolver,
) GatewayService {
	return &sqlGatewayService{
		lockFactory: lockFactory,
		gatewayDao:  gatewayDao,
		events:      events,
		placement:   placement,
		profiles:    profiles,
	}
}

var _ GatewayService = &sqlGatewayService{}

type sqlGatewayService struct {
	lockFactory db.LockFactory
	gatewayDao  GatewayDao
	events      services.EventService
	placement   PlacementResolver
	profiles    ProfileResolver
}

func (s *sqlGatewayService) OnUpsert(ctx context.Context, id string) error {
	logger := logger.NewLogger(ctx)

	gateway, err := s.gatewayDao.Get(ctx, id)
	if err != nil {
		return err
	}

	logger.Infof("Gateway upserted: %s (name=%s namespace=%s)", gateway.ID, gateway.Name, gateway.Namespace)

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
	// A nil resolver disables cluster/profile validation and quota enforcement.
	// Production wiring always injects one (see NewServiceLocator), so this path
	// is reachable only from tests or a wiring regression; make the latter loud.
	if s.profiles == nil {
		logger.NewLogger(ctx).Warning("Gateway create proceeding with no ProfileResolver wired; skipping cluster and profile validation (production wiring always injects a resolver)")
	}

	// Validate cluster_id and profile_id before calling placement.Resolve, which
	// may provision a real ManagedDatabase. Bad requests must be rejected before
	// any server-side resources are created.

	// Validate that cluster_id references a real ManagedCluster.
	if s.profiles != nil && gateway.ClusterId != "" {
		clusterExists, clusterErr := s.profiles.ClusterExists(ctx, gateway.ClusterId)
		if clusterErr != nil {
			return nil, errors.GeneralError("failed to validate cluster_id: %s", clusterErr)
		}
		if !clusterExists {
			return nil, errors.Validation("cluster %s does not exist", gateway.ClusterId)
		}
	}

	// Resolve the gateway quota profile. A client-supplied profile_id wins; if
	// none was supplied, fall back to the cluster's default profile. A profile
	// is required, so if neither source yields one the create is rejected.
	if gateway.ProfileId == "" && s.profiles != nil {
		clusterDefault, err := s.profiles.ClusterDefaultProfileID(ctx, gateway.ClusterId)
		if err != nil {
			return nil, errors.GeneralError("failed to resolve cluster default gateway profile: %s", err)
		}
		gateway.ProfileId = clusterDefault
	}
	if gateway.ProfileId == "" {
		return nil, errors.Validation("profile_id is required: none supplied and cluster %s has no default profile", gateway.ClusterId)
	}
	// The referenced profile must exist so the control plane can fetch it when
	// building the namespace ResourceQuota; otherwise provisioning would block.
	if s.profiles != nil {
		exists, err := s.profiles.ProfileExists(ctx, gateway.ProfileId)
		if err != nil {
			return nil, errors.GeneralError("failed to validate gateway profile: %s", err)
		}
		if !exists {
			return nil, errors.Validation("gateway profile %s does not exist", gateway.ProfileId)
		}
	}

	// database_id is server-owned. Clear any value that reached the business
	// layer from an API client before selecting the configured placement strategy.
	gateway.DatabaseId = ""
	if s.placement != nil {
		if err := s.placement.Resolve(ctx, gateway); err != nil {
			if IsPlacementValidationError(err) {
				return nil, errors.Validation("gateway placement is invalid: %s", err)
			}
			return nil, errors.GeneralError("gateway placement failed: %s", err)
		}
	}
	if gateway.DatabaseId == "" {
		return nil, errors.GeneralError("gateway placement did not assign database_id")
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

func (s *sqlGatewayService) ProfileExists(ctx context.Context, profileID string) (bool, *errors.ServiceError) {
	if s.profiles == nil {
		logger.NewLogger(ctx).Warning(fmt.Sprintf("ProfileExists called with no ProfileResolver wired; skipping profile validation for %s (production wiring always injects a resolver)", profileID))
		return true, nil
	}
	exists, err := s.profiles.ProfileExists(ctx, profileID)
	if err != nil {
		return false, errors.GeneralError("failed to validate gateway profile: %s", err)
	}
	return exists, nil
}

func (s *sqlGatewayService) ClusterExists(ctx context.Context, clusterID string) (bool, *errors.ServiceError) {
	if s.profiles == nil {
		logger.NewLogger(ctx).Warning(fmt.Sprintf("ClusterExists called with no ProfileResolver wired; skipping cluster validation for %s (production wiring always injects a resolver)", clusterID))
		return true, nil
	}
	if clusterID == "" {
		return true, nil
	}
	exists, err := s.profiles.ClusterExists(ctx, clusterID)
	if err != nil {
		return false, errors.GeneralError("failed to check cluster existence: %s", err)
	}
	return exists, nil
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
