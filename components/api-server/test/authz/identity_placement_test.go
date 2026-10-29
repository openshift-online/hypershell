package authz_test

import (
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
)

// A platform:admin caller may place a gateway by a cluster's server-assigned
// identity capability, which resolves deterministically to exactly that cluster.
// Verifies: PLAT-CLUS-14.03, PLAT-DATA-02
func TestIdentitySelectorResolvesForPlatformAdmin(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	target := registerSpoke(t, h, client)
	admin := newPrincipal(t, h, "admin-"+strings.ToLower(api.NewID()), realmRoles("platform:admin"))

	input := openapi.GatewayCreateRequest{Name: "identity-" + strings.ToLower(api.NewID())}
	input.SetPlacement(openapi.GatewayPlacementIntent{Selector: []string{managedClusters.IdentityCapability(target.name)}})

	out, httpResp, err := client.DefaultAPI.CreateGateway(admin.rest).GatewayCreateRequest(input).Execute()
	Expect(err).NotTo(HaveOccurred(), "admin identity placement: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(statusOf(httpResp)).To(Equal(http.StatusCreated))
	Expect(out.ClusterId).To(Equal(target.clusterID), "identity selector must resolve to exactly the named cluster")
}

// A non-admin caller's identity selector is rejected (403) and the gateway is not
// placed: gating the selector's use is the authoritative control (PLAT-DATA-02).
// Verifies: PLAT-CLUS-14.04, PLAT-DATA-02
func TestIdentitySelectorRejectedForNonAdmin(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	target := registerSpoke(t, h, client)
	// An ordinary signed-in principal holds gateway:creator (default role) but not
	// platform:admin.
	user := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), nil)

	input := openapi.GatewayCreateRequest{Name: "identity-deny-" + strings.ToLower(api.NewID())}
	input.SetPlacement(openapi.GatewayPlacementIntent{Selector: []string{managedClusters.IdentityCapability(target.name)}})

	_, httpResp, err := client.DefaultAPI.CreateGateway(user.rest).GatewayCreateRequest(input).Execute()
	Expect(err).To(HaveOccurred(), "non-admin identity placement unexpectedly succeeded")
	Expect(statusOf(httpResp)).To(Equal(http.StatusForbidden))
}

// A requirement-only selector keeps resolving by random selection for an ordinary
// caller, unaffected by identity gating (PLAT-DATA-02).
func TestRequirementSelectorUnaffectedForNonAdmin(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	registerSpoke(t, h, client)
	user := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), nil)

	input := openapi.GatewayCreateRequest{Name: "req-" + strings.ToLower(api.NewID())}
	input.SetPlacement(openapi.GatewayPlacementIntent{Selector: []string{"network/public", "provider/aws"}})

	out, httpResp, err := client.DefaultAPI.CreateGateway(user.rest).GatewayCreateRequest(input).Execute()
	Expect(err).NotTo(HaveOccurred(), "requirement placement: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(statusOf(httpResp)).To(Equal(http.StatusCreated))
	Expect(out.ClusterId).NotTo(BeEmpty(), "a requirement selector must resolve to an eligible cluster")
}

// A platform:admin read of GET /managed_clusters exposes the derived identity
// capability; a non-admin read does not.
// Verifies: PLAT-CLUS-14.01, PLAT-CLUS-14.05
func TestIdentityCapabilityVisibility(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	target := registerSpoke(t, h, client)
	identity := managedClusters.IdentityCapability(target.name)
	admin := newPrincipal(t, h, "admin-"+strings.ToLower(api.NewID()), realmRoles("platform:admin"))
	user := newPrincipal(t, h, "user-"+strings.ToLower(api.NewID()), nil)

	adminList, httpResp, err := client.DefaultAPI.ListManagedClusters(admin.rest).Execute()
	Expect(err).NotTo(HaveOccurred(), "admin list: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(capabilitiesFor(adminList, target.name)).To(ContainElement(identity), "platform:admin read must include the identity capability")

	userList, httpResp, err := client.DefaultAPI.ListManagedClusters(user.rest).Execute()
	Expect(err).NotTo(HaveOccurred(), "user list: HTTP %d %s", statusOf(httpResp), test.APIErrorReason(err))
	Expect(capabilitiesFor(userList, target.name)).NotTo(ContainElement(identity), "non-admin read must omit the identity capability")
}

// A registration heartbeat that declares a managed-cluster/* capability is rejected
// with 400: the reserved namespace is server-assigned and may not be declared.
// Verifies: PLAT-CLUS-14.02
func TestRegistrationRejectsReservedCapability(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	subject := "cp-reserved-" + strings.ToLower(api.NewID())
	p := newPrincipal(t, h, "service-account-cp-"+strings.ToLower(api.NewID()), test.ControlPlaneClaims(subject))
	name := "mc-reserved-" + strings.ToLower(api.NewID())

	_, httpResp, err := client.DefaultAPI.RegisterManagedCluster(p.rest).
		ManagedClusterRegistrationRequest(openapi.ManagedClusterRegistrationRequest{
			Name:         name,
			Capabilities: []string{managedClusters.IdentityCapability(name)},
		}).Execute()
	Expect(err).To(HaveOccurred(), "registration with a reserved capability unexpectedly succeeded")
	Expect(statusOf(httpResp)).To(Equal(http.StatusBadRequest))
}

func capabilitiesFor(list *openapi.ManagedClusterList, name string) []string {
	for _, item := range list.Items {
		if item.Name == name {
			return item.Capabilities
		}
	}
	return nil
}
