package managedClusters

import (
	"context"
	stderrors "errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
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
	// whether it was newly created (true=201, false=200), and any error. A name
	// held by a record with a different or empty oidc_subject, or a subject
	// already registered under another name, is a 409 Conflict: registration
	// never creates a duplicate name and never adopts an existing record.
	Register(ctx context.Context, name, description, oidcSubject string) (*ManagedCluster, bool, *errors.ServiceError)
	// FindRegisteredBySubject returns the ManagedCluster a control plane
	// registered under the given OIDC subject, or (nil, nil) when the subject
	// has not registered. An empty subject never matches.
	FindRegisteredBySubject(ctx context.Context, oidcSubject string) (*ManagedCluster, *errors.ServiceError)
	// GetRegistered returns the ManagedCluster with the given id only when a
	// control plane registered it (non-empty oidc_subject). It returns (nil, nil)
	// when no such record exists or the record was created manually.
	GetRegistered(ctx context.Context, id string) (*ManagedCluster, *errors.ServiceError)

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
			return nil, false, errors.Conflict("managed cluster already registered under a different name %q (record %s); a control plane may not change its registered name without an operator deleting that record", existing.Name, existing.ID)
		}
		now := time.Now()
		existing.LastSeenAt = &now
		updated, replaceErr := s.managedClusterDao.Replace(ctx, existing)
		if replaceErr != nil {
			return nil, false, services.HandleUpdateError("ManagedCluster", replaceErr)
		}
		return updated, false, nil
	}

	// Name Collision Is a Conflict: a record with this name that this subject
	// does not own (another control plane's, or a manual POST /managed_clusters
	// placeholder with an empty oidc_subject) is never adopted, because gateways
	// may already reference its id. The unique index on name backs this check
	// against a concurrent registration by a different subject.
	holder, nameErr := s.managedClusterDao.FindByName(ctx, name)
	if nameErr != nil && !stderrors.Is(nameErr, gorm.ErrRecordNotFound) {
		return nil, false, errors.GeneralError("registration lookup failed: %s", nameErr)
	}
	if holder != nil {
		return nil, false, nameCollisionConflict(holder)
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
		// A concurrent registration by another subject can win the name between
		// the lookup above and this insert; the unique index turns that race
		// into a conflict, reported with the winner's id when it can be read.
		if isUniqueViolation(createErr) {
			if winner, err := s.managedClusterDao.FindByName(ctx, name); err == nil && winner != nil {
				return nil, false, nameCollisionConflict(winner)
			}
		}
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

func (s *sqlManagedClusterService) FindRegisteredBySubject(ctx context.Context, oidcSubject string) (*ManagedCluster, *errors.ServiceError) {
	if oidcSubject == "" {
		return nil, nil
	}
	cluster, err := s.managedClusterDao.FindByOIDCSubject(ctx, oidcSubject)
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.GeneralError("managed cluster lookup by subject failed: %s", err)
	}
	return cluster, nil
}

func (s *sqlManagedClusterService) GetRegistered(ctx context.Context, id string) (*ManagedCluster, *errors.ServiceError) {
	if id == "" {
		return nil, nil
	}
	cluster, err := s.managedClusterDao.Get(ctx, id)
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, errors.GeneralError("managed cluster lookup failed: %s", err)
	}
	if cluster.OIDCSubject == "" {
		return nil, nil
	}
	return cluster, nil
}

// nameCollisionConflict is the 409 for a registration whose name is held by a
// record this OIDC subject does not own. The message names the record and the
// operator action required, per managed-cluster-registration.spec.md.
func nameCollisionConflict(holder *ManagedCluster) *errors.ServiceError {
	owner := "was created manually (empty oidc_subject)"
	if holder.OIDCSubject != "" {
		owner = "is registered by another control plane"
	}
	return errors.Conflict(
		"managed cluster name %q is held by record %s, which %s; an operator must delete that record (and re-point any gateways that reference it) before this control plane can register under that name",
		holder.Name, holder.ID, owner,
	)
}

// isUniqueViolation reports whether err is PostgreSQL SQLSTATE 23505
// (unique_violation). The GORM session uses pgx in production and lib/pq in the
// test session, so both driver error types are checked; the message text is
// never inspected.
func isUniqueViolation(err error) bool {
	const uniqueViolation = "23505"
	var pgErr *pgconn.PgError
	if stderrors.As(err, &pgErr) {
		return pgErr.Code == uniqueViolation
	}
	var pqErr *pq.Error
	if stderrors.As(err, &pqErr) {
		return string(pqErr.Code) == uniqueViolation
	}
	return false
}
