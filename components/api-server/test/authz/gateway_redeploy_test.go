package authz_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
)

// redeploy POSTs the administrator redeploy action as p and returns the
// response status and, on success, the returned gateway.
func redeploy(client *openapi.APIClient, p principal, id string) (int, *openapi.Gateway) {
	gw, resp, _ := client.DefaultAPI.RedeployGateway(p.rest, id).Execute()
	return statusOf(resp), gw
}

// Verifies: SEC-RBAC-36.01, SEC-RBAC-36.02, PLAT-DATA-33.01, PLAT-DATA-33.02
func TestRESTGatewayRedeployIsPlatformAdminOnly(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	f := newReadFixture(t, h, client)
	outsider := newPrincipal(t, h, "outsider-"+strings.ToLower(api.NewID()), realmRoles("gateway:creator"))
	id := f.gw1.GetId()

	before, resp, err := client.DefaultAPI.GetGateway(f.owner.rest, id).Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusOK), "get: %s", test.APIErrorReason(err))
	Expect(before.RedeployRequestedAt).To(BeNil())

	// Owner, viewer, and a caller with no binding on the gateway are all denied,
	// and none of their attempts advances generation.
	for _, p := range []principal{f.owner, f.viewer, outsider} {
		code, _ := redeploy(client, p, id)
		Expect(code).To(Equal(http.StatusForbidden), "%s must not redeploy", p.name)
	}
	unchanged, _, err := client.DefaultAPI.GetGateway(f.owner.rest, id).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(unchanged.GetGeneration()).To(Equal(before.GetGeneration()))

	// platform:admin redeploys: generation advances, observed_generation does not,
	// and redeploy_requested_at is recorded.
	code, after := redeploy(client, f.admin, id)
	Expect(code).To(Equal(http.StatusOK))
	Expect(after.GetGeneration()).To(Equal(before.GetGeneration() + 1))
	Expect(after.GetObservedGeneration()).To(Equal(before.GetObservedGeneration()))
	Expect(after.RedeployRequestedAt).NotTo(BeNil())

	// A second request before convergence coalesces into one more generation.
	code, again := redeploy(client, f.admin, id)
	Expect(code).To(Equal(http.StatusOK))
	Expect(again.GetGeneration()).To(Equal(before.GetGeneration() + 2))
	Expect(again.GetRedeployRequestedAt()).To(BeTemporally(">=", after.GetRedeployRequestedAt()))
}

// Verifies: PLAT-DATA-33.04
func TestRESTGatewayPatchCannotSetRedeployRequestedAt(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	f := newReadFixture(t, h, client)
	id := f.gw1.GetId()

	req, err := http.NewRequest(http.MethodPatch, h.RestURL("/gateways/"+id),
		strings.NewReader(`{"redeploy_requested_at":"2030-01-01T00:00:00Z"}`))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", f.owner.token))
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	Expect(string(body)).To(ContainSubstring("redeploy_requested_at"))

	after, _, err := client.DefaultAPI.GetGateway(f.owner.rest, id).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(after.RedeployRequestedAt).To(BeNil())
}

// Verifies: PLAT-DATA-33.03
func TestRESTGatewayRedeployOfDeletedGatewayIsNotFound(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	f := newReadFixture(t, h, client)
	id := f.gw2.GetId()

	resp, err := client.DefaultAPI.DeleteGateway(f.owner.rest, id).Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusNoContent), "delete: %s", test.APIErrorReason(err))

	code, _ := redeploy(client, f.admin, id)
	Expect(code).To(Equal(http.StatusNotFound))
}
