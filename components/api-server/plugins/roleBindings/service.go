package roleBindings

import (
	"context"
	"fmt"
	"sort"

	"github.com/golang/glog"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/services"
)

type RoleBindingService interface {
	Get(ctx context.Context, id string) (*RoleBinding, *errors.ServiceError)
	GetUnscoped(ctx context.Context, id string) (*RoleBinding, *errors.ServiceError)
	Create(ctx context.Context, rb *RoleBinding) (*RoleBinding, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	CreateGatewayOwnerBinding(ctx context.Context, userID string, gatewayID string) error
	FindBindingsByUserID(ctx context.Context, userID string) ([]rbac.BindingSummary, error)
	BindingsForGateway(ctx context.Context, gatewayID string) (RoleBindingList, *errors.ServiceError)
	// SetGatewayRole converges the user's bindings on gatewayID to exactly one
	// binding for roleName (one of gateway:owner/admin/viewer), in place where
	// possible (GAM-05, GAM-10). It emits an UPDATED watch event when an existing
	// single binding's role_id is changed in place (no zero-access window), a
	// CREATED event when a new binding is made, and DELETED events for any surplus
	// bindings removed during convergence. It performs NO caller authorization;
	// the access facade authorizes before calling.
	SetGatewayRole(ctx context.Context, userID string, gatewayID string, roleName string) (*RoleBinding, *errors.ServiceError)
	// RevokeGatewayAccess deletes all of the user's gateway-scoped bindings on
	// gatewayID (one DELETED event each) and returns the number removed. It
	// performs NO caller authorization.
	RevokeGatewayAccess(ctx context.Context, userID string, gatewayID string) (int, *errors.ServiceError)
	FindByUserID(ctx context.Context, userID string) (RoleBindingList, *errors.ServiceError)
	FindGatewayIDsByUserID(ctx context.Context, userID string) ([]string, *errors.ServiceError)
	FindOwnerUsernamesByGatewayIDs(ctx context.Context, gatewayIDs []string) (map[string]string, error)
	All(ctx context.Context) (RoleBindingList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (RoleBindingList, *errors.ServiceError)
	SyncJWTRoles(ctx context.Context, userID string, jwtRoles []string) error

	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewRoleBindingService(
	lockFactory db.LockFactory,
	rbDao RoleBindingDao,
	roleDao roles.RoleDao,
	events services.EventService,
	defaultRoles []string,
) RoleBindingService {
	for _, r := range defaultRoles {
		if !roles.JWTSyncedRoles[r] {
			glog.Warningf("RBAC_DEFAULT_ROLES: role %q is not in JWTSyncedRoles and will be ignored; add it to JWTSyncedRoles to make it sync-eligible", r)
		}
	}
	return &sqlRoleBindingService{
		lockFactory:  lockFactory,
		rbDao:        rbDao,
		roleDao:      roleDao,
		events:       events,
		defaultRoles: defaultRoles,
	}
}

var _ RoleBindingService = &sqlRoleBindingService{}

type sqlRoleBindingService struct {
	lockFactory  db.LockFactory
	rbDao        RoleBindingDao
	roleDao      roles.RoleDao
	events       services.EventService
	defaultRoles []string
}

func (s *sqlRoleBindingService) CreateGatewayOwnerBinding(ctx context.Context, userID string, gatewayID string) error {
	ownerRole, roleErr := s.roleDao.GetByName(ctx, roles.RoleGatewayOwner)
	if roleErr != nil {
		return roleErr
	}

	rb := &RoleBinding{
		RoleID:    ownerRole.ID,
		Scope:     ScopeGateway,
		UserID:    &userID,
		GatewayID: &gatewayID,
	}

	// Intentionally no CaptureTraceContext here: this is a server-initiated
	// binding (created as a side effect of gateway provisioning), not a client
	// request, so there is no meaningful originating request span to link. The
	// trace context stays NULL and the resulting reconcile is knowingly
	// link-less (RTC-01 tolerates NULL trace context).
	_, createErr := s.rbDao.Create(ctx, rb)
	if createErr != nil {
		return createErr
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "RoleBindings",
		SourceID:  rb.ID,
		EventType: api.CreateEventType,
	})
	// events.Create returns a concrete *errors.ServiceError. Returning it
	// directly would box a typed nil into the error interface, making the
	// result non-nil even on success. Convert explicitly.
	if evErr != nil {
		return evErr
	}
	return nil
}

func (s *sqlRoleBindingService) SyncJWTRoles(ctx context.Context, userID string, jwtRoles []string) error {
	jwtRoleSet := make(map[string]bool)
	for _, r := range jwtRoles {
		if roles.JWTSyncedRoles[r] {
			jwtRoleSet[r] = true
		}
	}
	// Default roles are always merged regardless of what the JWT carries.
	// They represent the platform's baseline posture: every authenticated
	// principal receives these capabilities unless explicitly disabled via
	// RBAC_DEFAULT_ROLES=. Only roles in JWTSyncedRoles participate in the
	// sync lifecycle (idempotent add, never revoked by JWT absence).
	for _, r := range s.defaultRoles {
		if roles.JWTSyncedRoles[r] {
			jwtRoleSet[r] = true
		}
	}

	existing, err := s.rbDao.FindByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("unable to find bindings for user %s: %w", userID, err)
	}

	existingSynced := make(map[string]*RoleBinding)
	for _, b := range existing {
		if b.Scope != ScopeGlobal {
			continue
		}
		role, roleErr := s.roleDao.Get(ctx, b.RoleID)
		if roleErr != nil {
			continue
		}
		if roles.JWTSyncedRoles[role.Name] {
			existingSynced[role.Name] = b
		}
	}

	for roleName := range jwtRoleSet {
		if _, exists := existingSynced[roleName]; exists {
			continue
		}
		role, roleErr := s.roleDao.GetByName(ctx, roleName)
		if roleErr != nil {
			return fmt.Errorf("unable to find role %s: %w", roleName, roleErr)
		}
		rb := &RoleBinding{
			RoleID: role.ID,
			Scope:  ScopeGlobal,
			UserID: &userID,
		}
		// Intentionally no CaptureTraceContext here: JWT role sync is driven by
		// the login/token flow, not a client mutation of this resource, so there
		// is no originating request span worth linking. NULL trace context is a
		// valid state (RTC-01) and these reconciles are knowingly link-less.
		if _, createErr := s.rbDao.Create(ctx, rb); createErr != nil {
			return fmt.Errorf("unable to create binding for role %s: %w", roleName, createErr)
		}
	}

	for roleName, binding := range existingSynced {
		if jwtRoleSet[roleName] {
			continue
		}
		if err := s.rbDao.Delete(ctx, binding.ID); err != nil {
			return fmt.Errorf("unable to remove revoked binding for role %s: %w", roleName, err)
		}
	}

	return nil
}

func (s *sqlRoleBindingService) OnUpsert(ctx context.Context, id string) error {
	return nil
}

func (s *sqlRoleBindingService) OnDelete(ctx context.Context, id string) error {
	return nil
}

func (s *sqlRoleBindingService) Get(ctx context.Context, id string) (*RoleBinding, *errors.ServiceError) {
	rb, err := s.rbDao.Get(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("RoleBinding", "id", id, err)
	}
	return rb, nil
}

func (s *sqlRoleBindingService) GetUnscoped(ctx context.Context, id string) (*RoleBinding, *errors.ServiceError) {
	rb, err := s.rbDao.GetUnscoped(ctx, id)
	if err != nil {
		return nil, services.HandleGetError("RoleBinding", "id", id, err)
	}
	return rb, nil
}

func (s *sqlRoleBindingService) Create(ctx context.Context, rb *RoleBinding) (*RoleBinding, *errors.ServiceError) {
	if err := s.validateScope(rb); err != nil {
		return nil, err
	}

	role, roleErr := s.roleDao.Get(ctx, rb.RoleID)
	if roleErr != nil {
		return nil, errors.Validation("invalid role_id: role not found")
	}

	if err := s.validateScopeMatchesRole(role.Name, rb); err != nil {
		return nil, err
	}

	if role.Name == roles.RoleGatewayOwner || role.Name == roles.RoleGatewayAdmin || role.Name == roles.RoleGatewayViewer {
		if err := s.validateCallerOwnsGateway(ctx, rb.GatewayID); err != nil {
			return nil, err
		}
	}

	if role.Name == roles.RoleGatewayCreator {
		return nil, errors.Forbidden("gateway:creator can only be assigned via Keycloak")
	}

	if role.Name == roles.RolePlatformAdmin {
		return nil, errors.Forbidden("platform:admin can only be assigned via Keycloak")
	}

	rb.CaptureTraceContext(ctx)
	rb, createErr := s.rbDao.Create(ctx, rb)
	if createErr != nil {
		return nil, services.HandleCreateError("RoleBinding", createErr)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "RoleBindings",
		SourceID:  rb.ID,
		EventType: api.CreateEventType,
	})
	if evErr != nil {
		return nil, services.HandleCreateError("RoleBinding", evErr)
	}

	return rb, nil
}

