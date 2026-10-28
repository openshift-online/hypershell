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
}
