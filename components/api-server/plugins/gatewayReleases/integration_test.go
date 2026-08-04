package gatewayReleases_test

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

func TestGatewayReleaseGet(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, _, err := client.DefaultAPI.ApiHypershellV1GatewayReleasesIdGet(context.Background(), "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 401 but got nil error")

	_, resp, err := client.DefaultAPI.ApiHypershellV1GatewayReleasesIdGet(ctx, "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

	gatewayReleaseModel, err := newGatewayRelease(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	gatewayReleaseOutput, resp, err := client.DefaultAPI.ApiHypershellV1GatewayReleasesIdGet(ctx, gatewayReleaseModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	Expect(*gatewayReleaseOutput.Id).To(Equal(gatewayReleaseModel.ID), "found object does not match test object")
	Expect(*gatewayReleaseOutput.Kind).To(Equal("GatewayRelease"))
	Expect(*gatewayReleaseOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateway_releases/%s", gatewayReleaseModel.ID)))
	Expect(*gatewayReleaseOutput.CreatedAt).To(BeTemporally("~", gatewayReleaseModel.CreatedAt))
	Expect(*gatewayReleaseOutput.UpdatedAt).To(BeTemporally("~", gatewayReleaseModel.UpdatedAt))
}

func TestGatewayReleasePost(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayReleaseInput := openapi.GatewayRelease{
		Name:            "test-name",
		FleetId:         "test-fleet_id",
		Image:           "test-image",
		RolloutStrategy: openapi.PtrString("test-rollout_strategy"),
		CanaryPercent:   openapi.PtrInt32(42),
		CanaryDuration:  openapi.PtrString("test-canary_duration"),
		Status:          openapi.PtrString("test-status"),
	}

	gatewayReleaseOutput, resp, err := client.DefaultAPI.ApiHypershellV1GatewayReleasesPost(ctx).GatewayRelease(gatewayReleaseInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*gatewayReleaseOutput.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(*gatewayReleaseOutput.Kind).To(Equal("GatewayRelease"))
	Expect(*gatewayReleaseOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateway_releases/%s", *gatewayReleaseOutput.Id)))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Post(h.RestURL("/gateway_releases"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestGatewayReleasePatch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayReleaseModel, err := newGatewayRelease(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	gatewayReleaseOutput, resp, err := client.DefaultAPI.ApiHypershellV1GatewayReleasesIdPatch(ctx, gatewayReleaseModel.ID).GatewayReleasePatchRequest(openapi.GatewayReleasePatchRequest{}).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(*gatewayReleaseOutput.Id).To(Equal(gatewayReleaseModel.ID))
	Expect(*gatewayReleaseOutput.CreatedAt).To(BeTemporally("~", gatewayReleaseModel.CreatedAt))
	Expect(*gatewayReleaseOutput.Kind).To(Equal("GatewayRelease"))
	Expect(*gatewayReleaseOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateway_releases/%s", *gatewayReleaseOutput.Id)))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Patch(h.RestURL("/gateway_releases/foo"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestGatewayReleasePaging(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, err := newGatewayReleaseList("Bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	list, _, err := client.DefaultAPI.ApiHypershellV1GatewayReleasesGet(ctx).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting gatewayRelease list: %v", err)
	Expect(len(list.Items)).To(Equal(20))
	Expect(list.Size).To(Equal(int32(20)))
	Expect(list.Total).To(Equal(int32(20)))
	Expect(list.Page).To(Equal(int32(1)))

	list, _, err = client.DefaultAPI.ApiHypershellV1GatewayReleasesGet(ctx).Page(2).Size(5).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting gatewayRelease list: %v", err)
	Expect(len(list.Items)).To(Equal(5))
	Expect(list.Size).To(Equal(int32(5)))
	Expect(list.Total).To(Equal(int32(20)))
	Expect(list.Page).To(Equal(int32(2)))
}

func TestGatewayReleaseListSearch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayReleases, err := newGatewayReleaseList("bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	search := fmt.Sprintf("id in ('%s')", gatewayReleases[0].ID)
	list, _, err := client.DefaultAPI.ApiHypershellV1GatewayReleasesGet(ctx).Search(search).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting gatewayRelease list: %v", err)
	Expect(len(list.Items)).To(Equal(1))
	Expect(list.Total).To(Equal(int32(1)))
	Expect(*list.Items[0].Id).To(Equal(gatewayReleases[0].ID))
}
