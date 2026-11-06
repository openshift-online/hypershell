package gatewayAccess_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/plugins/defaultWorkspaceMemberships"
	"github.com/openshift-online/hypershell/components/api-server/plugins/gatewayAccess"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

func membershipService() defaultWorkspaceMemberships.MembershipService {
	return defaultWorkspaceMemberships.Service(&environments.Environment().Services)
}

// Verifies: GTW-ACC-16.01
func TestGrant_UserTierCreatesDefaultWorkspaceMembership(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-dwm-grant-user"
	owner := seed(t, gw, "owner-dwm1", roles.RoleGatewayOwner)

	dir := realm(gatewayAccess.DirectoryUser{Username: "dana", Name: "Dana", Subject: "sub-dana-1"})
	svc := accessSvc(dir)

	_, aerr := svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "dana", Role: gatewayAccess.TierUser})
	Expect(aerr).To(BeNil())

	m, serr := membershipService().FindByGatewayAndSubject(context.Background(), gw, "sub-dana-1")
	Expect(serr).To(BeNil())
	Expect(m).NotTo(BeNil())
	Expect(m.Role).To(Equal(defaultWorkspaceMemberships.RoleUser))
	Expect(m.Source).To(Equal(defaultWorkspaceMemberships.SourceGatewayAccess))
	// Not the username: OpenShell's workspace authorization matches the real
	// Keycloak sub, never the mutable, human-readable username (GTW-ACC-16).
	Expect(m.Subject).NotTo(Equal("dana"))
}

// Verifies: GTW-ACC-16.02
func TestGrant_AdminOrOwnerTierCreatesNoDefaultWorkspaceMembership(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-dwm-grant-admin"
	owner := seed(t, gw, "owner-dwm2", roles.RoleGatewayOwner)

	dir := realm(gatewayAccess.DirectoryUser{Username: "erin", Subject: "sub-erin-1"})
	svc := accessSvc(dir)

	_, aerr := svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "erin", Role: gatewayAccess.TierAdmin})
	Expect(aerr).To(BeNil())

	m, serr := membershipService().FindByGatewayAndSubject(context.Background(), gw, "sub-erin-1")
	Expect(serr).To(BeNil())
	Expect(m).To(BeNil())
}

// Verifies: GTW-ACC-16.03
func TestChangeRole_PromotionOutOfUserTierRemovesMembership(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-dwm-promote"
	owner := seed(t, gw, "owner-dwm3", roles.RoleGatewayOwner)

	dir := realm(gatewayAccess.DirectoryUser{Username: "ben", Subject: "sub-ben-1"})
	svc := accessSvc(dir)
	item, aerr := svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "ben", Role: gatewayAccess.TierUser})
	Expect(aerr).To(BeNil())

	m, _ := membershipService().FindByGatewayAndSubject(context.Background(), gw, "sub-ben-1")
	Expect(m).NotTo(BeNil())

	_, aerr = svc.ChangeRole(context.Background(), gw, owner, item.UserID, gatewayAccess.TierAdmin)
	Expect(aerr).To(BeNil())

	m, serr := membershipService().FindByGatewayAndSubject(context.Background(), gw, "sub-ben-1")
	Expect(serr).To(BeNil())
	Expect(m).To(BeNil())
}

// Verifies: GTW-ACC-16.04
func TestChangeRole_DemotionIntoUserTierCreatesMembership(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-dwm-demote"
	owner := seed(t, gw, "owner-dwm4", roles.RoleGatewayOwner)

	dir := realm(gatewayAccess.DirectoryUser{Username: "cal", Subject: "sub-cal-1"})
	svc := accessSvc(dir)
	item, aerr := svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "cal", Role: gatewayAccess.TierAdmin})
	Expect(aerr).To(BeNil())

	m, _ := membershipService().FindByGatewayAndSubject(context.Background(), gw, "sub-cal-1")
	Expect(m).To(BeNil())

	_, aerr = svc.ChangeRole(context.Background(), gw, owner, item.UserID, gatewayAccess.TierUser)
	Expect(aerr).To(BeNil())

	m, serr := membershipService().FindByGatewayAndSubject(context.Background(), gw, "sub-cal-1")
	Expect(serr).To(BeNil())
	Expect(m).NotTo(BeNil())
	Expect(m.Role).To(Equal(defaultWorkspaceMemberships.RoleUser))
}

// Verifies: GTW-ACC-16.05
func TestRevoke_DeletesDefaultWorkspaceMembership(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-dwm-revoke"
	owner := seed(t, gw, "owner-dwm5", roles.RoleGatewayOwner)

	dir := realm(gatewayAccess.DirectoryUser{Username: "fay", Subject: "sub-fay-1"})
	svc := accessSvc(dir)
	item, aerr := svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "fay", Role: gatewayAccess.TierUser})
	Expect(aerr).To(BeNil())

	aerr = svc.Revoke(context.Background(), gw, owner, item.UserID)
	Expect(aerr).To(BeNil())

	m, serr := membershipService().FindByGatewayAndSubject(context.Background(), gw, "sub-fay-1")
	Expect(serr).To(BeNil())
	Expect(m).To(BeNil())
}

// Verifies: GTW-ACC-16.06
func TestGrant_ReGrantingSameTierStaysOneMembershipRow(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-dwm-idempotent"
	owner := seed(t, gw, "owner-dwm6", roles.RoleGatewayOwner)

	dir := realm(gatewayAccess.DirectoryUser{Username: "gus", Subject: "sub-gus-1"})
	svc := accessSvc(dir)
	_, aerr := svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "gus", Role: gatewayAccess.TierUser})
	Expect(aerr).To(BeNil())
	_, aerr = svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "gus", Role: gatewayAccess.TierUser})
	Expect(aerr).To(BeNil())

	count, serr := membershipService().CountLiveForGateway(context.Background(), gw)
	Expect(serr).To(BeNil())
	Expect(count).To(Equal(int64(1)))
}

