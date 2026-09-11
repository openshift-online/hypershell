package managedDatabases

import (
	"context"
	"regexp"
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

// externalCredentialsNamespacePrefix is the reserved prefix for the namespace
// holding an external server's admin credentials. ManagedDatabase.connection_secret
// names that NAMESPACE, not a Secret: the credentials are provisioned out-of-band,
// normally before HyperShell is installed, so they must not depend on the control
// plane instance namespace existing.
//
// The prefix is a security boundary, not a convention. Combined with the fixed
// Secret name below it bounds what the control plane can be made to read to a
// single deliberately-named Secret inside deliberately-created namespaces,
// preventing an API-level reference from pointing the reconciler at an unrelated
// Secret such as hypershell-db-app. The same values are enforced by the control
// plane (gateway/external_db.go). See naming-multitenancy.spec.md §6.2.
const externalCredentialsNamespacePrefix = "hypershell-managed-db-"

// externalCredentialsSecretName is the fixed name of the Secret read inside a
// hypershell-managed-db-<name> namespace. It is not configurable.
const externalCredentialsSecretName = "hypershell-managed-db-credentials"

// dns1123LabelMaxLength is the Kubernetes limit for a namespace name. Namespace
// names are DNS-1123 labels, not subdomains, so dots are not permitted.
const dns1123LabelMaxLength = 63

// dns1123LabelPattern matches a valid DNS-1123 label (a valid namespace name).
var dns1123LabelPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

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

// validateExternalConnectionSecret checks the connection_secret reference for
// external ManagedDatabases. The value names the NAMESPACE holding the admin
// credentials Secret, so it must be a bare namespace name (no "/"), carry the
// reserved prefix, and be a valid DNS-1123 label. The Secret inside it always
// has the fixed name externalCredentialsSecretName.
func validateExternalConnectionSecret(secret *string) *errors.ServiceError {
	if secret == nil || *secret == "" {
		return errors.Validation("connection_secret is required for provider \"external\": it names the namespace holding the %q Secret", externalCredentialsSecretName)
	}
	value := *secret
	if strings.Contains(value, "/") {
		return errors.Validation("connection_secret must be a bare namespace name without a \"/\": it names the namespace holding the %q Secret, not the Secret itself", externalCredentialsSecretName)
	}
	if !strings.HasPrefix(value, externalCredentialsNamespacePrefix) {
		return errors.Validation("connection_secret namespace %q must begin with the reserved prefix %q", value, externalCredentialsNamespacePrefix)
	}
	if len(value) > dns1123LabelMaxLength {
		return errors.Validation("connection_secret namespace %q is %d characters; a namespace name may be at most %d", value, len(value), dns1123LabelMaxLength)
	}
	if !dns1123LabelPattern.MatchString(value) {
		return errors.Validation("connection_secret namespace %q is not a valid DNS-1123 label: use lowercase alphanumerics and '-', starting and ending with an alphanumeric", value)
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

	managedDatabase.CaptureTraceContext(ctx)
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

	managedDatabase.CaptureTraceContext(ctx)
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
