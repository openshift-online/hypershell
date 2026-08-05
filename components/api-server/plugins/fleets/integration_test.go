package fleets_test

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

func TestFleetGet(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, _, err := client.DefaultAPI.GetFleet(context.Background(), "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 401 but got nil error")

	_, resp, err := client.DefaultAPI.GetFleet(ctx, "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

	fleetModel, err := newFleet(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	fleetOutput, resp, err := client.DefaultAPI.GetFleet(ctx, fleetModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	Expect(*fleetOutput.Id).To(Equal(fleetModel.ID), "found object does not match test object")
	Expect(*fleetOutput.Kind).To(Equal("Fleet"))
	Expect(*fleetOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/fleets/%s", fleetModel.ID)))
	Expect(*fleetOutput.CreatedAt).To(BeTemporally("~", fleetModel.CreatedAt))
	Expect(*fleetOutput.UpdatedAt).To(BeTemporally("~", fleetModel.UpdatedAt))
}

func TestFleetPost(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	fleetInput := openapi.Fleet{
		Name:        "test-name",
		Description: openapi.PtrString("test-description"),
		Status:      openapi.PtrString("test-status"),
	}

	fleetOutput, resp, err := client.DefaultAPI.CreateFleet(ctx).Fleet(fleetInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*fleetOutput.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(*fleetOutput.Kind).To(Equal("Fleet"))
	Expect(*fleetOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/fleets/%s", *fleetOutput.Id)))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Post(h.RestURL("/fleets"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestFleetPatch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	fleetModel, err := newFleet(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	fleetOutput, resp, err := client.DefaultAPI.UpdateFleet(ctx, fleetModel.ID).FleetPatchRequest(openapi.FleetPatchRequest{}).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(*fleetOutput.Id).To(Equal(fleetModel.ID))
	Expect(*fleetOutput.CreatedAt).To(BeTemporally("~", fleetModel.CreatedAt))
	Expect(*fleetOutput.Kind).To(Equal("Fleet"))
	Expect(*fleetOutput.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/fleets/%s", *fleetOutput.Id)))

	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Patch(h.RestURL("/fleets/foo"))

	Expect(err).NotTo(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestFleetDelete(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	fleetModel, err := newFleet(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	resp, err := client.DefaultAPI.DeleteFleet(ctx, fleetModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	_, resp, err = client.DefaultAPI.GetFleet(ctx, fleetModel.ID).Execute()
	Expect(err).To(HaveOccurred(), "Expected deleted fleet to return 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
}

func TestFleetPaging(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	_, err := newFleetList("Bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	list, _, err := client.DefaultAPI.ListFleets(ctx).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting fleet list: %v", err)
	Expect(len(list.Items)).To(Equal(20))
	Expect(list.GetSize()).To(Equal(int32(20)))
	Expect(list.GetTotal()).To(Equal(int32(20)))
	Expect(list.GetPage()).To(Equal(int32(1)))

	list, _, err = client.DefaultAPI.ListFleets(ctx).Page(2).Size(5).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting fleet list: %v", err)
	Expect(len(list.Items)).To(Equal(5))
	Expect(list.GetSize()).To(Equal(int32(5)))
	Expect(list.GetTotal()).To(Equal(int32(20)))
	Expect(list.GetPage()).To(Equal(int32(2)))
}

func TestFleetListSearch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	fleets, err := newFleetList("bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	search := fmt.Sprintf("id in ('%s')", fleets[0].ID)
	list, _, err := client.DefaultAPI.ListFleets(ctx).Search(search).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting fleet list: %v", err)
	Expect(len(list.Items)).To(Equal(1))
	Expect(list.GetTotal()).To(Equal(int32(1)))
	Expect(*list.Items[0].Id).To(Equal(fleets[0].ID))
}