func (s *sqlRoleBindingService) Delete(ctx context.Context, id string) *errors.ServiceError {
	rb, svcErr := s.Get(ctx, id)
	if svcErr != nil {
		return svcErr
	}

	role, roleErr := s.roleDao.Get(ctx, rb.RoleID)
	if roleErr != nil {
		return errors.Validation("invalid role_id: role not found")
	}

	if role.Name == roles.RoleGatewayOwner || role.Name == roles.RoleGatewayAdmin || role.Name == roles.RoleGatewayViewer {
		if err := s.validateCallerOwnsGateway(ctx, rb.GatewayID); err != nil {
			return err
		}
	}

	if err := s.rbDao.Delete(ctx, id); err != nil {
		return services.HandleDeleteError("RoleBinding", errors.GeneralError("Unable to delete role binding: %s", err))
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "RoleBindings",
		SourceID:  id,
		EventType: api.DeleteEventType,
	})
	if evErr != nil {
		return services.HandleDeleteError("RoleBinding", evErr)
	}

	return nil
}

func (s *sqlRoleBindingService) BindingsForGateway(ctx context.Context, gatewayID string) (RoleBindingList, *errors.ServiceError) {
	bindings, err := s.rbDao.FindByGatewayID(ctx, gatewayID)
	if err != nil {
		return nil, errors.GeneralError("Unable to get gateway role bindings: %s", err)
	}
	return bindings, nil
}

