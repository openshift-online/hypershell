package authz_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// These tests pin read visibility on the low-level role_bindings resource and
// the gateway list search (SEC-RBAC-10), with RBAC enforced.

// readFixture is two gateways owned by one user, and a second user who is a
// viewer on the first gateway only.
type readFixture struct {
	owner, viewer, admin principal
	gw1, gw2             *openapi.Gateway
	viewerUserID         string
}

func newReadFixture(t *testing.T, h *test.Helper, client *openapi.APIClient) readFixture {
	t.Helper()
	cluster := registerSpoke(t, h, client)
	suffix := strings.ToLower(api.NewID())
	f := readFixture{
		owner:  newPrincipal(t, h, "owner-"+suffix, realmRoles(roles.RoleGatewayCreator)),
		viewer: newPrincipal(t, h, "viewer-"+suffix, nil),
		admin:  newPrincipal(t, h, "admin-"+suffix, realmRoles(roles.RolePlatformAdmin)),
	}
	create := func(prefix string) *openapi.Gateway {
		gw, resp, err := client.DefaultAPI.CreateGateway(f.owner.rest).
			GatewayCreateRequest(gatewayCreateRequest(uniqueName(prefix), cluster.clusterID, false)).Execute()
		Expect(statusOf(resp)).To(Equal(http.StatusCreated), "create gateway: %s", test.APIErrorReason(err))
		return gw
	}
	f.gw1 = create("reads-a")
	f.gw2 = create("reads-b")

	// The first request provisions the viewer's User record.
	_, _, _ = client.DefaultAPI.ListGateways(f.viewer.rest).Execute()
	envServices := &environments.Environment().Services
	viewerUser, svcErr := users.Service(envServices).GetByUsername(context.Background(), f.viewer.name)
	Expect(svcErr).NotTo(HaveOccurred(), "find viewer user")
	f.viewerUserID = viewerUser.ID
	_, svcErr = roleBindings.Service(envServices).SetGatewayRole(context.Background(), viewerUser.ID, f.gw1.GetId(), roles.RoleGatewayViewer)
	Expect(svcErr).NotTo(HaveOccurred(), "grant viewer on gw1")
	return f
}

func gatewaysOf(list *openapi.RoleBindingList) map[string]bool {
	seen := map[string]bool{}
	for _, item := range list.GetItems() {
		seen[item.GetGatewayId()] = true
	}
	return seen
}

// Verifies: SEC-RBAC-10.02
func TestRESTRoleBindingListShowsOnlyVisibleGateways(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	f := newReadFixture(t, h, client)

	list, resp, err := client.DefaultAPI.ListRoleBindings(f.viewer.rest).Size(500).Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusOK), "list: %s", test.APIErrorReason(err))
	seen := gatewaysOf(list)
	Expect(seen[f.gw1.GetId()]).To(BeTrue(), "viewer must see bindings on gw1")
	Expect(seen[f.gw2.GetId()]).To(BeFalse(), "viewer must not see bindings on gw2")
	for _, item := range list.GetItems() {
		if item.GetScope() == roleBindings.ScopeGlobal {
			Expect(item.GetUserId()).To(Equal(f.viewerUserID), "viewer must not see another user's global binding")
		}
	}

	// A caller search is combined with the restriction, not replacing it.
	searched, resp, err := client.DefaultAPI.ListRoleBindings(f.viewer.rest).
		Search("gateway_id = '" + f.gw2.GetId() + "'").Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusOK), "search: %s", test.APIErrorReason(err))
	Expect(searched.GetItems()).To(BeEmpty(), "search must not reveal gw2 bindings")

	// A search that is not a complete expression on its own is rejected.
	_, resp, _ = client.DefaultAPI.ListRoleBindings(f.viewer.rest).
		Search("gateway_id = 'x') or (gateway_id = '" + f.gw2.GetId() + "'").Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusBadRequest))
}

// Verifies: SEC-RBAC-10.03
func TestRESTRoleBindingGetOnOtherGatewayIsNotFound(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	f := newReadFixture(t, h, client)

	gw2Bindings, svcErr := roleBindings.Service(&environments.Environment().Services).
		BindingsForGateway(context.Background(), f.gw2.GetId())
	Expect(svcErr).NotTo(HaveOccurred())
	Expect(gw2Bindings).NotTo(BeEmpty())
	hidden := gw2Bindings[0].ID

	_, resp, _ := client.DefaultAPI.GetRoleBinding(f.viewer.rest, hidden).Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusNotFound), "binding on an unseen gateway must be 404")

	_, resp, err := client.DefaultAPI.GetRoleBinding(f.owner.rest, hidden).Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusOK), "owner reads it: %s", test.APIErrorReason(err))
}

// Verifies: SEC-RBAC-10.04
func TestRESTRoleBindingListPlatformAdminSeesAll(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	f := newReadFixture(t, h, client)

	list, resp, err := client.DefaultAPI.ListRoleBindings(f.admin.rest).Size(500).Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusOK), "list: %s", test.APIErrorReason(err))
	seen := gatewaysOf(list)
	Expect(seen[f.gw1.GetId()]).To(BeTrue())
	Expect(seen[f.gw2.GetId()]).To(BeTrue())
}

// Verifies: SEC-RBAC-07.07
func TestRESTPlatformAdminCannotCreateLowLevelRoleBinding(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	f := newReadFixture(t, h, client)

	viewerRole, svcErr := roles.Service(&environments.Environment().Services).
		GetByName(context.Background(), roles.RoleGatewayViewer)
	Expect(svcErr).NotTo(HaveOccurred())
	gatewayID := f.gw1.GetId()
	_, resp, _ := client.DefaultAPI.CreateRoleBinding(f.admin.rest).RoleBinding(openapi.RoleBinding{
		RoleId:    viewerRole.ID,
		Scope:     roleBindings.ScopeGateway,
		GatewayId: &gatewayID,
		UserId:    &f.viewerUserID,
	}).Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusForbidden))
}

// Verifies: SEC-RBAC-10.05
func TestRESTGatewaySearchCannotEscapeVisibility(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	f := newReadFixture(t, h, client)
	name1, name2 := f.gw1.GetName(), f.gw2.GetName()

	// A plain search still works and stays inside the visible set.
	list, resp, err := client.DefaultAPI.ListGateways(f.viewer.rest).Search("name = '" + name1 + "'").Execute()
	Expect(statusOf(resp)).To(Equal(http.StatusOK), "search: %s", test.APIErrorReason(err))
	Expect(list.GetItems()).To(HaveLen(1))

	// The crafted value closes the parentheses the server adds around it. The
	// server appends "and id in (...)", which binds tighter than "or", so the
	// first half (naming the unseen gw2) is left unrestricted.
	list, resp, _ = client.DefaultAPI.ListGateways(f.viewer.rest).
		Search("name = '" + name2 + "') or (name = 'no-such-gateway'").Execute()
	if list != nil {
		for _, gw := range list.GetItems() {
			Expect(gw.GetName()).NotTo(Equal(name2), "gw2 leaked through the search")
		}
	}
	Expect(statusOf(resp)).To(Equal(http.StatusBadRequest))
}
