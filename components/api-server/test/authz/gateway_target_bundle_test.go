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

// patchGateway sends a raw PATCH body as p and returns the status and body.
func patchGateway(h *test.Helper, p principal, id, body string) (int, string) {
	req, err := http.NewRequest(http.MethodPatch, h.RestURL("/gateways/"+id), strings.NewReader(body))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.token))
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func getGateway(client *openapi.APIClient, p principal, id string) *openapi.Gateway {
	gw, resp, err := client.DefaultAPI.GetGateway(p.rest, id).Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusOK), "get: %s", test.APIErrorReason(err))
	return gw
}

// Verifies: SEC-RBAC-36.01, SEC-RBAC-36.02, SEC-RBAC-36.03, SEC-RBAC-07.05, PLAT-DATA-33.01, PLAT-DATA-33.05, PLAT-DATA-33.07
func TestRESTGatewayTargetBundleIsPlatformAdminOnly(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	f := newReadFixture(t, h, client)
	outsider := newPrincipal(t, h, "outsider-"+strings.ToLower(api.NewID()), realmRoles("gateway:creator"))
	id := f.gw1.GetId()
	before := getGateway(client, f.owner, id)
	Expect(before.TargetBundle).To(BeNil())

	// Owner, viewer, and a caller with no binding may not set the target, and
	// none of their attempts advances generation.
	for _, p := range []principal{f.owner, f.viewer, outsider} {
		code, body := patchGateway(h, p, id, `{"target_bundle":"b2"}`)
		Expect(code).To(Equal(http.StatusForbidden), "%s must not set target_bundle: %s", p.name, body)
	}
	Expect(getGateway(client, f.owner, id).GetGeneration()).To(Equal(before.GetGeneration()))

	// platform:admin without a binding may not rename the gateway.
	code, body := patchGateway(h, f.admin, id, `{"name":"admin-rename"}`)
	Expect(code).To(Equal(http.StatusForbidden), "admin rename: %s", body)

	// platform:admin sets the target: generation advances, observed does not.
	code, body = patchGateway(h, f.admin, id, `{"target_bundle":"b2#1"}`)
	Expect(code).To(Equal(http.StatusOK), "admin set target: %s", body)
	after := getGateway(client, f.owner, id)
	Expect(after.GetTargetBundle()).To(Equal("b2#1"))
	Expect(after.GetGeneration()).To(Equal(before.GetGeneration() + 1))
	Expect(after.GetObservedGeneration()).To(Equal(before.GetObservedGeneration()))

	// Resending the same value is a no-op; a new token advances generation again.
	code, _ = patchGateway(h, f.admin, id, `{"target_bundle":"b2#1"}`)
	Expect(code).To(Equal(http.StatusOK))
	Expect(getGateway(client, f.owner, id).GetGeneration()).To(Equal(after.GetGeneration()))
	code, _ = patchGateway(h, f.admin, id, `{"target_bundle":"b2#2"}`)
	Expect(code).To(Equal(http.StatusOK))
	Expect(getGateway(client, f.owner, id).GetGeneration()).To(Equal(after.GetGeneration() + 1))

	// The owner can still rename, and a rename keeps the admin's target.
	code, body = patchGateway(h, f.owner, id, `{"name":"owner-rename"}`)
	Expect(code).To(Equal(http.StatusOK), "owner rename: %s", body)
	Expect(getGateway(client, f.owner, id).GetTargetBundle()).To(Equal("b2#2"))
}
