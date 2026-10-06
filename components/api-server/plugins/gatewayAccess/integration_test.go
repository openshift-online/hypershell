package gatewayAccess_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/plugins/gatewayAccess"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// fakeDirectory is an in-memory Keycloak realm directory keyed by lowercased
// username, standing in for the control-plane projection (GAM-09).
type fakeDirectory struct {
	users map[string]gatewayAccess.DirectoryUser
}

func (f fakeDirectory) Resolve(_ context.Context, username string) (gatewayAccess.DirectoryUser, bool, error) {
	u, ok := f.users[strings.ToLower(username)]
	return u, ok, nil
}

func (f fakeDirectory) Search(_ context.Context, query string, _ int) ([]gatewayAccess.DirectoryUser, error) {
	q := strings.ToLower(query)
	out := []gatewayAccess.DirectoryUser{}
	for _, u := range f.users {
		if q == "" || strings.Contains(strings.ToLower(u.Username), q) || strings.Contains(strings.ToLower(u.Name), q) {
			out = append(out, u)
		}
	}
	return out, nil
}

func realm(users ...gatewayAccess.DirectoryUser) fakeDirectory {
	m := map[string]gatewayAccess.DirectoryUser{}
	for _, u := range users {
		m[strings.ToLower(u.Username)] = u
	}
	return fakeDirectory{users: m}
}

func rbService() roleBindings.RoleBindingService {
	return roleBindings.Service(&environments.Environment().Services)
}
func userService() users.UserService {
	return users.Service(&environments.Environment().Services)
}

// seed provisions a user and gives them roleName on gatewayID, returning the
// user's id. It uses the service's convergence path (no caller authz).
func seed(t *testing.T, gatewayID, username, roleName string) string {
	t.Helper()
	uid, err := userService().UpsertByUsername(context.Background(), strings.ToLower(username), nil, nil)
	Expect(err).NotTo(HaveOccurred())
	_, serr := rbService().SetGatewayRole(context.Background(), uid, gatewayID, roleName)
	Expect(serr).To(BeNil())
	return uid
}

// accessSvc returns the real facade service with a fake directory installed.
func accessSvc(dir gatewayAccess.DirectoryResolver) gatewayAccess.Service {
	gatewayAccess.SetDirectoryResolver(dir)
	return gatewayAccess.ServiceFrom(&environments.Environment().Services)
}

func TestAccessList_EnrichedTiersAndCreator(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-list-enriched"

	creator := seed(t, gw, "creator-user", roles.RoleGatewayOwner)
	seed(t, gw, "second-owner", roles.RoleGatewayOwner)
	seed(t, gw, "an-admin", roles.RoleGatewayAdmin)
	seed(t, gw, "viewer-one", roles.RoleGatewayViewer)
	seed(t, gw, "viewer-two", roles.RoleGatewayViewer)

	svc := accessSvc(realm())
	items, total, caps, aerr := svc.List(context.Background(), gw, creator, gatewayAccess.ListOptions{Page: 1, Size: 50})
	Expect(aerr).To(BeNil())
	Expect(total).To(Equal(int64(5)))
	Expect(items).To(HaveLen(5))

	creators := 0
	tiers := map[string]int{}
	for _, it := range items {
		tiers[it.Role]++
		if it.IsCreator {
			creators++
			Expect(it.Role).To(Equal(gatewayAccess.TierOwner))
			Expect(it.UserID).To(Equal(creator))
		}
	}
	Expect(creators).To(Equal(1))
	Expect(tiers[gatewayAccess.TierOwner]).To(Equal(2))
	Expect(tiers[gatewayAccess.TierAdmin]).To(Equal(1))
	Expect(tiers[gatewayAccess.TierUser]).To(Equal(2))

	// Caller is an owner.
	Expect(caps.CallerRole).To(Equal(gatewayAccess.TierOwner))
	Expect(caps.CanManageAccess).To(BeTrue())
	Expect(caps.CanManageOwners).To(BeTrue())
}

func TestAccessList_FilterByRoleAndSearch(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-list-filter"

	owner := seed(t, gw, "owner-ff", roles.RoleGatewayOwner)
	seed(t, gw, "alice-user", roles.RoleGatewayViewer)
	seed(t, gw, "bob-user", roles.RoleGatewayViewer)
	seed(t, gw, "alice-admin", roles.RoleGatewayAdmin)

	svc := accessSvc(realm())
	items, _, _, aerr := svc.List(context.Background(), gw, owner, gatewayAccess.ListOptions{Page: 1, Size: 50, Role: gatewayAccess.TierUser, Search: "ali"})
	Expect(aerr).To(BeNil())
	Expect(items).To(HaveLen(1))
	Expect(items[0].Username).To(Equal("alice-user"))
	Expect(items[0].Role).To(Equal(gatewayAccess.TierUser))
}

