package gateways_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"gopkg.in/resty.v1"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
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
}

func TestGatewayPost(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayInput := openapi.GatewayCreateRequest{
		Name:        "test-name",
		FleetId:     "test-fleet_id",
		ClusterId:   "test-cluster_id",
		ReleaseId:   "test-release_id",
		DatabaseId:  "test-database_id",
		ExternalDns: openapi.PtrString("test-external_dns"),
		TlsMode:     openapi.PtrString("test-tls_mode"),
		ServiceType: openapi.PtrString("test-service_type"),
		Status:      openapi.PtrString("test-status"),
		Phase:       openapi.PtrString("test-phase"),
	}

	gatewayOutput, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*gatewayOutput.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(*gatewayOutput.Kind).To(Equal("Gateway"))
	Expect(*gatewayOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateways/%s", *gatewayOutput.Id)))
	Expect(gatewayOutput.Namespace).To(MatchRegexp(`^openshell-[0-9a-f]{40}$`))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Post(h.RestURL("/gateways"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestGatewayPostAllowsEmptyReconcilerOwnedIDs(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	gatewayInput := openapi.GatewayCreateRequest{
		Name:       "local-gateway",
		FleetId:    "",
		ClusterId:  "",
		ReleaseId:  "",
		DatabaseId: "",
	}

	gatewayOutput, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting gateway with local placement: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(gatewayOutput.FleetId).To(BeEmpty())
	Expect(gatewayOutput.ClusterId).To(BeEmpty())
	Expect(gatewayOutput.ReleaseId).To(BeEmpty())
	Expect(gatewayOutput.DatabaseId).To(BeEmpty())
	Expect(gatewayOutput.Namespace).To(MatchRegexp(`^openshell-[0-9a-f]{40}$`))
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
