package managedDatabases

import (
	"context"
	"strings"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/logger"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

const managedDatabasesLockType db.LockType = "managed_databases"

const (
	providerCNPG       = "cnpg"
	providerDeployment = "deployment"
	providerExternal   = "external"
)

// externalSecretPrefix is the reserved prefix for admin connection Secrets and
// is a security boundary: it prevents an API-level reference from naming an
// arbitrary Secret. The same value is enforced by the control plane
// (gateway/external_db.go). See naming-multitenancy.spec.md.
const externalSecretPrefix = "hypershell-managed-db-"

type ManagedDatabaseService interface {
	Get(ctx context.Context, id string) (*ManagedDatabase, *errors.ServiceError)
	GetUnscoped(ctx context.Context, id string) (*ManagedDatabase, *errors.ServiceError)
	ListDeleted(ctx context.Context, offset, limit int) ([]ManagedDatabase, *errors.ServiceError)
	Create(ctx context.Context, managedDatabase *ManagedDatabase) (*ManagedDatabase, *errors.ServiceError)
	Replace(ctx context.Context, managedDatabase *ManagedDatabase) (*ManagedDatabase, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (ManagedDatabaseList, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (ManagedDatabaseList, *errors.ServiceError)

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

// GetUnscoped loads soft-deleted records only for delete watch enrichment.
func (s *sqlManagedDatabaseService) GetUnscoped(ctx context.Context, id string) (*ManagedDatabase, *errors.ServiceError) {
	managedDatabase, err := s.managedDatabaseDao.GetUnscoped(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("ManagedDatabase", "id", id, err)
	}
	return managedDatabase, nil
}

// ListDeleted returns durable delete tombstones for watch-stream replay. It is
// internal to the gRPC watch handshake; public list/get operations remain scoped.
func (s *sqlManagedDatabaseService) ListDeleted(ctx context.Context, offset, limit int) ([]ManagedDatabase, *errors.ServiceError) {
	managedDatabases, err := s.managedDatabaseDao.ListDeleted(ctx, offset, limit)
	if err != nil {
		return nil, errors.GeneralError("list deleted ManagedDatabases: %s", err)
	}
	return managedDatabases, nil
}

func isSupportedProvider(provider string) bool {
	return provider == providerCNPG || provider == providerDeployment || provider == providerExternal
}

func unsupportedProviderError(provider string) *errors.ServiceError {
	return errors.Validation("unsupported provider %q: supported providers are \"cnpg\", \"deployment\", and \"external\"", provider)
}

// validateExternalConnectionSecret checks the connection_secret reference
// format for external ManagedDatabases: no namespace slash, reserved prefix,
// and a non-empty value.
func validateExternalConnectionSecret(secret *string) *errors.ServiceError {
	if secret == nil || *secret == "" {
		return errors.Validation("connection_secret is required for provider \"external\"")
	}
	if strings.Contains(*secret, "/") {
		return errors.Validation("connection_secret must be a plain Secret name without a namespace prefix (no \"/\")")
	}
	if !strings.HasPrefix(*secret, externalSecretPrefix) {
		return errors.Validation("connection_secret must begin with the reserved prefix %q", externalSecretPrefix)
	}
	return nil
}

func (s *sqlManagedDatabaseService) Create(ctx context.Context, managedDatabase *ManagedDatabase) (*ManagedDatabase, *errors.ServiceError) {
	if !isSupportedProvider(managedDatabase.Provider) {
		return nil, unsupportedProviderError(managedDatabase.Provider)
	}
	if managedDatabase.Provider == providerExternal {
		if svcErr := validateExternalConnectionSecret(managedDatabase.ConnectionSecret); svcErr != nil {
			return nil, svcErr
		}
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

	persisted, err := s.managedDatabaseDao.Get(ctx, managedDatabase.ID)
	if err != nil {
		return nil, services.HandleUpdateError("ManagedDatabase", err)
	}
	if !isSupportedProvider(managedDatabase.Provider) {
		return nil, unsupportedProviderError(managedDatabase.Provider)
	}
	if isSupportedProvider(persisted.Provider) && managedDatabase.Provider != persisted.Provider {
		return nil, errors.Validation("provider cannot be changed from %q to %q", persisted.Provider, managedDatabase.Provider)
	}
	if managedDatabase.Provider == providerExternal {
		if svcErr := validateExternalConnectionSecret(managedDatabase.ConnectionSecret); svcErr != nil {
			return nil, svcErr
		}
	}

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
