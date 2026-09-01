package gateways_test

import (
	"context"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// createGatewayProfile creates a GatewayProfile through the OpenAPI client and
// returns its ID for use in gateway profile-assignment tests.
func createGatewayProfile(ctx context.Context, client *openapi.APIClient, name string) string {
	profile, resp, err := client.DefaultAPI.CreateGatewayProfile(ctx).
		GatewayProfile(openapi.GatewayProfile{Name: name, CpuRequestTotal: openapi.PtrString("1")}).
		Execute()
	Expect(err).NotTo(HaveOccurred(), "Error creating gateway profile: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	return *profile.Id
}

// TestGatewayPostAssignsProfile covers profile assignment on gateway creation:
// a client-supplied, existing profile_id is persisted on the gateway.
func TestGatewayPostAssignsProfile(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	profileID := createGatewayProfile(ctx, client, "assign-on-create")

	gatewayInput := openapi.GatewayCreateRequest{
		Name:      "gw-with-profile",
		ProfileId: openapi.PtrString(profileID),
	}
	gateway, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error creating gateway with profile: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(gateway.GetProfileId()).To(Equal(profileID))
}

// TestGatewayPostRejectsNonexistentProfile covers create-time profile
// validation: a profile_id that does not exist is rejected with HTTP 400.
func TestGatewayPostRejectsNonexistentProfile(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	gatewayInput := openapi.GatewayCreateRequest{
		Name:      "gw-bad-profile",
		ProfileId: openapi.PtrString("does-not-exist"),
	}
	_, resp, err := client.DefaultAPI.CreateGateway(ctx).GatewayCreateRequest(gatewayInput).Execute()
	Expect(err).To(HaveOccurred(), "Expected create with nonexistent profile to be rejected")
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
}

// TestGatewayPatchReassignsProfile covers profile reassignment via PATCH:
// reassigning to another existing profile succeeds; clearing the profile is
// rejected; and reassigning to a nonexistent profile is rejected.
func TestGatewayPatchReassignsProfile(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	profileA := createGatewayProfile(ctx, client, "reassign-a")
	profileB := createGatewayProfile(ctx, client, "reassign-b")

	created, resp, err := client.DefaultAPI.CreateGateway(ctx).
		GatewayCreateRequest(openapi.GatewayCreateRequest{Name: "gw-reassign", ProfileId: openapi.PtrString(profileA)}).
		Execute()
	Expect(err).NotTo(HaveOccurred(), "Error creating gateway: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	// Reassign to another existing profile succeeds.
	patched, resp, err := client.DefaultAPI.UpdateGateway(ctx, *created.Id).
		GatewayPatchRequest(openapi.GatewayPatchRequest{ProfileId: openapi.PtrString(profileB)}).
		Execute()
	Expect(err).NotTo(HaveOccurred(), "Error reassigning gateway profile: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(patched.GetProfileId()).To(Equal(profileB))

	// Clearing the profile is forbidden.
	_, resp, err = client.DefaultAPI.UpdateGateway(ctx, *created.Id).
		GatewayPatchRequest(openapi.GatewayPatchRequest{ProfileId: openapi.PtrString("")}).
		Execute()
	Expect(err).To(HaveOccurred(), "Expected clearing profile_id to be rejected")
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

	// Reassigning to a nonexistent profile is rejected.
	_, resp, err = client.DefaultAPI.UpdateGateway(ctx, *created.Id).
		GatewayPatchRequest(openapi.GatewayPatchRequest{ProfileId: openapi.PtrString("does-not-exist")}).
		Execute()
	Expect(err).To(HaveOccurred(), "Expected reassignment to a nonexistent profile to be rejected")
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
}
