package roles_test

import (
	"context"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/pkg/environments"
)

func TestRoleListReturnsBuiltInRoles(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	list, resp, err := client.DefaultAPI.ListRoles(ctx).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(len(list.Items)).To(BeNumerically(">=", 3))

	expectedRoles := map[string]bool{
		"gateway:creator": false,
		"gateway:owner":   false,
		"gateway:viewer":  false,
	}
	for _, role := range list.Items {
		if _, ok := expectedRoles[role.Name]; ok {
			expectedRoles[role.Name] = true
			Expect(role.GetBuiltIn()).To(BeTrue())
		}
	}
	for name, found := range expectedRoles {
		Expect(found).To(BeTrue(), "expected built-in role %q to exist", name)
	}
}

func TestRoleGetById(t *testing.T) {
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
	Expect(*role.Kind).To(Equal("Role"))
}

func TestRoleGetNotFound(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, resp, err := client.DefaultAPI.GetRole(ctx, "nonexistent-id").Execute()
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
}

func TestRoleListUnauthenticated(t *testing.T) {
	_, client := test.RegisterIntegration(t)

	_, _, err := client.DefaultAPI.ListRoles(context.Background()).Execute()
	Expect(err).To(HaveOccurred())
}