// gatewayBindingsForUser returns the user's gateway-scoped bindings on gatewayID.
func (s *sqlRoleBindingService) gatewayBindingsForUser(ctx context.Context, userID, gatewayID string) (RoleBindingList, error) {
	all, err := s.rbDao.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := RoleBindingList{}
	for _, b := range all {
		if b.Scope == ScopeGateway && b.GatewayID != nil && *b.GatewayID == gatewayID {
			out = append(out, b)
		}
	}
	return out, nil
}

func (s *sqlRoleBindingService) SetGatewayRole(ctx context.Context, userID, gatewayID, roleName string) (*RoleBinding, *errors.ServiceError) {
	if roleName != roles.RoleGatewayOwner && roleName != roles.RoleGatewayAdmin && roleName != roles.RoleGatewayViewer {
		return nil, errors.Validation("invalid gateway role: %q", roleName)
	}
	targetRole, roleErr := s.roleDao.GetByName(ctx, roleName)
	if roleErr != nil {
		return nil, errors.Validation("invalid role %q: %s", roleName, roleErr)
	}

	existing, err := s.gatewayBindingsForUser(ctx, userID, gatewayID)
	if err != nil {
		return nil, errors.GeneralError("unable to look up existing bindings: %s", err)
	}

	// No binding yet: create one (CREATED event).
	if len(existing) == 0 {
		uid := userID
		gid := gatewayID
		rb := &RoleBinding{RoleID: targetRole.ID, Scope: ScopeGateway, UserID: &uid, GatewayID: &gid}
		rb.CaptureTraceContext(ctx)
		created, createErr := s.rbDao.Create(ctx, rb)
		if createErr != nil {
			return nil, services.HandleCreateError("RoleBinding", createErr)
		}
		if evErr := s.emitBindingEvent(ctx, created.ID, api.CreateEventType); evErr != nil {
			return nil, evErr
		}
		return created, nil
	}

	// Converge to exactly one binding (GAM-10): keep the earliest-created,
	// delete any surplus so a user never accumulates conflicting bindings.
	sort.Slice(existing, func(i, j int) bool {
		if existing[i].CreatedAt.Equal(existing[j].CreatedAt) {
			return existing[i].ID < existing[j].ID
		}
		return existing[i].CreatedAt.Before(existing[j].CreatedAt)
	})
	keep := existing[0]
	for _, extra := range existing[1:] {
		if delErr := s.rbDao.Delete(ctx, extra.ID); delErr != nil {
			return nil, services.HandleDeleteError("RoleBinding", errors.GeneralError("unable to delete duplicate binding: %s", delErr))
		}
		if evErr := s.emitBindingEvent(ctx, extra.ID, api.DeleteEventType); evErr != nil {
			return nil, evErr
		}
	}

	// Already the target role: idempotent no-op success (GAM-10).
	if keep.RoleID == targetRole.ID {
		return keep, nil
	}

	// Change in place (GAM-05): a single UPDATED event, no zero-access window.
	keep.RoleID = targetRole.ID
	keep.CaptureTraceContext(ctx)
	updated, updErr := s.rbDao.Update(ctx, keep)
	if updErr != nil {
		return nil, services.HandleUpdateError("RoleBinding", updErr)
	}
	if evErr := s.emitBindingEvent(ctx, updated.ID, api.UpdateEventType); evErr != nil {
		return nil, evErr
	}
	return updated, nil
}

