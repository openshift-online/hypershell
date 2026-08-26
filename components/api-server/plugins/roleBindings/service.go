package roleBindings

import (
	"context"
	"fmt"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
	"github.com/openshift-online/rh-trex-ai/pkg/errors"
	"github.com/openshift-online/rh-trex-ai/pkg/services"
)

type RoleBindingService interface {
	Get(ctx context.Context, id string) (*RoleBinding, *errors.ServiceError)
	GetUnscoped(ctx context.Context, id string) (*RoleBinding, *errors.ServiceError)
	Create(ctx context.Context, rb *RoleBinding) (*RoleBinding, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	CreateGatewayOwnerBinding(ctx context.Context, userID string, gatewayID string) error
	FindBindingsByUserID(ctx context.Context, userID string) ([]rbac.BindingSummary, error)
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
) RoleBindingService {
	return &sqlRoleBindingService{
		lockFactory: lockFactory,
		rbDao:       rbDao,
		roleDao:     roleDao,
		events:      events,
	}
}

var _ RoleBindingService = &sqlRoleBindingService{}

type sqlRoleBindingService struct {
	lockFactory db.LockFactory
	rbDao       RoleBindingDao
	roleDao     roles.RoleDao
	events      services.EventService
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

	if role.Name == roles.RoleGatewayOwner || role.Name == roles.RoleGatewayViewer {
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

	if role.Name == roles.RoleGatewayOwner || role.Name == roles.RoleGatewayViewer {
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
	case roles.RoleGatewayOwner, roles.RoleGatewayViewer:
		if rb.Scope != ScopeGateway {
			return errors.Validation("role %q requires scope=gateway", roleName)
		}
	}
	return nil
}