func TestGrant_NeverSignedInUserPreProvisions(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-grant-newuser"
	owner := seed(t, gw, "owner-np", roles.RoleGatewayOwner)

	dir := realm(gatewayAccess.DirectoryUser{Username: "dana", Name: "Dana Scully", Email: "dana@example.com", Subject: "sub-dana"})
	svc := accessSvc(dir)

	item, aerr := svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "dana", Role: gatewayAccess.TierUser})
	Expect(aerr).To(BeNil())
	Expect(item.Role).To(Equal(gatewayAccess.TierUser))
	Expect(item.Username).To(Equal("dana"))

	// A HyperShell user record was pre-provisioned from the directory.
	u, uerr := userService().GetByUsername(context.Background(), "dana")
	Expect(uerr).To(BeNil())
	Expect(u.Name).NotTo(BeNil())
	Expect(*u.Name).To(Equal("Dana Scully"))
}

func TestGrant_UnknownRealmUserRejected(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-grant-ghost"
	owner := seed(t, gw, "owner-gh", roles.RoleGatewayOwner)

	svc := accessSvc(realm()) // empty realm
	_, aerr := svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "ghost", Role: gatewayAccess.TierUser})
	Expect(aerr).NotTo(BeNil())
	Expect(aerr.Status).To(Equal(http.StatusNotFound))

	// No user record pre-provisioned.
	_, uerr := userService().GetByUsername(context.Background(), "ghost")
	Expect(uerr).NotTo(BeNil())
}

func TestGrant_AdminCannotAssignOwner(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-admin-noowner"
	seed(t, gw, "owner-ao", roles.RoleGatewayOwner)
	admin := seed(t, gw, "admin-ao", roles.RoleGatewayAdmin)

	dir := realm(gatewayAccess.DirectoryUser{Username: "erin", Name: "Erin"})
	svc := accessSvc(dir)
	_, aerr := svc.Grant(context.Background(), gw, admin, gatewayAccess.GrantInput{Username: "erin", Role: gatewayAccess.TierOwner})
	Expect(aerr).NotTo(BeNil())
	Expect(aerr.Status).To(Equal(http.StatusForbidden))
}

func TestGrant_AdminCanGrantUser(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-admin-grants"
	seed(t, gw, "owner-ag", roles.RoleGatewayOwner)
	admin := seed(t, gw, "admin-ag", roles.RoleGatewayAdmin)

	dir := realm(gatewayAccess.DirectoryUser{Username: "dee", Name: "Dee"})
	svc := accessSvc(dir)
	item, aerr := svc.Grant(context.Background(), gw, admin, gatewayAccess.GrantInput{Username: "dee", Role: gatewayAccess.TierUser})
	Expect(aerr).To(BeNil())
	Expect(item.Role).To(Equal(gatewayAccess.TierUser))
}

func TestChangeRole_PromoteInPlaceKeepsBindingID(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-change-inplace"
	owner := seed(t, gw, "owner-ip", roles.RoleGatewayOwner)
	target := seed(t, gw, "target-ip", roles.RoleGatewayViewer)

	svc := accessSvc(realm())
	// Capture the original binding id.
	before, _, _, _ := svc.List(context.Background(), gw, owner, gatewayAccess.ListOptions{Page: 1, Size: 50})
	var beforeID string
	for _, it := range before {
		if it.UserID == target {
			beforeID = it.RoleBindingID
		}
	}
	Expect(beforeID).NotTo(BeEmpty())

	item, aerr := svc.ChangeRole(context.Background(), gw, owner, target, gatewayAccess.TierAdmin)
	Expect(aerr).To(BeNil())
	Expect(item.Role).To(Equal(gatewayAccess.TierAdmin))
	// In-place update (GAM-05): same binding id, no delete-then-create.
	Expect(item.RoleBindingID).To(Equal(beforeID))

	// Exactly one binding remains for the target (GAM-10).
	after, _, _, _ := svc.List(context.Background(), gw, owner, gatewayAccess.ListOptions{Page: 1, Size: 50})
	count := 0
	for _, it := range after {
		if it.UserID == target {
			count++
		}
	}
	Expect(count).To(Equal(1))
}

func TestChangeRole_AdminCannotPromoteToOwner(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-admin-nopromote"
	seed(t, gw, "owner-ap2", roles.RoleGatewayOwner)
	admin := seed(t, gw, "admin-ap2", roles.RoleGatewayAdmin)
	target := seed(t, gw, "viewer-ap2", roles.RoleGatewayViewer)

	svc := accessSvc(realm())
	_, aerr := svc.ChangeRole(context.Background(), gw, admin, target, gatewayAccess.TierOwner)
	Expect(aerr).NotTo(BeNil())
	Expect(aerr.Status).To(Equal(http.StatusForbidden))
}