// Verifies: GTW-ACC-16.07
func TestChangeRole_DemotionResolvesMissingSubjectViaDirectory(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	gw := "gw-dwm-backfill"
	owner := seed(t, gw, "owner-dwm7", roles.RoleGatewayOwner)
	// seed() provisions the User row directly (bypassing Grant's pre-provisioning),
	// so "bob" has no stored subject yet, mirroring a User row that predates
	// subject capture.
	bob := seed(t, gw, "bob", roles.RoleGatewayAdmin)

	u, uerr := userService().Get(context.Background(), bob)
	Expect(uerr).To(BeNil())
	Expect(u.Subject).To(BeEmpty())

	dir := realm(gatewayAccess.DirectoryUser{Username: "bob", Subject: "sub-bob-resolved"})
	svc := accessSvc(dir)

	_, aerr := svc.ChangeRole(context.Background(), gw, owner, bob, gatewayAccess.TierUser)
	Expect(aerr).To(BeNil())

	m, serr := membershipService().FindByGatewayAndSubject(context.Background(), gw, "sub-bob-resolved")
	Expect(serr).To(BeNil())
	Expect(m).NotTo(BeNil())

	u, uerr = userService().Get(context.Background(), bob)
	Expect(uerr).To(BeNil())
	Expect(u.Subject).To(Equal("sub-bob-resolved"))
}

// Verifies: GTW-ACC-17.01
func TestGrant_RejectedAtDefaultWorkspaceMemberLimit(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	defer withLoweredMemberLimit(2)()
	gw := "gw-dwm-limit-grant"
	owner := seed(t, gw, "owner-dwm8", roles.RoleGatewayOwner)

	fillDefaultWorkspaceToLimit(t, gw)

	dir := realm(gatewayAccess.DirectoryUser{Username: "newuser", Subject: "sub-newuser-1"})
	svc := accessSvc(dir)
	_, aerr := svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "newuser", Role: gatewayAccess.TierUser})
	Expect(aerr).NotTo(BeNil())
	Expect(aerr.Status).To(Equal(http.StatusConflict))

	// No membership was created for the rejected grant.
	m, _ := membershipService().FindByGatewayAndSubject(context.Background(), gw, "sub-newuser-1")
	Expect(m).To(BeNil())
}

// Verifies: GTW-ACC-17.02
func TestChangeRole_DemotionRejectedAtDefaultWorkspaceMemberLimit(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	defer withLoweredMemberLimit(2)()
	gw := "gw-dwm-limit-demote"
	owner := seed(t, gw, "owner-dwm9", roles.RoleGatewayOwner)

	dir := realm(gatewayAccess.DirectoryUser{Username: "hal", Subject: "sub-hal-1"})
	svc := accessSvc(dir)
	item, aerr := svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "hal", Role: gatewayAccess.TierAdmin})
	Expect(aerr).To(BeNil())

	fillDefaultWorkspaceToLimit(t, gw)

	_, aerr = svc.ChangeRole(context.Background(), gw, owner, item.UserID, gatewayAccess.TierUser)
	Expect(aerr).NotTo(BeNil())
	Expect(aerr.Status).To(Equal(http.StatusConflict))

	tier, terr := userTierOf(svc, gw, item.UserID)
	Expect(terr).To(BeNil())
	Expect(tier).To(Equal(gatewayAccess.TierAdmin))
}

// Verifies: GTW-ACC-17.03
func TestGrant_OwnerAndAdminGrantsAreNotSubjectToTheLimit(t *testing.T) {
	test.RegisterIntegration(t)
	defer gatewayAccess.SetDirectoryResolver(nil)
	defer withLoweredMemberLimit(2)()
	gw := "gw-dwm-limit-admin-exempt"
	owner := seed(t, gw, "owner-dwm10", roles.RoleGatewayOwner)

	fillDefaultWorkspaceToLimit(t, gw)

	dir := realm(gatewayAccess.DirectoryUser{Username: "ivy", Subject: "sub-ivy-1"})
	svc := accessSvc(dir)
	_, aerr := svc.Grant(context.Background(), gw, owner, gatewayAccess.GrantInput{Username: "ivy", Role: gatewayAccess.TierAdmin})
	Expect(aerr).To(BeNil())
}

// withLoweredMemberLimit temporarily lowers the package-wide member limit so a
// test can exercise the at-limit rejection path (GTW-ACC-17) by seeding a
// handful of rows instead of 1,000 real ones. The returned func restores the
// original limit; call it via defer.
func withLoweredMemberLimit(limit int64) func() {
	original := defaultWorkspaceMemberships.MemberLimit
	defaultWorkspaceMemberships.MemberLimit = limit
	return func() { defaultWorkspaceMemberships.MemberLimit = original }
}

// fillDefaultWorkspaceToLimit seeds gatewayID's default workspace with exactly
// defaultWorkspaceMemberships.MemberLimit live rows, directly through the
// membership service (bypassing gateway-access automation, which is exactly
// what is under test). Callers lower MemberLimit first (withLoweredMemberLimit)
// so this seeds a handful of rows, not 1,000.
func fillDefaultWorkspaceToLimit(t *testing.T, gatewayID string) {
	t.Helper()
	svc := membershipService()
	for i := int64(0); i < defaultWorkspaceMemberships.MemberLimit; i++ {
		_, serr := svc.Create(context.Background(), gatewayID, "filler-subject-"+strconv.FormatInt(i, 10), defaultWorkspaceMemberships.SourceGatewayAccess)
		Expect(serr).To(BeNil())
	}
}
