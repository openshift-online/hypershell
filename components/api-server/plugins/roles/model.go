package roles

import (
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"gorm.io/gorm"
)

type Role struct {
	api.Meta
	Name        string  `json:"name" gorm:"uniqueIndex"`
	DisplayName *string `json:"display_name"`
	Description *string `json:"description"`
	Permissions *string `json:"permissions" gorm:"type:jsonb"`
	BuiltIn     bool    `json:"built_in"`
}

type RoleList []*Role
type RoleIndex map[string]*Role

func (l RoleList) Index() RoleIndex {
	index := RoleIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (d *Role) BeforeCreate(tx *gorm.DB) error {
	if d.ID == "" {
		d.ID = api.NewID()
	}
	return nil
}

const (
	RolePlatformAdmin  = "platform:admin"
	RoleGatewayCreator = "gateway:creator"
	// RoleGatewayDedicatedRouteCreator is the platform-wide entitlement to request
	// a dedicated per-gateway route and wildcard certificate at gateway create
	// (GatewayCreateRequest.dedicated_route). It is deliberately separate from
	// gateway:creator because a dedicated route and per-gateway certificate is a
	// rate-limited resource.
	RoleGatewayDedicatedRouteCreator = "gateway:dedicated-route-creator"
	RoleGatewayOwner                 = "gateway:owner"
	RoleGatewayAdmin                 = "gateway:admin"
	RoleGatewayViewer                = "gateway:viewer"
	RoleManagedClusterRegistrar      = "managed-cluster-registrar"
	// RoleCIFederationAdmin is the narrow platform-wide role that authorizes the
	// ci_providers ext kind (register/read/update/delete a CI OIDC forge,
	// SEC-RBAC-21) and nothing else. It lets the gitops CI-federation seeder
	// register forges with a least-privilege credential instead of
	// platform:admin. It confers no gateway, platform, or other ext-kind
	// authority.
	//
	// Unlike the other platform-wide roles below it is NOT Keycloak-owned: it is
	// assigned through the RoleBinding API (scope=global) by a platform:admin or by
	// a ci-federation:grantor (SEC-RBAC-23), so authorization over ci_providers is a
	// decision HyperShell makes from its own RBAC, not one delegated to a Keycloak
	// realm role.
	RoleCIFederationAdmin = "ci-federation:admin"
	// RoleCIFederationGrantor is the narrow bootstrap role that authorizes creating
	// and deleting a global ci-federation:admin RoleBinding (SEC-RBAC-23) -- and
	// nothing else. It exists so the CI-federation seeder's ci_providers
	// authorization can be granted declaratively from gitops (a dedicated bootstrap
	// identity holds this role via a Keycloak realm role) instead of requiring a
	// platform:admin to run the one-time grant. It is itself Keycloak-owned
	// (JWT-synced) and confers no other authority: it cannot grant any role other
	// than ci-federation:admin, and holding it does not authorize ci_providers.
	RoleCIFederationGrantor = "ci-federation:grantor"
)

// JWTSyncedRoles is the set of roles whose bindings are reconciled from the
// caller's JWT realm roles by SyncJWTRoles. Role assignment for a synced role is
// therefore owned by Keycloak, not by the RoleBinding API: SyncJWTRoles removes a
// binding whose role the JWT no longer carries, so a directly created binding would
// not survive the caller's next sign-in.
var JWTSyncedRoles = map[string]bool{
	RolePlatformAdmin:                true,
	RoleGatewayCreator:               true,
	RoleGatewayDedicatedRouteCreator: true,
	// RoleCIFederationGrantor is the bootstrap grantor (SEC-RBAC-23): it is held by
	// a gitops bootstrap identity via a Keycloak realm role, so it IS Keycloak-owned
	// and synced here.
	RoleCIFederationGrantor: true,
	// RoleCIFederationAdmin is intentionally absent: it is granted through the
	// RoleBinding API by a platform:admin or ci-federation:grantor (SEC-RBAC-21/23),
	// not synced from a Keycloak realm role. Keeping it out of this set means an
	// API-created binding is never revoked by SyncJWTRoles on the caller's next
	// sign-in.
}