func TestLastOwner_SoleOwnerCannotDemote(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-sole-demote"
	owner := seed(t, gw, "sole-owner-d", roles.RoleGatewayOwner)

	svc := accessSvc(realm())
	_, aerr := svc.ChangeRole(context.Background(), gw, owner, owner, gatewayAccess.TierUser)
	Expect(aerr).NotTo(BeNil())
	Expect(aerr.Status).To(Equal(http.StatusConflict))
	Expect(aerr.Message).NotTo(BeEmpty())
}

func TestLastOwner_SoleOwnerCannotBeRevoked(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-sole-revoke"
	owner := seed(t, gw, "sole-owner-r", roles.RoleGatewayOwner)

	svc := accessSvc(realm())
	aerr := svc.Revoke(context.Background(), gw, owner, owner)
	Expect(aerr).NotTo(BeNil())
	Expect(aerr.Status).To(Equal(http.StatusConflict))
}

func TestRevoke_OwnerSelfWhenAnotherOwnerRemains(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-two-owners"
	a := seed(t, gw, "owner-a-two", roles.RoleGatewayOwner)
	seed(t, gw, "owner-e-two", roles.RoleGatewayOwner)

	svc := accessSvc(realm())
	aerr := svc.Revoke(context.Background(), gw, a, a)
	Expect(aerr).To(BeNil())

	tier, _ := userTierOf(svc, gw, a)
	Expect(tier).To(Equal(""))
}

func TestRevoke_AdminCannotRevokeOwner(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-admin-norevoke-owner"
	owner := seed(t, gw, "owner-nr", roles.RoleGatewayOwner)
	admin := seed(t, gw, "admin-nr", roles.RoleGatewayAdmin)

	svc := accessSvc(realm())
	aerr := svc.Revoke(context.Background(), gw, admin, owner)
	Expect(aerr).NotTo(BeNil())
	Expect(aerr.Status).To(Equal(http.StatusForbidden))
}

func TestViewerCannotManage(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-viewer-nomanage"
	seed(t, gw, "owner-vn", roles.RoleGatewayOwner)
	viewer := seed(t, gw, "viewer-vn", roles.RoleGatewayViewer)

	dir := realm(gatewayAccess.DirectoryUser{Username: "newbie"})
	svc := accessSvc(dir)
	_, aerr := svc.Grant(context.Background(), gw, viewer, gatewayAccess.GrantInput{Username: "newbie", Role: gatewayAccess.TierUser})
	Expect(aerr).NotTo(BeNil())
	Expect(aerr.Status).To(Equal(http.StatusForbidden))
}

func TestIdempotentRegrant(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-idempotent"
	owner := seed(t, gw, "owner-id", roles.RoleGatewayOwner)
	dir := realm(gatewayAccess.DirectoryUser{Username: "repeat"})
	svc := accessSvc(dir)

	_, aerr := svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "repeat", Role: gatewayAccess.TierUser})
	Expect(aerr).To(BeNil())
	_, aerr = svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "repeat", Role: gatewayAccess.TierUser})
	Expect(aerr).To(BeNil())

	items, _, _, _ := svc.List(context.Background(), gw, owner, gatewayAccess.ListOptions{Page: 1, Size: 50})
	count := 0
	for _, it := range items {
		if it.Username == "repeat" {
			count++
		}
	}
	Expect(count).To(Equal(1))
}

func TestDirectorySearch_Authorization(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-dir-authz"
	admin := seed(t, gw, "admin-dir", roles.RoleGatewayAdmin)
	viewer := seed(t, gw, "viewer-dir", roles.RoleGatewayViewer)

	dir := realm(
		gatewayAccess.DirectoryUser{Username: "dana", Name: "Dana"},
		gatewayAccess.DirectoryUser{Username: "dale", Name: "Dale"},
	)
	svc := accessSvc(dir)

	// Viewer cannot search.
	_, aerr := svc.SearchDirectory(context.Background(), gw, viewer, "da")
	Expect(aerr).NotTo(BeNil())
	Expect(aerr.Status).To(Equal(http.StatusForbidden))

	// Admin can.
	candidates, aerr := svc.SearchDirectory(context.Background(), gw, admin, "da")
	Expect(aerr).To(BeNil())
	Expect(len(candidates)).To(Equal(2))
}

// userTierOf lists access and returns the target's tier via capabilities-style
// lookup (the caller is the subject here).
func userTierOf(svc gatewayAccess.Service, gw, userID string) (string, error) {
	items, _, _, aerr := svc.List(context.Background(), gw, userID, gatewayAccess.ListOptions{Page: 1, Size: 100})
	if aerr != nil {
		return "", aerr
	}
	for _, it := range items {
		if it.UserID == userID {
			return it.Role, nil
		}
	}
	return "", nil
}
