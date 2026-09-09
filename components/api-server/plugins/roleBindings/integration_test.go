package roleBindings_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
)

func TestRoleList(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	list, resp, err := client.DefaultAPI.ListRoles(ctx).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(len(list.Items)).To(BeNumerically(">=", 3))

	foundCreator := false
	for _, role := range list.Items {
		if role.Name == "gateway:creator" {
			foundCreator = true
			Expect(role.GetBuiltIn()).To(BeTrue())
		}
	}
	Expect(foundCreator).To(BeTrue(), "expected gateway:creator role to exist")
}

func TestRoleGet(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	roleService := roles.Service(&environments.Environment().Services)
	creatorRole, svcErr := roleService.GetByName(context.Background(), roles.RoleGatewayCreator)
	Expect(svcErr).NotTo(HaveOccurred())

	role, resp, err := client.DefaultAPI.GetRole(ctx, creatorRole.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(role.Name).To(Equal("gateway:creator"))
}

func TestRoleBindingCreate_GatewayOwner(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	userService := users.Service(&environments.Environment().Services)
	rbService := roleBindings.Service(&environments.Environment().Services)
	roleService := roles.Service(&environments.Environment().Services)

	ownerRole, svcErr := roleService.GetByName(context.Background(), roles.RoleGatewayOwner)
	Expect(svcErr).NotTo(HaveOccurred())

	gatewayID := "gw-test-create"

	// Provision the account as gateway:owner so the HTTP create passes validation.
	callerID, userErr := userService.UpsertByUsername(context.Background(), strings.ToLower(account.Username), nil, nil)
	Expect(userErr).NotTo(HaveOccurred())
	Expect(rbService.CreateGatewayOwnerBinding(context.Background(), callerID, gatewayID)).To(Succeed())

	rbInput := openapi.RoleBinding{
		RoleId:    ownerRole.ID,
		Scope:     "gateway",
		GatewayId: &gatewayID,
	}

	rbOutput, resp, err := client.DefaultAPI.CreateRoleBinding(ctx).RoleBinding(rbInput).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*rbOutput.Id).NotTo(BeEmpty())
	Expect(rbOutput.RoleId).To(Equal(ownerRole.ID))
	Expect(rbOutput.Scope).To(Equal("gateway"))
}

func TestRoleBindingCreate_CreatorBlockedViaAPI(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	roleService := roles.Service(&environments.Environment().Services)
	creatorRole, svcErr := roleService.GetByName(context.Background(), roles.RoleGatewayCreator)
	Expect(svcErr).NotTo(HaveOccurred())

	rbInput := openapi.RoleBinding{
		RoleId: creatorRole.ID,
		Scope:  "global",
	}

	_, resp, err := client.DefaultAPI.CreateRoleBinding(ctx).RoleBinding(rbInput).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
}

