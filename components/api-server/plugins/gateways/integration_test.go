package gateways_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"gopkg.in/resty.v1"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

func TestGatewayGet(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, _, err := client.DefaultAPI.GetGateway(context.Background(), "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 401 but got nil error")

	_, resp, err := client.DefaultAPI.GetGateway(ctx, "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

	gatewayModel, err := newGateway(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	gatewayOutput, resp, err := client.DefaultAPI.GetGateway(ctx, gatewayModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	Expect(*gatewayOutput.Id).To(Equal(gatewayModel.ID), "found object does not match test object")
	Expect(*gatewayOutput.Kind).To(Equal("Gateway"))
	Expect(*gatewayOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateways/%s", gatewayModel.ID)))
	Expect(*gatewayOutput.CreatedAt).To(BeTemporally("~", gatewayModel.CreatedAt))
	Expect(*gatewayOutput.UpdatedAt).To(BeTemporally("~", gatewayModel.UpdatedAt))
	Expect(gatewayOutput.GetGatewayVersion()).To(Equal("0.0.109"))
}

// Verifies: PLAT-DATA-02.02
func TestGatewayPost(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	clusterID := registerTestCluster(t)
	// A user create carries only the user-writable allowlist: name + placement/cluster_id
	// (PLAT-DATA-23). Platform-managed fields are not part of the create contract.
	gatewayInput := openapi.GatewayCreateRequest{Name: "test-name", ClusterId: openapi.PtrString(clusterID)}

	gatewayOutput, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*gatewayOutput.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(*gatewayOutput.Kind).To(Equal("Gateway"))
	Expect(*gatewayOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateways/%s", *gatewayOutput.Id)))
	Expect(gatewayOutput.ClusterId).To(Equal(clusterID))
	Expect(gatewayOutput.Namespace).To(MatchRegexp(`^openshell-[0-9a-f]{16}$`))
	// The platform-managed fields are control-plane-only and unset on a bare create.
	Expect(gatewayOutput.GetImage()).To(BeEmpty())
	Expect(gatewayOutput.GetExternalDns()).To(BeEmpty())

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Post(h.RestURL("/gateways"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

// Verifies: PLAT-DATA-02.02
func TestGatewayPostMinimalFields(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	clusterID := registerTestCluster(t)
	gatewayInput := managedGatewayCreateRequest("local-gateway")

	gatewayOutput, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting gateway with minimal fields: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(gatewayOutput.ClusterId).To(Equal(clusterID))
	Expect(gatewayOutput.Namespace).To(MatchRegexp(`^openshell-[0-9a-f]{16}$`))
}

// Gateway creation takes exactly one of cluster_id (pinned) or placement
// (late-bound). Supplying BOTH is rejected (data-model.spec.md, "Gateway Placement
// and Late Binding"). The pinned-only and placement-only shapes are accepted; see
// TestGatewayPostAcceptsPinnedClusterID and TestGatewayPostLateBindsWhenNoClusterMatches.
func TestGatewayPostRejectsDirectClusterID(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	registerTestCluster(t)
	jwToken := ctx.Value(openapi.ContextAccessToken)
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwToken)).
		SetBody(`{"name":"direct-cluster","placement":{"selector":["network/public","provider/aws"]},"cluster_id":"legacy-cluster"}`).
		Post(h.RestURL("/gateways"))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest))
	Expect(string(resp.Body())).To(ContainSubstring("cluster_id"))
	expectNoGatewayNamed(ctx, client, "direct-cluster")
}

// The REST PATCH contract exposes only name (PLAT-DATA-23): cluster_id and every
// platform-managed field are no longer user-writable on update. A rename is accepted and
// leaves the control-plane-managed fields untouched.
func TestGatewayPatchRenameOnly(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	registerTestCluster(t)
	created, _, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(managedGatewayCreateRequest("patch-rename")).Execute()
	Expect(err).NotTo(HaveOccurred())

	patched, resp, err := client.DefaultAPI.UpdateGateway(ctx, *created.Id).GatewayPatchRequest(openapi.GatewayPatchRequest{
		Name: openapi.PtrString("renamed-gateway"),
	}).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(patched.Name).To(Equal("renamed-gateway"))
	Expect(patched.ClusterId).To(Equal(created.ClusterId))
}

// A user PATCH that carries a control-plane-only field (for example phase) is rejected
// at the REST boundary with 400 naming the field, rather than being silently dropped and
// returning 200. GatewayPatchRequest has no required property, so the OpenAPI generator
// emits no strict decoder for it; the plugin's strict PATCH decode supplies the
// rejection, matching the strict create contract (PLAT-DATA-23, SEC-RBAC-18).
func TestGatewayPatchRejectsLockedField(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	registerTestCluster(t)
	created, _, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(managedGatewayCreateRequest("patch-locked")).Execute()
	Expect(err).NotTo(HaveOccurred())

	jwToken := ctx.Value(openapi.ContextAccessToken)
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwToken)).
		SetBody(`{"name":"patch-locked","phase":"Running"}`).
		Patch(h.RestURL("/gateways/" + *created.Id))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest))
	Expect(string(resp.Body())).To(ContainSubstring("phase"))

	// The stored phase is unchanged by the rejected write.
	after, _, err := client.DefaultAPI.GetGateway(ctx, *created.Id).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(after.GetPhase()).To(Equal(created.GetPhase()))
}

