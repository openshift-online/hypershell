package gatewayNetworks_test

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

func TestGatewayNetworkGet(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, _, err := client.DefaultAPI.ApiHypershellV1GatewayNetworksIdGet(context.Background(), "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 401 but got nil error")

	_, resp, err := client.DefaultAPI.ApiHypershellV1GatewayNetworksIdGet(ctx, "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

	gatewayNetworkModel, err := newGatewayNetwork(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	gatewayNetworkOutput, resp, err := client.DefaultAPI.ApiHypershellV1GatewayNetworksIdGet(ctx, gatewayNetworkModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	Expect(*gatewayNetworkOutput.Id).To(Equal(gatewayNetworkModel.ID), "found object does not match test object")
	Expect(*gatewayNetworkOutput.Kind).To(Equal("GatewayNetwork"))
	Expect(*gatewayNetworkOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateway_networks/%s", gatewayNetworkModel.ID)))
	Expect(*gatewayNetworkOutput.CreatedAt).To(BeTemporally("~", gatewayNetworkModel.CreatedAt))
	Expect(*gatewayNetworkOutput.UpdatedAt).To(BeTemporally("~", gatewayNetworkModel.UpdatedAt))
}

func TestGatewayNetworkPost(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayNetworkInput := openapi.GatewayNetwork{
		Name:         "test-name",
		FleetId:      "test-fleet_id",
		Topology:     openapi.PtrString("test-topology"),
		TunnelMode:   openapi.PtrString("test-tunnel_mode"),
		HubGatewayId: openapi.PtrString("test-hub_gateway_id"),
		Status:       openapi.PtrString("test-status"),
	}

	gatewayNetworkOutput, resp, err := client.DefaultAPI.ApiHypershellV1GatewayNetworksPost(ctx).GatewayNetwork(gatewayNetworkInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*gatewayNetworkOutput.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(*gatewayNetworkOutput.Kind).To(Equal("GatewayNetwork"))
	Expect(*gatewayNetworkOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateway_networks/%s", *gatewayNetworkOutput.Id)))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Post(h.RestURL("/gateway_networks"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestGatewayNetworkPatch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayNetworkModel, err := newGatewayNetwork(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	gatewayNetworkOutput, resp, err := client.DefaultAPI.ApiHypershellV1GatewayNetworksIdPatch(ctx, gatewayNetworkModel.ID).GatewayNetworkPatchRequest(openapi.GatewayNetworkPatchRequest{}).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(*gatewayNetworkOutput.Id).To(Equal(gatewayNetworkModel.ID))
	Expect(*gatewayNetworkOutput.CreatedAt).To(BeTemporally("~", gatewayNetworkModel.CreatedAt))
	Expect(*gatewayNetworkOutput.Kind).To(Equal("GatewayNetwork"))
	Expect(*gatewayNetworkOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateway_networks/%s", *gatewayNetworkOutput.Id)))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Patch(h.RestURL("/gateway_networks/foo"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestGatewayNetworkPaging(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, err := newGatewayNetworkList("Bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	list, _, err := client.DefaultAPI.ApiHypershellV1GatewayNetworksGet(ctx).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting gatewayNetwork list: %v", err)
	Expect(len(list.Items)).To(Equal(20))
	Expect(list.Size).To(Equal(int32(20)))
	Expect(list.Total).To(Equal(int32(20)))
	Expect(list.Page).To(Equal(int32(1)))

	list, _, err = client.DefaultAPI.ApiHypershellV1GatewayNetworksGet(ctx).Page(2).Size(5).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting gatewayNetwork list: %v", err)
	Expect(len(list.Items)).To(Equal(5))
	Expect(list.Size).To(Equal(int32(5)))
	Expect(list.Total).To(Equal(int32(20)))
	Expect(list.Page).To(Equal(int32(2)))
}

func TestGatewayNetworkListSearch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayNetworks, err := newGatewayNetworkList("bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	search := fmt.Sprintf("id in ('%s')", gatewayNetworks[0].ID)
	list, _, err := client.DefaultAPI.ApiHypershellV1GatewayNetworksGet(ctx).Search(search).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting gatewayNetwork list: %v", err)
	Expect(len(list.Items)).To(Equal(1))
	Expect(list.Total).To(Equal(int32(1)))
	Expect(*list.Items[0].Id).To(Equal(gatewayNetworks[0].ID))
}
