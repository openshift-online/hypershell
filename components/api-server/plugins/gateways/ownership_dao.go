package gateways

import (
	"context"

	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

// GatewayOwnershipLookup answers ownership questions needed by the gateway list:
// which gateways the caller owns (holds gateway:owner on), and whether the
// caller's authorized scope contains any gateway the caller does not own. It
// backs the "Show only my gateways" ownership filter and the conditional
// visibility of its toggle (WEB-UIEX-07, WEB-API-01). Ownership means the caller
// holds gateway:owner (gateway-access-management.spec.md); it never means
// ownership of the hosting cluster.
type GatewayOwnershipLookup interface {
	// OwnedGatewayIDs returns the identifiers of gateways on which the given user
	// holds gateway:owner. It returns an empty slice when the user owns none.
	OwnedGatewayIDs(ctx context.Context, userID string) ([]string, error)
	// OtherOwnerGatewayExists reports whether at least one non-deleted gateway
	// exists that the given user does NOT own (does not hold gateway:owner on).
	// It is evaluated with a single bounded existence query rather than by
	// enumerating pages, and is only meaningful over an unrestricted (platform
	// admin) scope; caller-scoped existence is derived in memory from the
	// authorized set to respect RBAC visibility (SEC-RBAC-10).
	OtherOwnerGatewayExists(ctx context.Context, userID string) (bool, error)
}

type gatewayOwnershipDao struct {
	sessionFactory *db.SessionFactory
}

func newGatewayOwnershipDao(sessionFactory *db.SessionFactory) *gatewayOwnershipDao {
	return &gatewayOwnershipDao{sessionFactory: sessionFactory}
}

func (d *gatewayOwnershipDao) OwnedGatewayIDs(ctx context.Context, userID string) ([]string, error) {
	if userID == "" {
		return nil, nil
	}
	g2 := (*d.sessionFactory).New(ctx)
	var ids []string
	const query = `
		SELECT DISTINCT rb.gateway_id
		FROM role_bindings rb
		JOIN roles r ON r.id = rb.role_id AND r.deleted_at IS NULL
		WHERE rb.user_id = ?
		  AND rb.gateway_id IS NOT NULL
		  AND rb.deleted_at IS NULL
		  AND r.name = ?`
	if err := g2.Raw(query, userID, roles.RoleGatewayOwner).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (d *gatewayOwnershipDao) OtherOwnerGatewayExists(ctx context.Context, userID string) (bool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var exists bool
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM gateways g
			WHERE g.deleted_at IS NULL
			  AND NOT EXISTS (
				SELECT 1
				FROM role_bindings rb
				JOIN roles r ON r.id = rb.role_id AND r.deleted_at IS NULL
				WHERE rb.gateway_id = g.id
				  AND rb.user_id = ?
				  AND rb.deleted_at IS NULL
				  AND r.name = ?
			  )
		)`
	if err := g2.Raw(query, userID, roles.RoleGatewayOwner).Scan(&exists).Error; err != nil {
		return false, err
	}
	return exists, nil
}
