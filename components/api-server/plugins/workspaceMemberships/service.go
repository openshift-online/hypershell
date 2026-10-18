package workspaceMemberships

import (
	"context"
	"regexp"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

// subjectPattern bounds a membership subject to a log-safe token (the OIDC subject the
// agent uses to connect to the gateway, typically a UUID). It is format-validated at the
// API boundary (security.spec.md "Validate All User Input") so a malformed, unbounded,
// or control-character value never reaches the gateway membership.
var subjectPattern = regexp.MustCompile(`^[A-Za-z0-9._:@-]{1,255}$`)

// workspaceMembershipEventSource is the event broker Source string for
// WorkspaceMembership mutations. The gRPC watch handler filters the shared broker
// fan-out on this value, so it MUST match the Source written here.
const workspaceMembershipEventSource = "WorkspaceMemberships"

type WorkspaceMembershipService interface {
	Get(ctx context.Context, id string) (*WorkspaceMembership, *errors.ServiceError)
	Create(ctx context.Context, workspaceMembership *WorkspaceMembership) (*WorkspaceMembership, *errors.ServiceError)
	Replace(ctx context.Context, workspaceMembership *WorkspaceMembership) (*WorkspaceMembership, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (WorkspaceMembershipList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (WorkspaceMembershipList, *errors.ServiceError)
}

func NewWorkspaceMembershipService(dao WorkspaceMembershipDao, events services.EventService) WorkspaceMembershipService {
	return &sqlWorkspaceMembershipService{dao: dao, events: events}
}

var _ WorkspaceMembershipService = &sqlWorkspaceMembershipService{}

type sqlWorkspaceMembershipService struct {
	dao    WorkspaceMembershipDao
	events services.EventService
}

func (s *sqlWorkspaceMembershipService) Get(ctx context.Context, id string) (*WorkspaceMembership, *errors.ServiceError) {
	item, err := s.dao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("WorkspaceMembership", "id", id, err)
	}
	return item, nil
}

func (s *sqlWorkspaceMembershipService) Create(ctx context.Context, workspaceMembership *WorkspaceMembership) (*WorkspaceMembership, *errors.ServiceError) {
	if svcErr := validateSubject(workspaceMembership.Subject); svcErr != nil {
		return nil, svcErr
	}
	if svcErr := validateRole(workspaceMembership.Role); svcErr != nil {
		return nil, svcErr
	}
	// An admin create always satisfies the invariant. For a member create, the
	// workspace must still retain an admin when it hosts a refresh-bearing
	// ProviderBinding (data-model.spec.md "Admin Role Required for Credential
	// Refresh"). excludeID is empty: this membership does not exist yet.
	if workspaceMembership.Role != RoleAdmin {
		if svcErr := s.requireAdminRetained(ctx, workspaceMembership.WorkspaceId, ""); svcErr != nil {
			return nil, svcErr
		}
	}
	item, err := s.dao.Create(ctx, workspaceMembership)
	if err != nil {
		return nil, errors.GeneralError("Unable to create workspace membership: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    workspaceMembershipEventSource,
		SourceID:  item.ID,
		EventType: api.CreateEventType,
	}); evErr != nil {
		return nil, services.HandleCreateError("WorkspaceMembership", evErr)
	}
	return item, nil
}

func (s *sqlWorkspaceMembershipService) Replace(ctx context.Context, workspaceMembership *WorkspaceMembership) (*WorkspaceMembership, *errors.ServiceError) {
	if svcErr := validateRole(workspaceMembership.Role); svcErr != nil {
		return nil, svcErr
	}
	// Demoting this membership out of admin must not strip the last admin from a
	// workspace that hosts a refresh-bearing ProviderBinding.
	if workspaceMembership.Role != RoleAdmin {
		if svcErr := s.requireAdminRetained(ctx, workspaceMembership.WorkspaceId, workspaceMembership.ID); svcErr != nil {
			return nil, svcErr
		}
	}
	item, err := s.dao.Replace(ctx, workspaceMembership)
	if err != nil {
		return nil, errors.GeneralError("Unable to update workspace membership: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    workspaceMembershipEventSource,
		SourceID:  item.ID,
		EventType: api.UpdateEventType,
	}); evErr != nil {
		return nil, services.HandleUpdateError("WorkspaceMembership", evErr)
	}
	return item, nil
}

func (s *sqlWorkspaceMembershipService) Delete(ctx context.Context, id string) *errors.ServiceError {
	// Deleting the last admin of a workspace that hosts a refresh-bearing
	// ProviderBinding is rejected so the workspace keeps an administrator.
	found, err := s.dao.Get(ctx, id)
	if err != nil {
		return services.HandleGetError("WorkspaceMembership", "id", id, err)
	}
	if found.Role == RoleAdmin {
		if svcErr := s.requireAdminRetained(ctx, found.WorkspaceId, found.ID); svcErr != nil {
			return svcErr
		}
	}
	if err := s.dao.Delete(ctx, id); err != nil {
		return errors.GeneralError("Unable to delete workspace membership: %s", err)
	}
	if _, evErr := s.events.Create(ctx, &api.Event{
		Source:    workspaceMembershipEventSource,
		SourceID:  id,
		EventType: api.DeleteEventType,
	}); evErr != nil {
		return services.HandleDeleteError("WorkspaceMembership", evErr)
	}
	return nil
}

func (s *sqlWorkspaceMembershipService) All(ctx context.Context) (WorkspaceMembershipList, *errors.ServiceError) {
	items, err := s.dao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all workspace memberships: %s", err)
	}
	return items, nil
}

func (s *sqlWorkspaceMembershipService) FindByIDs(ctx context.Context, ids []string) (WorkspaceMembershipList, *errors.ServiceError) {
	items, err := s.dao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get workspace memberships: %s", err)
	}
	return items, nil
}

// requireAdminRetained returns a 422 when the workspace hosts a refresh-bearing
// ProviderBinding yet would be left with no admin membership after the operation
// (excludeID is the membership being demoted or deleted, "" for a create). The
// gateway is the authoritative enforcement point; this check reports the error at
// request time (agent-workspace-gateway-api.spec.md "Admin-Membership-Required
// Enforcement").
func (s *sqlWorkspaceMembershipService) requireAdminRetained(ctx context.Context, workspaceID, excludeID string) *errors.ServiceError {
	if workspaceID == "" {
		return nil
	}
	requires, err := s.dao.WorkspaceRequiresAdmin(ctx, workspaceID)
	if err != nil {
		return errors.GeneralError("Unable to validate workspace membership: %s", err)
	}
	if !requires {
		return nil
	}
	admins, err := s.dao.CountAdmins(ctx, workspaceID, excludeID)
	if err != nil {
		return errors.GeneralError("Unable to validate workspace membership: %s", err)
	}
	if admins == 0 {
		return errors.UnprocessableEntity("workspace %s hosts a provider binding with a refresh strategy and requires at least one admin membership", workspaceID)
	}
	return nil
}

func validateRole(role string) *errors.ServiceError {
	switch role {
	case RoleAdmin, RoleMember:
		return nil
	default:
		return errors.UnprocessableEntity("workspace membership role must be %q or %q", RoleAdmin, RoleMember)
	}
}

// validateSubject rejects a malformed or unbounded OIDC subject with a 400 before the
// membership is persisted or provisioned onto the gateway.
func validateSubject(subject string) *errors.ServiceError {
	if !subjectPattern.MatchString(subject) {
		return errors.Validation("workspace membership subject must be a non-empty token (letters, digits, and . _ : @ -), at most 255 characters")
	}
	return nil
}