func (s *sqlRoleBindingService) RevokeGatewayAccess(ctx context.Context, userID, gatewayID string) (int, *errors.ServiceError) {
	existing, err := s.gatewayBindingsForUser(ctx, userID, gatewayID)
	if err != nil {
		return 0, errors.GeneralError("unable to look up existing bindings: %s", err)
	}
	for _, b := range existing {
		if delErr := s.rbDao.Delete(ctx, b.ID); delErr != nil {
			return 0, services.HandleDeleteError("RoleBinding", errors.GeneralError("unable to delete binding: %s", delErr))
		}
		if evErr := s.emitBindingEvent(ctx, b.ID, api.DeleteEventType); evErr != nil {
			return 0, evErr
		}
	}
	return len(existing), nil
}

func (s *sqlRoleBindingService) emitBindingEvent(ctx context.Context, sourceID string, eventType api.EventType) *errors.ServiceError {
	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "RoleBindings",
		SourceID:  sourceID,
		EventType: eventType,
	})
	if evErr != nil {
		return evErr
	}
	return nil
}

func (s *sqlRoleBindingService) FindBindingsByUserID(ctx context.Context, userID string) ([]rbac.BindingSummary, error) {
	bindings, err := s.rbDao.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(bindings) == 0 {
		return nil, nil
	}

	roleIDs := make([]string, 0, len(bindings))
	for _, b := range bindings {
		roleIDs = append(roleIDs, b.RoleID)
	}

	roleList, roleErr := s.roleDao.FindByIDs(ctx, roleIDs)
	if roleErr != nil {
		return nil, fmt.Errorf("unable to batch-load roles for bindings: %w", roleErr)
	}

	roleIndex := roleList.Index()

	summaries := make([]rbac.BindingSummary, 0, len(bindings))
	for _, b := range bindings {
		role, ok := roleIndex[b.RoleID]
		if !ok {
			return nil, fmt.Errorf("role %s referenced by binding %s not found", b.RoleID, b.ID)
		}
		summaries = append(summaries, rbac.BindingSummary{
			RoleName:  role.Name,
			Scope:     b.Scope,
			GatewayID: b.GatewayID,
		})
	}
	return summaries, nil
}