func TestRoleBindingDelete(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	userService := users.Service(&environments.Environment().Services)
	rbService := roleBindings.Service(&environments.Environment().Services)
	roleService := roles.Service(&environments.Environment().Services)

	viewerRole, svcErr := roleService.GetByName(context.Background(), roles.RoleGatewayViewer)
	Expect(svcErr).NotTo(HaveOccurred())

	gatewayID := "gw-test-delete"

	// Provision the account as gateway:owner so the HTTP delete passes validation.
	callerID, userErr := userService.UpsertByUsername(context.Background(), strings.ToLower(account.Username), nil, nil)
	Expect(userErr).NotTo(HaveOccurred())
	Expect(rbService.CreateGatewayOwnerBinding(context.Background(), callerID, gatewayID)).To(Succeed())

	// Create a viewer binding to delete (bypassing HTTP so no owner check needed for setup).
	ownerCtx := context.WithValue(context.Background(), rbac.ContextUserIDKey, callerID)
	rb, createErr := rbService.Create(ownerCtx, &roleBindings.RoleBinding{
		RoleID:    viewerRole.ID,
		Scope:     roleBindings.ScopeGateway,
		GatewayID: &gatewayID,
	})
	Expect(createErr).NotTo(HaveOccurred())

	resp, err := client.DefaultAPI.DeleteRoleBinding(ctx, rb.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	_, resp, err = client.DefaultAPI.GetRoleBinding(ctx, rb.ID).Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
}

func TestRoleBindingList(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	roleService := roles.Service(&environments.Environment().Services)
	viewerRole, svcErr := roleService.GetByName(context.Background(), roles.RoleGatewayViewer)
	Expect(svcErr).NotTo(HaveOccurred())

	gatewayID := "gw-test-list"
	rbService := roleBindings.Service(&environments.Environment().Services)
	_, createErr := rbService.Create(context.Background(), &roleBindings.RoleBinding{
		RoleID:    viewerRole.ID,
		Scope:     roleBindings.ScopeGateway,
		GatewayID: &gatewayID,
	})
	Expect(createErr).NotTo(HaveOccurred())

	list, resp, err := client.DefaultAPI.ListRoleBindings(ctx).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(len(list.Items)).To(BeNumerically(">=", 1))
}

func TestRoleBindingScopeValidation_OwnerRequiresGatewayScope(t *testing.T) {
	test.RegisterIntegration(t)

	roleService := roles.Service(&environments.Environment().Services)
	ownerRole, svcErr := roleService.GetByName(context.Background(), roles.RoleGatewayOwner)
	Expect(svcErr).NotTo(HaveOccurred())

	rbService := roleBindings.Service(&environments.Environment().Services)
	_, err := rbService.Create(context.Background(), &roleBindings.RoleBinding{
		RoleID: ownerRole.ID,
		Scope:  roleBindings.ScopeGlobal,
	})
	Expect(err).To(HaveOccurred())
	Expect(err.HttpCode).To(Equal(http.StatusBadRequest))
}

func TestRoleBindingScopeValidation_GatewayScopeRequiresGatewayID(t *testing.T) {
	test.RegisterIntegration(t)

	roleService := roles.Service(&environments.Environment().Services)
	viewerRole, svcErr := roleService.GetByName(context.Background(), roles.RoleGatewayViewer)
	Expect(svcErr).NotTo(HaveOccurred())

	rbService := roleBindings.Service(&environments.Environment().Services)
	_, err := rbService.Create(context.Background(), &roleBindings.RoleBinding{
		RoleID: viewerRole.ID,
		Scope:  roleBindings.ScopeGateway,
	})
	Expect(err).To(HaveOccurred())
	Expect(err.HttpCode).To(Equal(http.StatusBadRequest))
}

func TestGrantValidation_OwnerCanGrantViewerOnSameGateway(t *testing.T) {
	test.RegisterIntegration(t)

	roleService := roles.Service(&environments.Environment().Services)
	rbService := roleBindings.Service(&environments.Environment().Services)
	userService := users.Service(&environments.Environment().Services)

	callerUser, userErr := userService.UpsertByUsername(context.Background(), "grant-owner", nil, nil)
	Expect(userErr).NotTo(HaveOccurred())

	ownerRole, _ := roleService.GetByName(context.Background(), roles.RoleGatewayOwner)
	viewerRole, _ := roleService.GetByName(context.Background(), roles.RoleGatewayViewer)

	gatewayID := "gw-grant-test"

	_, ownerErr := rbService.Create(context.Background(), &roleBindings.RoleBinding{
		RoleID:    ownerRole.ID,
		Scope:     roleBindings.ScopeGateway,
		UserID:    &callerUser,
		GatewayID: &gatewayID,
	})
	Expect(ownerErr).NotTo(HaveOccurred())

	callerCtx := context.WithValue(context.Background(), rbac.ContextUserIDKey, callerUser)

	_, viewerErr := rbService.Create(callerCtx, &roleBindings.RoleBinding{
		RoleID:    viewerRole.ID,
		Scope:     roleBindings.ScopeGateway,
		GatewayID: &gatewayID,
	})
	Expect(viewerErr).NotTo(HaveOccurred())
}

func TestGrantValidation_OwnerCanGrantOwnerOnSameGateway(t *testing.T) {
	test.RegisterIntegration(t)

	roleService := roles.Service(&environments.Environment().Services)
	rbService := roleBindings.Service(&environments.Environment().Services)
	userService := users.Service(&environments.Environment().Services)

	callerUser, userErr := userService.UpsertByUsername(context.Background(), "grant-owner-owner", nil, nil)
	Expect(userErr).NotTo(HaveOccurred())

	ownerRole, _ := roleService.GetByName(context.Background(), roles.RoleGatewayOwner)

	gatewayID := "gw-grant-owner-test"

	_, ownerErr := rbService.Create(context.Background(), &roleBindings.RoleBinding{
		RoleID:    ownerRole.ID,
		Scope:     roleBindings.ScopeGateway,
		UserID:    &callerUser,
		GatewayID: &gatewayID,
	})
	Expect(ownerErr).NotTo(HaveOccurred())

	callerCtx := context.WithValue(context.Background(), rbac.ContextUserIDKey, callerUser)

	_, grantErr := rbService.Create(callerCtx, &roleBindings.RoleBinding{
		RoleID:    ownerRole.ID,
		Scope:     roleBindings.ScopeGateway,
		GatewayID: &gatewayID,
	})
	Expect(grantErr).NotTo(HaveOccurred())
}

func TestGrantValidation_NonOwnerCannotGrantOnGateway(t *testing.T) {
	test.RegisterIntegration(t)

	roleService := roles.Service(&environments.Environment().Services)
	rbService := roleBindings.Service(&environments.Environment().Services)
	userService := users.Service(&environments.Environment().Services)

	callerUser, userErr := userService.UpsertByUsername(context.Background(), "grant-viewer-only", nil, nil)
	Expect(userErr).NotTo(HaveOccurred())

	viewerRole, _ := roleService.GetByName(context.Background(), roles.RoleGatewayViewer)

	gatewayID := "gw-grant-fail-test"

	_, viewerErr := rbService.Create(context.Background(), &roleBindings.RoleBinding{
		RoleID:    viewerRole.ID,
		Scope:     roleBindings.ScopeGateway,
		UserID:    &callerUser,
		GatewayID: &gatewayID,
	})
	Expect(viewerErr).NotTo(HaveOccurred())

	callerCtx := context.WithValue(context.Background(), rbac.ContextUserIDKey, callerUser)

	_, grantErr := rbService.Create(callerCtx, &roleBindings.RoleBinding{
		RoleID:    viewerRole.ID,
		Scope:     roleBindings.ScopeGateway,
		GatewayID: &gatewayID,
	})
	Expect(grantErr).To(HaveOccurred())
	Expect(grantErr.HttpCode).To(Equal(http.StatusForbidden))
}

func TestGrantValidation_CrossGatewayEscalation(t *testing.T) {
	test.RegisterIntegration(t)

	roleService := roles.Service(&environments.Environment().Services)
	rbService := roleBindings.Service(&environments.Environment().Services)
	userService := users.Service(&environments.Environment().Services)

	callerUser, userErr := userService.UpsertByUsername(context.Background(), "crossgw-caller", nil, nil)
	Expect(userErr).NotTo(HaveOccurred())

	ownerRole, _ := roleService.GetByName(context.Background(), roles.RoleGatewayOwner)
	viewerRole, _ := roleService.GetByName(context.Background(), roles.RoleGatewayViewer)

	gwOwned := "gw-owned"
	gwOther := "gw-other"

	_, createErr := rbService.Create(context.Background(), &roleBindings.RoleBinding{
		RoleID:    ownerRole.ID,
		Scope:     roleBindings.ScopeGateway,
		UserID:    &callerUser,
		GatewayID: &gwOwned,
	})
	Expect(createErr).NotTo(HaveOccurred())

	callerCtx := context.WithValue(context.Background(), rbac.ContextUserIDKey, callerUser)

	_, crossGWErr := rbService.Create(callerCtx, &roleBindings.RoleBinding{
		RoleID:    viewerRole.ID,
		Scope:     roleBindings.ScopeGateway,
		GatewayID: &gwOther,
	})
	Expect(crossGWErr).To(HaveOccurred())
	Expect(crossGWErr.HttpCode).To(Equal(http.StatusForbidden))
}

// TestUserProvisioningMiddleware_DefaultRoleAssignedWithNoJWTRoles drives the
// real HTTP middleware path with a token that carries no realm_access roles and
// asserts that the user receives a gateway:creator binding. This is the exact
// scenario RBAC_ENFORCE=true must handle: a brand-new user with no Keycloak roles.
func TestUserProvisioningMiddleware_DefaultRoleAssignedWithNoJWTRoles(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	// NewRandAccount creates a JWT with no realm_access claims.
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// GET /roles is RBAC-exempt so it succeeds regardless of bindings,
	// but it still runs UserProvisioningMiddleware on the apiV1Router.
	_, resp, err := client.DefaultAPI.ListRoles(ctx).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	// The JWT helper lowercases the username; UpsertByUsername is idempotent.
	userService := users.Service(&environments.Environment().Services)
	rbService := roleBindings.Service(&environments.Environment().Services)
	userID, userErr := userService.UpsertByUsername(context.Background(), strings.ToLower(account.Username), nil, nil)
	Expect(userErr).NotTo(HaveOccurred())

	bindings, findErr := rbService.FindBindingsByUserID(context.Background(), userID)
	Expect(findErr).NotTo(HaveOccurred())

	found := false
	for _, b := range bindings {
		if b.RoleName == roles.RoleGatewayCreator && b.Scope == roleBindings.ScopeGlobal {
			found = true
		}
	}
	Expect(found).To(BeTrue(), "expected gateway:creator binding after first authenticated request with no JWT roles")
}

// TestSyncJWTRoles_DefaultRoleAssigned verifies that a newly provisioned user
// receives a gateway:creator binding even when the JWT carries no roles.
func TestSyncJWTRoles_DefaultRoleAssigned(t *testing.T) {
	test.RegisterIntegration(t)

	rbService := roleBindings.Service(&environments.Environment().Services)
	userService := users.Service(&environments.Environment().Services)

	userID, userErr := userService.UpsertByUsername(context.Background(), "sync-default-new", nil, nil)
	Expect(userErr).NotTo(HaveOccurred())

	syncErr := rbService.SyncJWTRoles(context.Background(), userID, nil)
	Expect(syncErr).NotTo(HaveOccurred())

	bindings, findErr := rbService.FindBindingsByUserID(context.Background(), userID)
	Expect(findErr).NotTo(HaveOccurred())

	found := false
	for _, b := range bindings {
		if b.RoleName == roles.RoleGatewayCreator && b.Scope == roleBindings.ScopeGlobal {
			found = true
		}
	}
	Expect(found).To(BeTrue(), "expected gateway:creator global binding after sync with empty JWT")
}

// TestSyncJWTRoles_DefaultRoleIsIdempotent verifies that calling SyncJWTRoles
// twice does not create duplicate gateway:creator bindings.
func TestSyncJWTRoles_DefaultRoleIsIdempotent(t *testing.T) {
	test.RegisterIntegration(t)

	rbService := roleBindings.Service(&environments.Environment().Services)
	userService := users.Service(&environments.Environment().Services)

	userID, userErr := userService.UpsertByUsername(context.Background(), "sync-default-idempotent", nil, nil)
	Expect(userErr).NotTo(HaveOccurred())

	Expect(rbService.SyncJWTRoles(context.Background(), userID, nil)).To(Succeed())
	Expect(rbService.SyncJWTRoles(context.Background(), userID, nil)).To(Succeed())

	bindings, findErr := rbService.FindBindingsByUserID(context.Background(), userID)
	Expect(findErr).NotTo(HaveOccurred())

	creatorCount := 0
	for _, b := range bindings {
		if b.RoleName == roles.RoleGatewayCreator && b.Scope == roleBindings.ScopeGlobal {
			creatorCount++
		}
	}
	Expect(creatorCount).To(Equal(1), "expected exactly one gateway:creator binding after two syncs")
}

// TestSyncJWTRoles_DefaultRoleNotRemovedWhenAbsentFromJWT verifies that a user's
// gateway:creator binding is retained across syncs even when the JWT never carries it.
func TestSyncJWTRoles_DefaultRoleNotRemovedWhenAbsentFromJWT(t *testing.T) {
	test.RegisterIntegration(t)

	rbService := roleBindings.Service(&environments.Environment().Services)
	userService := users.Service(&environments.Environment().Services)

	userID, userErr := userService.UpsertByUsername(context.Background(), "sync-default-persist", nil, nil)
	Expect(userErr).NotTo(HaveOccurred())

	// First sync assigns the default.
	Expect(rbService.SyncJWTRoles(context.Background(), userID, nil)).To(Succeed())

	// Second sync with still-empty JWT must not revoke it.
	Expect(rbService.SyncJWTRoles(context.Background(), userID, nil)).To(Succeed())

	bindings, findErr := rbService.FindBindingsByUserID(context.Background(), userID)
	Expect(findErr).NotTo(HaveOccurred())

	found := false
	for _, b := range bindings {
		if b.RoleName == roles.RoleGatewayCreator && b.Scope == roleBindings.ScopeGlobal {
			found = true
		}
	}
	Expect(found).To(BeTrue(), "gateway:creator binding must survive a sync with an empty JWT")
}

// TestSyncJWTRoles_DefaultAndJWTRolesBothApplied verifies that a JWT-carried role
// (platform:admin) is synced in addition to the default gateway:creator.
// Default roles are always applied regardless of what the JWT carries.
func TestSyncJWTRoles_DefaultAndJWTRolesBothApplied(t *testing.T) {
	test.RegisterIntegration(t)

	rbService := roleBindings.Service(&environments.Environment().Services)
	userService := users.Service(&environments.Environment().Services)

	userID, userErr := userService.UpsertByUsername(context.Background(), "sync-default-and-jwt", nil, nil)
	Expect(userErr).NotTo(HaveOccurred())

	syncErr := rbService.SyncJWTRoles(context.Background(), userID, []string{roles.RolePlatformAdmin})
	Expect(syncErr).NotTo(HaveOccurred())

	bindings, findErr := rbService.FindBindingsByUserID(context.Background(), userID)
	Expect(findErr).NotTo(HaveOccurred())

	roleNames := make(map[string]bool)
	for _, b := range bindings {
		if b.Scope == roleBindings.ScopeGlobal {
			roleNames[b.RoleName] = true
		}
	}
	Expect(roleNames[roles.RolePlatformAdmin]).To(BeTrue(), "expected JWT-carried platform:admin binding")
	Expect(roleNames[roles.RoleGatewayCreator]).To(BeTrue(), "expected default gateway:creator binding alongside JWT roles")
}
