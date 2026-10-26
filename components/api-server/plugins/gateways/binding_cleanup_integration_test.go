package gateways_test

import (
	"context"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// Verifies: GTW-DEL-09.01
func TestGatewayDeleteRemovesItsRoleBindings(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	ctx := h.NewAuthenticatedContext(h.NewRandAccount())

	deleted, err := newGateway(h.NewID())
	Expect(err).NotTo(HaveOccurred())
	kept, err := newGateway(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	envServices := &environments.Environment().Services
	bindingService := roleBindings.Service(envServices)
	userService := users.Service(envServices)
	ownerID, uerr := userService.UpsertByUsername(context.Background(), "binding-cleanup-owner", nil, nil)
	Expect(uerr).NotTo(HaveOccurred())
	viewerID, uerr := userService.UpsertByUsername(context.Background(), "binding-cleanup-viewer", nil, nil)
	Expect(uerr).NotTo(HaveOccurred())

	Expect(bindingService.CreateGatewayOwnerBinding(context.Background(), ownerID, deleted.ID)).To(Succeed())
	_, svcErr := bindingService.SetGatewayRole(context.Background(), viewerID, deleted.ID, roles.RoleGatewayViewer)
	Expect(svcErr).To(BeNil())
	Expect(bindingService.CreateGatewayOwnerBinding(context.Background(), ownerID, kept.ID)).To(Succeed())

	before, svcErr := bindingService.BindingsForGateway(context.Background(), deleted.ID)
	Expect(svcErr).To(BeNil())
	Expect(before).To(HaveLen(2))

	resp, err := client.DefaultAPI.DeleteGateway(ctx, deleted.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	after, svcErr := bindingService.BindingsForGateway(context.Background(), deleted.ID)
	Expect(svcErr).To(BeNil())
	Expect(after).To(BeEmpty(), "a deleted gateway's role bindings must be deleted with it")
	for _, b := range before {
		gone, getErr := bindingService.GetUnscoped(context.Background(), b.ID)
		Expect(getErr).To(BeNil())
		Expect(gone.DeletedAt.Valid).To(BeTrue(), "binding %s should be soft-deleted", b.ID)
	}

	survivors, svcErr := bindingService.BindingsForGateway(context.Background(), kept.ID)
	Expect(svcErr).To(BeNil())
	Expect(survivors).To(HaveLen(1), "another gateway's bindings must survive")
}
