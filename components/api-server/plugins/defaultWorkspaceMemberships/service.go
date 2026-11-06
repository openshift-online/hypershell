package defaultWorkspaceMemberships

import (
	"context"
	"regexp"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// subjectPattern mirrors WorkspaceMembership.subject's validation exactly
// (PLAT-DATA-27.02; plugins/workspaceMemberships/service.go, subjectPattern):
// a non-empty token of letters, digits, and `. _ : @ -`, at most 255 characters.
var subjectPattern = regexp.MustCompile(`^[A-Za-z0-9._:@-]{1,255}$`)

// eventSource is the event broker Source string for DefaultWorkspaceMembership
// mutations. The gRPC watch handler filters the shared broker fan-out on this
// value, so it MUST match the Source written here.
const eventSource = "DefaultWorkspaceMemberships"

// MembershipService is the internal-only facade the gatewayAccess (GTW-ACC-16/17) and
// serviceAccounts (OSG-SACC-19) plugins call directly (same process, no gRPC;
// PLAT-DATA-27, "No public CRUD"). It is also used by the gateways plugin's
// deletion cleanup to implement the PLAT-DATA-27.05 cascade.
type MembershipService interface {
	Get(ctx context.Context, id string) (*DefaultWorkspaceMembership, *errors.ServiceError)
	// GetUnscoped reads a membership including a soft-deleted row, so a delete
	// watch event can still carry the row's gateway_id and subject (the row
	// cannot be re-read with Get once it is deleted).
	GetUnscoped(ctx context.Context, id string) (*DefaultWorkspaceMembership, *errors.ServiceError)
	// Create is idempotent by (gateway_id, subject) (PLAT-DATA-27.01): a
	// duplicate create returns the existing live row instead of a conflict, and
	// writes an audit event only for the row it actually creates.
	Create(ctx context.Context, gatewayID, subject, source string) (*DefaultWorkspaceMembership, *errors.ServiceError)
	// DeleteByGatewayAndSubject is idempotent: deleting an absent membership
	// succeeds as a no-op (OSG-SACC-19.05).
	DeleteByGatewayAndSubject(ctx context.Context, gatewayID, subject string) *errors.ServiceError
	FindByGatewayAndSubject(ctx context.Context, gatewayID, subject string) (*DefaultWorkspaceMembership, *errors.ServiceError)
	// CountLiveForGateway backs the GTW-ACC-17/OSG-SACC-19 1,000-member limit
	// check. It must be called before the new row's creating operation performs
	// any irreversible external side effect (a Keycloak provisioning call).
	CountLiveForGateway(ctx context.Context, gatewayID string) (int64, *errors.ServiceError)
	AllForGateway(ctx context.Context, gatewayID string) (DefaultWorkspaceMembershipList, *errors.ServiceError)
	// DeleteAllForGateway implements the PLAT-DATA-27.05 gateway-deletion
	// cascade. It makes no OpenShell-side cleanup call: the gateway's OpenShell
	// instance and its default workspace are removed with the gateway itself.
	DeleteAllForGateway(ctx context.Context, gatewayID string) (int64, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (DefaultWorkspaceMembershipList, *errors.ServiceError)
	All(ctx context.Context) (DefaultWorkspaceMembershipList, *errors.ServiceError)
	CountByStatus(ctx context.Context) (map[string]int64, *errors.ServiceError)
	// UpdateStatus is the control-plane's reconciliation status write-back
	// (CTRL-CORE-16), reached over DefaultWorkspaceMembershipService's
	// UpdateDefaultWorkspaceMembershipStatus RPC.
	UpdateStatus(ctx context.Context, id, status string, lastReconcileError *string) (*DefaultWorkspaceMembership, *errors.ServiceError)
}

func NewService(dao Dao, events services.EventService) MembershipService {
	return &service{dao: dao, events: events}
}

var _ MembershipService = &service{}

type service struct {
	dao    Dao
	events services.EventService
}

func (s *service) Get(ctx context.Context, id string) (*DefaultWorkspaceMembership, *errors.ServiceError) {
	item, err := s.dao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("DefaultWorkspaceMembership", "id", id, err)
	}
	return item, nil
}

func (s *service) GetUnscoped(ctx context.Context, id string) (*DefaultWorkspaceMembership, *errors.ServiceError) {
	item, err := s.dao.GetUnscoped(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("DefaultWorkspaceMembership", "id", id, err)
	}
	return item, nil
}

func (s *service) Create(ctx context.Context, gatewayID, subject, source string) (*DefaultWorkspaceMembership, *errors.ServiceError) {
	if gatewayID == "" {
		return nil, errors.Validation("gateway_id is required")
	}
	if !subjectPattern.MatchString(subject) {
		return nil, errors.Validation("default workspace membership subject must be a non-empty token (letters, digits, and . _ : @ -), at most 255 characters")
	}
	switch source {
	case SourceGatewayAccess, SourceServiceAccount:
	default:
		return nil, errors.Validation("default workspace membership source must be %q or %q", SourceGatewayAccess, SourceServiceAccount)
	}

	// Idempotent by (gateway_id, subject) (PLAT-DATA-27.01): a re-grant of the
	// same tier, or a retried request, must not create a second live row nor a
	// second audit event.
	existing, err := s.dao.FindByGatewayAndSubject(ctx, gatewayID, subject)
	if err != nil {
		return nil, errors.GeneralError("Unable to look up default workspace membership: %s", err)
	}
	if existing != nil {
		return existing, nil
	}

	item := &DefaultWorkspaceMembership{
		GatewayID:            gatewayID,
		Subject:              subject,
		Role:                 RoleUser,
		Source:               source,
		ReconciliationStatus: StatusPending,
	}
	item, err = s.dao.Create(ctx, item)
	if err != nil {
		return nil, errors.GeneralError("Unable to create default workspace membership: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    eventSource,
		SourceID:  item.ID,
		EventType: api.CreateEventType,
	}); evErr != nil {
		return nil, services.HandleCreateError("DefaultWorkspaceMembership", evErr)
	}
	if auditErr := s.dao.CreateAudit(ctx, &AuditEvent{
		MembershipID: item.ID,
		GatewayID:    gatewayID,
		Subject:      subject,
		Role:         RoleUser,
		Source:       source,
		Action:       "create",
	}); auditErr != nil {
		return nil, errors.GeneralError("Unable to audit default workspace membership create: %s", auditErr)
	}
	return item, nil
}

func (s *service) DeleteByGatewayAndSubject(ctx context.Context, gatewayID, subject string) *errors.ServiceError {
	existing, err := s.dao.FindByGatewayAndSubject(ctx, gatewayID, subject)
	if err != nil {
		return errors.GeneralError("Unable to look up default workspace membership: %s", err)
	}
	if existing == nil {
		// Idempotent no-op (OSG-SACC-19.05): nothing to delete.
		return nil
	}
	if err := s.dao.Delete(ctx, existing.ID); err != nil {
		return errors.GeneralError("Unable to delete default workspace membership: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    eventSource,
		SourceID:  existing.ID,
		EventType: api.DeleteEventType,
	}); evErr != nil {
		return services.HandleDeleteError("DefaultWorkspaceMembership", evErr)
	}
	if auditErr := s.dao.CreateAudit(ctx, &AuditEvent{
		MembershipID: existing.ID,
		GatewayID:    existing.GatewayID,
		Subject:      existing.Subject,
		Role:         existing.Role,
		Source:       existing.Source,
		Action:       "delete",
	}); auditErr != nil {
		return errors.GeneralError("Unable to audit default workspace membership delete: %s", auditErr)
	}
	return nil
}

func (s *service) FindByGatewayAndSubject(ctx context.Context, gatewayID, subject string) (*DefaultWorkspaceMembership, *errors.ServiceError) {
	item, err := s.dao.FindByGatewayAndSubject(ctx, gatewayID, subject)
	if err != nil {
		return nil, errors.GeneralError("Unable to look up default workspace membership: %s", err)
	}
	return item, nil
}

func (s *service) CountLiveForGateway(ctx context.Context, gatewayID string) (int64, *errors.ServiceError) {
	count, err := s.dao.CountLiveForGateway(ctx, gatewayID)
	if err != nil {
		return 0, errors.GeneralError("Unable to count default workspace memberships: %s", err)
	}
	return count, nil
}

func (s *service) AllForGateway(ctx context.Context, gatewayID string) (DefaultWorkspaceMembershipList, *errors.ServiceError) {
	items, err := s.dao.AllForGateway(ctx, gatewayID)
	if err != nil {
		return nil, errors.GeneralError("Unable to list default workspace memberships: %s", err)
	}
	return items, nil
}

func (s *service) DeleteAllForGateway(ctx context.Context, gatewayID string) (int64, *errors.ServiceError) {
	items, err := s.dao.AllForGateway(ctx, gatewayID)
	if err != nil {
		return 0, errors.GeneralError("Unable to list default workspace memberships: %s", err)
	}
	count, err := s.dao.DeleteAllForGateway(ctx, gatewayID)
	if err != nil {
		return 0, errors.GeneralError("Unable to delete default workspace memberships: %s", err)
	}
	for _, item := range items {
		if _, evErr := s.events.Create(ctx, &api.Event{
			Source:    eventSource,
			SourceID:  item.ID,
			EventType: api.DeleteEventType,
		}); evErr != nil {
			return count, services.HandleDeleteError("DefaultWorkspaceMembership", evErr)
		}
		if auditErr := s.dao.CreateAudit(ctx, &AuditEvent{
			MembershipID: item.ID,
			GatewayID:    item.GatewayID,
			Subject:      item.Subject,
			Role:         item.Role,
			Source:       item.Source,
			Action:       "delete_gateway_cascade",
		}); auditErr != nil {
			return count, errors.GeneralError("Unable to audit default workspace membership cascade delete: %s", auditErr)
		}
	}
	return count, nil
}

func (s *service) FindByIDs(ctx context.Context, ids []string) (DefaultWorkspaceMembershipList, *errors.ServiceError) {
	items, err := s.dao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get default workspace memberships: %s", err)
	}
	return items, nil
}

func (s *service) All(ctx context.Context) (DefaultWorkspaceMembershipList, *errors.ServiceError) {
	items, err := s.dao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all default workspace memberships: %s", err)
	}
	return items, nil
}

func (s *service) CountByStatus(ctx context.Context) (map[string]int64, *errors.ServiceError) {
	counts, err := s.dao.CountByStatus(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to count default workspace memberships by status: %s", err)
	}
	return counts, nil
}

func (s *service) UpdateStatus(ctx context.Context, id, status string, lastReconcileError *string) (*DefaultWorkspaceMembership, *errors.ServiceError) {
	switch status {
	case StatusPending, StatusReady, StatusFailed:
	default:
		return nil, errors.Validation("reconciliation_status must be one of %q, %q, %q", StatusPending, StatusReady, StatusFailed)
	}
	item, err := s.dao.UpdateStatus(ctx, id, status, lastReconcileError)
	if err != nil {
		return nil, services.HandleUpdateError("DefaultWorkspaceMembership", err)
	}
	return item, nil
}
