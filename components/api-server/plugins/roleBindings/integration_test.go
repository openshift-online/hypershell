package roleBindings_test

import (
	"context"
	"net/http"
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

	roleService := roles.Service(&environments.Environment().Services)
	ownerRole, svcErr := roleService.GetByName(context.Background(), roles.RoleGatewayOwner)
	Expect(svcErr).NotTo(HaveOccurred())

	gatewayID := "gw-test-create"

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

	roleService := roles.Service(&environments.Environment().Services)
	viewerRole, svcErr := roleService.GetByName(context.Background(), roles.RoleGatewayViewer)
	Expect(svcErr).NotTo(HaveOccurred())

	gatewayID := "gw-test-delete"
	rbService := roleBindings.Service(&environments.Environment().Services)
	rb, createErr := rbService.Create(context.Background(), &roleBindings.RoleBinding{
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