func expectNoGatewayNamed(ctx context.Context, client *openapi.APIClient, name string) {
	list, _, err := client.DefaultAPI.ListGateways(ctx).Search(fmt.Sprintf("name = '%s'", name)).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(list.Items).To(BeEmpty(), "no gateway named %s may be created", name)
}

func TestGatewayPostWithoutRouteRemainsUnrouted(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	registerTestCluster(t)
	gatewayInput := managedGatewayCreateRequest("route-default-test")

	gatewayOutput, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(gatewayOutput.GetRoute()).To(BeEmpty())
}

func TestGatewayPatch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayModel, err := newGateway(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	gatewayOutput, resp, err := client.DefaultAPI.UpdateGateway(ctx, gatewayModel.ID).GatewayPatchRequest(openapi.GatewayPatchRequest{}).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(*gatewayOutput.Id).To(Equal(gatewayModel.ID))
	Expect(*gatewayOutput.CreatedAt).To(BeTemporally("~", gatewayModel.CreatedAt))
	Expect(*gatewayOutput.Kind).To(Equal("Gateway"))
	Expect(*gatewayOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateways/%s", *gatewayOutput.Id)))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Patch(h.RestURL("/gateways/foo"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestGatewayDelete(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayModel, err := newGateway(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	resp, err := client.DefaultAPI.DeleteGateway(ctx, gatewayModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	_, resp, err = client.DefaultAPI.GetGateway(ctx, gatewayModel.ID).Execute()
	Expect(err).To(HaveOccurred(), "Expected deleted gateway to return 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
}

func TestGatewayPaging(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, err := newGatewayList("Bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	list, _, err := client.DefaultAPI.ListGateways(ctx).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting gateway list: %v", err)
	Expect(len(list.Items)).To(Equal(20))
	Expect(list.GetSize()).To(Equal(int32(20)))
	Expect(list.GetTotal()).To(Equal(int32(20)))
	Expect(list.GetPage()).To(Equal(int32(1)))

	list, _, err = client.DefaultAPI.ListGateways(ctx).Page(2).Size(5).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting gateway list: %v", err)
	Expect(len(list.Items)).To(Equal(5))
	Expect(list.GetSize()).To(Equal(int32(5)))
	Expect(list.GetTotal()).To(Equal(int32(20)))
	Expect(list.GetPage()).To(Equal(int32(2)))
}

func TestGatewayPagingSortedByCreator(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gateways, err := newGatewayList("creator-sort", 2)
	Expect(err).NotTo(HaveOccurred())

	envServices := &environments.Environment().Services
	userService := users.Service(envServices)
	bindingService := roleBindings.Service(envServices)
	creatorNames := []string{"zulu-creator", "alpha-creator"}
	for i, creatorName := range creatorNames {
		userID, createErr := userService.UpsertByUsername(context.Background(), creatorName, nil, nil)
		Expect(createErr).NotTo(HaveOccurred())
		Expect(bindingService.CreateGatewayOwnerBinding(context.Background(), userID, gateways[i].ID)).To(Succeed())
	}

	search := fmt.Sprintf("id in ('%s', '%s')", gateways[0].ID, gateways[1].ID)
	list, _, err := client.DefaultAPI.ListGateways(ctx).
		Search(search).
		OrderBy("created_by asc").
		Execute()
	Expect(err).NotTo(HaveOccurred(), "Error sorting gateway list by creator: %v", err)
	Expect(list.Items).To(HaveLen(2))
	Expect(list.Items[0].GetCreatedBy()).To(Equal("alpha-creator"))
	Expect(list.Items[1].GetCreatedBy()).To(Equal("zulu-creator"))
}

func TestGatewayListSearch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gateways, err := newGatewayList("bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	search := fmt.Sprintf("id in ('%s')", gateways[0].ID)
	list, _, err := client.DefaultAPI.ListGateways(ctx).Search(search).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting gateway list: %v", err)
	Expect(len(list.Items)).To(Equal(1))
	Expect(list.GetTotal()).To(Equal(int32(1)))
	Expect(*list.Items[0].Id).To(Equal(gateways[0].ID))
}