func (s *sqlRoleBindingService) FindByUserID(ctx context.Context, userID string) (RoleBindingList, *errors.ServiceError) {
	bindings, err := s.rbDao.FindByUserID(ctx, userID)
	if err != nil {
		return nil, errors.GeneralError("Unable to get role bindings: %s", err)
	}
	return bindings, nil
}

func (s *sqlRoleBindingService) FindGatewayIDsByUserID(ctx context.Context, userID string) ([]string, *errors.ServiceError) {
	ids, err := s.rbDao.FindGatewayIDsByUserID(ctx, userID)
	if err != nil {
		return nil, errors.GeneralError("Unable to get gateway IDs for user: %s", err)
	}
	return ids, nil
}

func (s *sqlRoleBindingService) FindOwnerUsernamesByGatewayIDs(ctx context.Context, gatewayIDs []string) (map[string]string, error) {
	return s.rbDao.FindOwnerUsernamesByGatewayIDs(ctx, gatewayIDs)
}

func (s *sqlRoleBindingService) All(ctx context.Context) (RoleBindingList, *errors.ServiceError) {
	bindings, err := s.rbDao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all role bindings: %s", err)
	}
	return bindings, nil
}

func (s *sqlRoleBindingService) FindByIDs(ctx context.Context, ids []string) (RoleBindingList, *errors.ServiceError) {
	bindings, err := s.rbDao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get role bindings: %s", err)
	}
	return bindings, nil
}

func (s *sqlRoleBindingService) validateScope(rb *RoleBinding) *errors.ServiceError {
	switch rb.Scope {
	case ScopeGlobal:
		if rb.GatewayID != nil {
			return errors.Validation("global scope must not have gateway_id")
		}
	case ScopeGateway:
		if rb.GatewayID == nil {
			return errors.Validation("gateway scope requires gateway_id")
		}
	default:
		return errors.Validation("invalid scope: %q; supported: global, gateway", rb.Scope)
	}
	return nil
}

func (s *sqlRoleBindingService) validateCallerOwnsGateway(ctx context.Context, gatewayID *string) *errors.ServiceError {
	if gatewayID == nil {
		return errors.Forbidden("gateway_id is required for gateway-scoped grants")
	}

	callerUserID := rbac.GetUserIDFromContext(ctx)
	if callerUserID == "" {
		return nil
	}

	callerBindings, err := s.rbDao.FindByUserID(ctx, callerUserID)
	if err != nil {
		return errors.GeneralError("unable to look up caller bindings: %s", err)
	}

	for _, b := range callerBindings {
		if b.GatewayID != nil && *b.GatewayID == *gatewayID {
			role, roleErr := s.roleDao.Get(ctx, b.RoleID)
			if roleErr != nil {
				continue
			}
			if role.Name == roles.RoleGatewayOwner {
				return nil
			}
		}
	}

	return errors.Forbidden("caller must be gateway:owner on the target gateway to grant bindings")
}

func (s *sqlRoleBindingService) validateScopeMatchesRole(roleName string, rb *RoleBinding) *errors.ServiceError {
	switch roleName {
	case roles.RoleGatewayCreator:
		if rb.Scope != ScopeGlobal {
			return errors.Validation("role %q requires scope=global", roleName)
		}
	case roles.RoleGatewayOwner, roles.RoleGatewayAdmin, roles.RoleGatewayViewer:
		if rb.Scope != ScopeGateway {
			return errors.Validation("role %q requires scope=gateway", roleName)
		}
	}
	return nil
}
