package gateways

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// TestOwnedGatewayIDsExcludesSoftDeletedGateway proves the owned-is-a-subset-of-accessible
// invariant (architecture.spec.md WEB-API-01, WEB-UIEX-07, scenario WEB-UIEX-07.06): a
// gateway whose gateway:owner binding is still live but whose gateway row has been
// soft-deleted MUST NOT count as owned. Were it still counted, the owned set would stop
// being a subset of the caller's accessible (non-deleted) set and the "Show only my
// gateways" toggle -- shown exactly when the accessible set contains a gateway the caller
// does not own (len(accessible) > len(owned)) -- would be wrongly hidden.
//
// This is a regression guard for ownership_dao.go OwnedGatewayIDs: before the gateways
// JOIN with g.deleted_at IS NULL, the query returned the soft-deleted gateway, making
// owned == accessible and hiding the toggle.
func TestOwnedGatewayIDsExcludesSoftDeletedGateway(t *testing.T) {
	test.RegisterIntegration(t)

	ctx := context.Background()
	env := environments.Environment()
	sessionFactory := &env.Database.SessionFactory

	gatewayService := Service(&env.Services)
	userService := users.Service(&env.Services)
	bindingService := roleBindings.Service(&env.Services)
	Expect(gatewayService).NotTo(BeNil())
	Expect(userService).NotTo(BeNil())
	Expect(bindingService).NotTo(BeNil())

	// Control-plane writer context so the fully-provisioned gateway fields are
	// accepted by the default-deny field policy (PLAT-DATA-23).
	writeCtx := rbac.WithGatewayWriterKind(ctx, rbac.GatewayWriterControlPlane)

	newProvisionedGateway := func(name string) *Gateway {
		gw, err := gatewayService.Create(writeCtx, &Gateway{
			Name:           name,
			ClusterId:      "test-cluster",
			GatewayVersion: strPtr("0.0.109"),
		})
		Expect(err).To(BeNil())
		return gw
	}

	owned := newProvisionedGateway("soft-delete-owned")
	other := newProvisionedGateway("soft-delete-other")

	userID, uerr := userService.UpsertByUsername(ctx, "soft-delete-owner", nil, nil)
	Expect(uerr).NotTo(HaveOccurred())

	// The caller owns `owned` and can additionally see `other` (viewer), so the
	// authorized set contains a gateway the caller does not own.
	Expect(bindingService.CreateGatewayOwnerBinding(ctx, userID, owned.ID)).To(Succeed())
	_, serr := bindingService.SetGatewayRole(ctx, userID, other.ID, roles.RoleGatewayViewer)
	Expect(serr).To(BeNil())

	expectSubset := func(sub, super []string) {
		for _, id := range sub {
			Expect(super).To(ContainElement(id),
				"owned gateway %s must be part of the accessible (non-deleted) set", id)
		}
	}

	dao := newGatewayOwnershipDao(sessionFactory)

	// accessibleNonDeleted returns the caller's authorized gateways whose rows are
	// not soft-deleted -- the accessible (non-deleted) set the spec requires owned
	// to be a subset of.
	accessibleNonDeleted := func() []string {
		authorized, aerr := bindingService.FindGatewayIDsByUserID(ctx, userID)
		Expect(aerr).To(BeNil())
		if len(authorized) == 0 {
			return nil
		}
		var live []string
		g := (*sessionFactory).New(ctx)
		Expect(g.Raw(
			`SELECT id FROM gateways WHERE deleted_at IS NULL AND id IN (?)`,
			authorized,
		).Scan(&live).Error).To(Succeed())
		return live
	}

	// Baseline: owned = {owned}, accessible = {owned, other}, toggle visible.
	ownedIDs, err := dao.OwnedGatewayIDs(ctx, userID)
	Expect(err).NotTo(HaveOccurred())
	Expect(ownedIDs).To(ConsistOf(owned.ID))
	accessibleIDs := accessibleNonDeleted()
	Expect(accessibleIDs).To(ConsistOf(owned.ID, other.ID))
	expectSubset(ownedIDs, accessibleIDs)
	Expect(len(accessibleIDs) > len(ownedIDs)).To(BeTrue(),
		"toggle must be visible while an unowned accessible gateway exists")

	// Soft-delete the owned gateway while deliberately leaving the gateway:owner
	// binding live (the anomalous state WEB-UIEX-07.06 defends).
	g := (*sessionFactory).New(ctx)
	Expect(g.Exec(`UPDATE gateways SET deleted_at = now() WHERE id = ?`, owned.ID).Error).To(Succeed())

	stillBound, berr := bindingService.FindGatewayIDsByUserID(ctx, userID)
	Expect(berr).To(BeNil())
	Expect(stillBound).To(ContainElement(owned.ID),
		"precondition: the owner binding must remain live after the gateway is soft-deleted")

	// The fix: the soft-deleted gateway is no longer owned.
	ownedIDs, err = dao.OwnedGatewayIDs(ctx, userID)
	Expect(err).NotTo(HaveOccurred())
	Expect(ownedIDs).NotTo(ContainElement(owned.ID),
		"a soft-deleted gateway must not be returned as owned (WEB-API-01)")
	Expect(ownedIDs).To(BeEmpty())

	// Owned stays a subset of accessible, and the toggle stays visible because the
	// caller still sees `other`, a gateway it does not own. Without the fix owned
	// would still contain the soft-deleted gateway, making len(accessible) ==
	// len(owned) and wrongly hiding the toggle.
	accessibleIDs = accessibleNonDeleted()
	Expect(accessibleIDs).To(ConsistOf(other.ID))
	expectSubset(ownedIDs, accessibleIDs)
	Expect(len(accessibleIDs) > len(ownedIDs)).To(BeTrue(),
		"toggle must stay visible; a soft-deleted owned gateway must not corrupt it (WEB-UIEX-07.06)")
}
