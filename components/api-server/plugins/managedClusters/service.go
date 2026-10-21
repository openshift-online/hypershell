package managedClusters

import (
	"context"
	stderrors "errors"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
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
	Register(ctx context.Context, name string, capabilities []string, oidcSubject string) (*ManagedCluster, bool, *errors.ServiceError)
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
	managedCluster, err := s.managedClusterDao.Get(ctx, id)
	if err != nil {
		return err
	}

	slog.InfoContext(ctx, "Do idempotent somethings with this managedCluster", "managed_cluster_id", managedCluster.ID)

	return nil
}

func (s *sqlManagedClusterService) OnDelete(ctx context.Context, id string) error {
	slog.InfoContext(ctx, "This managedCluster has been deleted", "managed_cluster_id", id)
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

var capabilitySegmentRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// gatewayVersionValueRE relaxes the value segment for the gateway-version
// namespace only: a gateway version tag carries dots and a release suffix (for
// example v0.1.2-rhaiv.7), which is not a DNS label. The value is a lowercase
// string of alphanumerics, dots, and hyphens that both begins and ends with an
// alphanumeric. The namespace segment stays DNS-label strict (capabilitySegmentRE).
// See managed-cluster-registration.spec.md, "Self-Declared Capability Labels".
var gatewayVersionValueRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

// gatewayVersionNamespace is the single-valued namespace through which a control
// plane advertises the gateway version it provisions onto new gateways.
const gatewayVersionNamespace = "gateway-version"

// providerNamespace and regionNamespace are the single-valued namespaces the
// inventory metric derives its provider/region buckets from (see inventory.go).
const (
	providerNamespace = "provider"
	regionNamespace   = "region"
)

// gatewayVersionValueMaxLen bounds the gateway-version value segment. The relaxed
// value regex accepts dots and hyphens but is otherwise unbounded, so a long
// label could otherwise be persisted into the JSONB capability set. A version tag
// such as v0.1.2-rhaiv.7 is far shorter; 128 is a generous ceiling.
const gatewayVersionValueMaxLen = 128

var capabilityNamespaces = map[string]bool{providerNamespace: true, "network": true, regionNamespace: true, "runtime": true, "environment": true, gatewayVersionNamespace: true}
var singleCapabilityNamespaces = map[string]bool{providerNamespace: true, regionNamespace: true, "environment": true, gatewayVersionNamespace: true}

// capabilityValueValid reports whether value is a valid value segment for the
// given namespace. Every namespace stays DNS-label strict except gateway-version,
// whose value accepts a dotted, suffixed version tag up to gatewayVersionValueMaxLen.
func capabilityValueValid(namespace, value string) bool {
	if namespace == gatewayVersionNamespace {
		return len(value) <= gatewayVersionValueMaxLen && gatewayVersionValueRE.MatchString(value)
	}
	return capabilitySegmentRE.MatchString(value)
}

// capabilityValue returns the value advertised through the given single-valued
// capability namespace, or "" when the set declares none. Inputs are assumed
// already normalized; the first matching label wins.
func capabilityValue(capabilities []string, namespace string) string {
	prefix := namespace + "/"
	for _, label := range capabilities {
		if strings.HasPrefix(label, prefix) {
			return strings.TrimPrefix(label, prefix)
		}
	}
	return ""
}

// GatewayVersionCapability returns the version advertised through the
// single-valued gateway-version capability, or "" when the set declares none.
func GatewayVersionCapability(capabilities []string) string {
	return capabilityValue(capabilities, gatewayVersionNamespace)
}

// NormalizeCapabilities validates capability labels and returns a stable,
// deduplicated representation suitable for persistence and selector matching.
func NormalizeCapabilities(labels []string, requireNonEmpty bool) ([]string, *errors.ServiceError) {
	return normalizeCapabilityLabels(labels, requireNonEmpty, true, true)
}

// NormalizeSelector applies the same vocabulary while collapsing duplicates,
// because selectors are query intent rather than the declared source-of-truth set.
func NormalizeSelector(labels []string) ([]string, *errors.ServiceError) {
	return normalizeCapabilityLabels(labels, true, false, false)
}

func normalizeCapabilityLabels(labels []string, requireNonEmpty, rejectDuplicates, enforceSingleValue bool) ([]string, *errors.ServiceError) {
	if len(labels) == 0 {
		if requireNonEmpty {
			return nil, errors.Validation("capabilities must contain at least one label")
		}
		return nil, nil
	}
	seen, namespaceCounts := map[string]bool{}, map[string]int{}
	result := make([]string, 0, len(labels))
	for _, label := range labels {
		parts := strings.Split(label, "/")
		if len(parts) != 2 || !capabilityNamespaces[parts[0]] || !capabilitySegmentRE.MatchString(parts[0]) || !capabilityValueValid(parts[0], parts[1]) {
			return nil, errors.Validation("invalid capability label %q: expected a recognized namespace and lowercase DNS-label value", label)
		}
		if seen[label] {
			if rejectDuplicates {
				return nil, errors.Validation("duplicate capability label %q", label)
			}
			continue
		}
		seen[label] = true
		namespaceCounts[parts[0]]++
		if enforceSingleValue && singleCapabilityNamespaces[parts[0]] && namespaceCounts[parts[0]] > 1 {
			return nil, errors.Validation("capability namespace %q accepts only one value", parts[0])
		}
		result = append(result, label)
	}
	sort.Strings(result)
	return result, nil
}

// equalSortedStrings reports whether two already-sorted string slices are equal.
// Both arguments are NormalizeCapabilities output, so an ordered comparison is a
// correct set comparison.
func equalSortedStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *sqlManagedClusterService) Register(ctx context.Context, name string, capabilities []string, oidcSubject string) (*ManagedCluster, bool, *errors.ServiceError) {
	// Backward compatibility: a capability-less heartbeat is accepted as a no-op
	// for the capability set. An older control plane that has not yet rolled to a
	// capability-aware image sends no capabilities every 60s; rejecting it would
	// stop last_seen_at and disconnect the cluster for the duration of a rolling
	// upgrade. We therefore allow an empty set here and preserve the stored
	// capabilities below rather than clobbering them. See
	// managed-cluster-registration.spec.md, "Capability Deployment Convergence".
	capabilities, validationErr := NormalizeCapabilities(capabilities, false)
	if validationErr != nil {
		return nil, false, validationErr
	}
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
		// A capability-less heartbeat (older control plane) is a no-op for the
		// capability set: keep the stored capabilities rather than wiping them, so
		// a mixed-version rolling upgrade does not drop placement data.
		capabilitiesChanged := false
		if len(capabilities) > 0 {
			// Both sides are NormalizeCapabilities output (sorted, deduplicated), so a
			// plain ordered comparison detects a genuine capability change.
			capabilitiesChanged = !equalSortedStrings(existing.Capabilities, capabilities)
			existing.Capabilities = append([]string(nil), capabilities...)
		}
		updated, replaceErr := s.managedClusterDao.Replace(ctx, existing)
		if replaceErr != nil {
			return nil, false, services.HandleUpdateError("ManagedCluster", replaceErr)
		}
		// A changed capability set can make a previously-unplaceable gateway
		// placeable (for example a control plane restart that publishes a new GitOps
		// capability), so emit an update event to drive gateway late-binding
		// re-resolution (data-model.spec.md, "Gateway Placement and Late Binding").
		// An unchanged or capability-less heartbeat emits nothing, so the routine 60s
		// heartbeat does not churn downstream reconcilers.
		if capabilitiesChanged {
			if _, evErr := s.events.Create(ctx, &api.Event{
				Source:    "ManagedClusters",
				SourceID:  updated.ID,
				EventType: api.UpdateEventType,
			}); evErr != nil {
				return nil, false, services.HandleUpdateError("ManagedCluster", evErr)
			}
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

	// Restore, don't recreate: a record this subject registered under this name
	// and that was since deleted comes back with its original id. Gateways keep
	// referencing that id, so the control plane resumes serving them. A new id
	// would make the control plane restart under it, see none of its gateways as
	// live, and let its namespace GC reap every gateway namespace it runs.
	deleted, delErr := s.managedClusterDao.FindDeletedBySubjectAndName(ctx, oidcSubject, name)
	if delErr != nil && !stderrors.Is(delErr, gorm.ErrRecordNotFound) {
		return nil, false, errors.GeneralError("registration lookup failed: %s", delErr)
	}
	if deleted != nil {
		// A capability-less heartbeat preserves the stored set on restore too: the
		// soft-deleted record still holds its last capabilities, so a downgraded
		// control plane re-registering within the deletion window keeps its
		// placement data rather than clearing it. See
		// managed-cluster-registration.spec.md, "Capability Deployment Convergence".
		restoreCapabilities := capabilities
		if len(restoreCapabilities) == 0 {
			restoreCapabilities = deleted.Capabilities
		}
		restored, restoreErr := s.managedClusterDao.Restore(ctx, deleted.ID, "", "", restoreCapabilities, now)
		if restoreErr != nil {
			if isUniqueViolation(restoreErr) {
				if winner, err := s.managedClusterDao.FindByName(ctx, name); err == nil && winner != nil {
					return nil, false, nameCollisionConflict(winner)
				}
			}
			return nil, false, services.HandleUpdateError("ManagedCluster", restoreErr)
		}
		if _, evErr := s.events.Create(ctx, &api.Event{
			Source:    "ManagedClusters",
			SourceID:  restored.ID,
			EventType: api.CreateEventType,
		}); evErr != nil {
			return nil, false, services.HandleUpdateError("ManagedCluster", evErr)
		}
		return restored, true, nil
	}

	cluster := &ManagedCluster{
		Name:         name,
		OIDCSubject:  oidcSubject,
		LastSeenAt:   &now,
		Capabilities: append([]string(nil), capabilities...),
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
