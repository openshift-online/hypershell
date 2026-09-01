package gatewayProfiles_test

import (
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// TestGatewayProfileCRUD covers the create/get/list/patch/delete happy path
// through the generated OpenAPI client against a live API server.
func TestGatewayProfileCRUD(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	input := openapi.GatewayProfile{
		Name:             "small",
		Description:      openapi.PtrString("small workloads"),
		CpuRequestTotal:  openapi.PtrString("2"),
		MemoryLimitTotal: openapi.PtrString("8Gi"),
		PodCount:         openapi.PtrInt32(10),
		PvcCount:         openapi.PtrInt32(2),
	}

	created, resp, err := client.DefaultAPI.CreateGatewayProfile(ctx).GatewayProfile(input).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error creating gateway profile: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*created.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(*created.Kind).To(Equal("GatewayProfile"))
	Expect(*created.Href).To(Equal(fmt.Sprintf("/api/hypershell/v1/gateway_profiles/%s", *created.Id)))
	Expect(created.Name).To(Equal("small"))

	got, resp, err := client.DefaultAPI.GetGatewayProfile(ctx, *created.Id).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(*got.Id).To(Equal(*created.Id))
	Expect(got.GetMemoryLimitTotal()).To(Equal("8Gi"))

	list, _, err := client.DefaultAPI.ListGatewayProfiles(ctx).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(list.GetTotal()).To(BeNumerically(">=", int32(1)))

	patched, resp, err := client.DefaultAPI.UpdateGatewayProfile(ctx, *created.Id).
		GatewayProfilePatchRequest(openapi.GatewayProfilePatchRequest{Description: openapi.PtrString("updated")}).
		Execute()
	Expect(err).NotTo(HaveOccurred(), "Error patching gateway profile: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(patched.GetDescription()).To(Equal("updated"))

	resp, err = client.DefaultAPI.DeleteGatewayProfile(ctx, *created.Id).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	_, resp, err = client.DefaultAPI.GetGatewayProfile(ctx, *created.Id).Execute()
	Expect(err).To(HaveOccurred(), "Expected deleted gateway profile to return 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
}

// TestGatewayProfileValidationRejected covers boundary validation: an invalid
// Kubernetes resource quantity must be rejected with HTTP 400 before storage.
func TestGatewayProfileValidationRejected(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	input := openapi.GatewayProfile{
		Name:             "invalid",
		MemoryLimitTotal: openapi.PtrString("8GB"), // "GB" is not a valid quantity suffix
	}

	_, resp, err := client.DefaultAPI.CreateGatewayProfile(ctx).GatewayProfile(input).Execute()
	Expect(err).To(HaveOccurred(), "Expected invalid quantity to be rejected")
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
}

// TestGatewayProfileMissingNameRejected covers required-field validation: a
// create without a name must be rejected with HTTP 400, matching the gRPC
// CreateGatewayProfile contract which requires name.
func TestGatewayProfileMissingNameRejected(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	input := openapi.GatewayProfile{
		CpuRequestTotal: openapi.PtrString("1"),
	}

	_, resp, err := client.DefaultAPI.CreateGatewayProfile(ctx).GatewayProfile(input).Execute()
	Expect(err).To(HaveOccurred(), "Expected missing name to be rejected")
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
}

// TestGatewayProfileDeletionProtectedByGateway covers deletion protection: a
// profile referenced by a live gateway must not be deleted (HTTP 409).
func TestGatewayProfileDeletionProtectedByGateway(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	profile, resp, err := client.DefaultAPI.CreateGatewayProfile(ctx).
		GatewayProfile(openapi.GatewayProfile{Name: "referenced", CpuRequestTotal: openapi.PtrString("1")}).
		Execute()
	Expect(err).NotTo(HaveOccurred(), "Error creating gateway profile: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	// A gateway referencing the profile makes it undeletable. Placement assigns
	// database_id server-side, so empty placement IDs are acceptable here.
	gatewayInput := openapi.GatewayCreateRequest{
		Name:      "profile-ref-gw",
		ProfileId: openapi.PtrString(*profile.Id),
	}
	gateway, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error creating gateway referencing the profile: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(gateway.GetProfileId()).To(Equal(*profile.Id))

	resp, err = client.DefaultAPI.DeleteGatewayProfile(ctx, *profile.Id).Execute()
	Expect(err).To(HaveOccurred(), "Expected referenced profile deletion to be rejected")
	Expect(resp.StatusCode).To(Equal(http.StatusConflict))

	// After removing the referencing gateway the profile becomes deletable.
	resp, err = client.DefaultAPI.DeleteGateway(ctx, *gateway.Id).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

	resp, err = client.DefaultAPI.DeleteGatewayProfile(ctx, *profile.Id).Execute()
	Expect(err).NotTo(HaveOccurred(), "Expected unreferenced profile to be deletable")
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
}
