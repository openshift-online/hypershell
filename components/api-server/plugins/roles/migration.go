package roles

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
)

func migration() *gormigrate.Migration {
	type Role struct {
		db.Model
		Name        string `gorm:"uniqueIndex"`
		DisplayName *string
		Description *string
		Permissions *string `gorm:"type:jsonb"`
		BuiltIn     bool
	}

	return &gormigrate.Migration{
		ID: "2026081112000002",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&Role{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&Role{})
		},
	}
}

func migrationSeedBuiltInRoles() *gormigrate.Migration {
	type Role struct {
		db.Model
		Name        string `gorm:"uniqueIndex"`
		DisplayName *string
		Description *string
		Permissions *string `gorm:"type:jsonb"`
		BuiltIn     bool
	}

	return &gormigrate.Migration{
		ID: "2026081112000004",
		Migrate: func(tx *gorm.DB) error {
			seeds := []struct {
				Name        string
				DisplayName string
				Description string
				Permissions map[string]interface{}
			}{
				{
					Name:        RolePlatformAdmin,
					DisplayName: "Platform Administrator",
					Description: "Platform-wide view and delete access for all gateways",
					Permissions: map[string]interface{}{
						"gateways": []string{"read", "delete"},
					},
				},
				{
					Name:        RoleGatewayCreator,
					DisplayName: "Gateway Creator",
					Description: "Can create gateways; auto-becomes owner on creation",
					Permissions: map[string]interface{}{
						"gateways":      []string{"create"},
						"role_bindings": []string{"create", "read", "delete", "list"},
					},
				},
				{
					Name:        RoleGatewayOwner,
					DisplayName: "Gateway Owner",
					Description: "Full CRUD on one gateway; can grant owner and viewer to others",
					Permissions: map[string]interface{}{
						"gateways":      []string{"read", "update", "delete"},
						"role_bindings": []string{"create", "read", "delete", "list"},
					},
				},
				{
					Name:        RoleGatewayViewer,
					DisplayName: "Gateway Viewer",
					Description: "Read-only access to a single gateway",
					Permissions: map[string]interface{}{
						"gateways": []string{"read"},
					},
				},
			}

			for _, seed := range seeds {
				permJSON, err := json.Marshal(seed.Permissions)
				if err != nil {
					return err
				}
				permStr := string(permJSON)
				displayName := seed.DisplayName
				description := seed.Description
				role := Role{
					Model:       db.Model{ID: api.NewID()},
					Name:        seed.Name,
					DisplayName: &displayName,
					Description: &description,
					Permissions: &permStr,
					BuiltIn:     true,
				}
				if err := tx.Create(&role).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Where("built_in = ?", true).Delete(&Role{}).Error
		},
	}
}

type roleSeed struct {
	Name        string
	DisplayName string
	Description string
	Permissions map[string]interface{}
}

var builtInRoleSeeds = []roleSeed{
	{
		Name:        RolePlatformAdmin,
		DisplayName: "Platform Administrator",
		Description: "Platform-wide view and delete access for all gateways",
		Permissions: map[string]interface{}{
			"gateways": []string{"read", "delete"},
		},
	},
	{
		Name:        RoleGatewayCreator,
		DisplayName: "Gateway Creator",
		Description: "Can create gateways; auto-becomes owner on creation",
		Permissions: map[string]interface{}{
			"gateways":      []string{"create"},
			"role_bindings": []string{"create", "read", "delete", "list"},
		},
	},
	{
		Name:        RoleGatewayOwner,
		DisplayName: "Gateway Owner",
		Description: "Full CRUD on one gateway; can grant owner, admin, and viewer to others",
		Permissions: map[string]interface{}{
			"gateways":         []string{"read", "update", "delete"},
			"role_bindings":    []string{"create", "read", "update", "delete", "list"},
			"service_accounts": []string{"create", "read", "delete", "list"},
		},
	},
	{
		Name:        RoleGatewayAdmin,
		DisplayName: "Gateway Admin",
		Description: "Granted administrator: read/update one gateway (no delete); can grant admin and viewer to others",
		Permissions: map[string]interface{}{
			"gateways":         []string{"read", "update"},
			"role_bindings":    []string{"create", "read", "update", "delete", "list"},
			"service_accounts": []string{"create", "read", "delete", "list"},
		},
	},
	{
		Name:        RoleGatewayViewer,
		DisplayName: "Gateway Viewer",
		Description: "Read-only access to a single gateway",
		Permissions: map[string]interface{}{
			"gateways": []string{"read"},
		},
	},
	{
		Name:        RoleGatewayDedicatedRouteCreator,
		DisplayName: "Gateway Dedicated Route Creator",
		Description: "Allows requesting a dedicated per-gateway route and certificate at gateway create; per-gateway certificates are a rate-limited resource.",
		Permissions: map[string]interface{}{
			"gateways": []string{"request_dedicated_route"},
		},
	},
	{
		Name:        RoleCIFederationAdmin,
		DisplayName: "CI Federation Administrator",
		Description: "Authorizes registering CI OIDC forges (ci_providers) and nothing else; a least-privilege alternative to platform:admin for the CI-federation seeder.",
		Permissions: map[string]interface{}{
			"ci_providers": []string{"create", "read", "update", "delete"},
		},
	},
}

func migrationAddPlatformAdminRole() *gormigrate.Migration {
	type Role struct {
		db.Model
		Name        string `gorm:"uniqueIndex"`
		DisplayName *string
		Description *string
		Permissions *string `gorm:"type:jsonb"`
		BuiltIn     bool
	}

	return &gormigrate.Migration{
		ID: "2026081812000001",
		Migrate: func(tx *gorm.DB) error {
			// Check if platform:admin role already exists
			var existing Role
			err := tx.Where("name = ?", RolePlatformAdmin).First(&existing).Error
			if err == nil {
				// Role already exists, nothing to do
				return nil
			}

			// Create platform:admin role
			permissions := map[string]interface{}{
				"gateways": []string{"read", "delete"},
			}
			permJSON, jsonErr := json.Marshal(permissions)
			if jsonErr != nil {
				return jsonErr
			}
			permStr := string(permJSON)
			displayName := "Platform Administrator"
			description := "Platform-wide view and delete access for all gateways"

			role := Role{
				Model:       db.Model{ID: api.NewID()},
				Name:        RolePlatformAdmin,
				DisplayName: &displayName,
				Description: &description,
				Permissions: &permStr,
				BuiltIn:     true,
			}
			return tx.Create(&role).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Where("name = ?", RolePlatformAdmin).Delete(&Role{}).Error
		},
	}
}

func migrationSeedManagedClusterRegistrarRole() *gormigrate.Migration {
	type Role struct {
		db.Model
		Name        string `gorm:"uniqueIndex"`
		DisplayName *string
		Description *string
		Permissions *string `gorm:"type:jsonb"`
		BuiltIn     bool
	}

	return &gormigrate.Migration{
		ID: "2026091000000002",
		Migrate: func(tx *gorm.DB) error {
			var existing Role
			if err := tx.Where("name = ?", RoleManagedClusterRegistrar).First(&existing).Error; err == nil {
				return nil
			}
			permissions := map[string]interface{}{
				"managed_clusters": []string{"register"},
			}
			permJSON, err := json.Marshal(permissions)
			if err != nil {
				return err
			}
			permStr := string(permJSON)
			displayName := "Managed Cluster Registrar"
			description := "Allows a spoke control-plane to self-register via POST /managed_clusters/registration. JWT-direct: no DB binding lifecycle."
			role := Role{
				Model:       db.Model{ID: api.NewID()},
				Name:        RoleManagedClusterRegistrar,
				DisplayName: &displayName,
				Description: &description,
				Permissions: &permStr,
				BuiltIn:     true,
			}
			return tx.Create(&role).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Where("name = ?", RoleManagedClusterRegistrar).Delete(&Role{}).Error
		},
	}
}

// migrationAddGatewayAdminRole seeds the gateway:admin built-in role
// idempotently (GTW-ACC-12). gateway:admin is a per-gateway, DB-assigned role (like
// gateway:owner and gateway:viewer); it is NOT JWT-synced. Re-running is safe,
// and no existing binding is touched.
func migrationAddGatewayAdminRole() *gormigrate.Migration {
	type Role struct {
		db.Model
		Name        string `gorm:"uniqueIndex"`
		DisplayName *string
		Description *string
		Permissions *string `gorm:"type:jsonb"`
		BuiltIn     bool
	}

	return &gormigrate.Migration{
		ID: "2026100600000004",
		Migrate: func(tx *gorm.DB) error {
			var existing Role
			if err := tx.Where("name = ?", RoleGatewayAdmin).First(&existing).Error; err == nil {
				return nil
			}
			permissions := map[string]interface{}{
				"gateways":         []string{"read", "update"},
				"role_bindings":    []string{"create", "read", "update", "delete", "list"},
				"service_accounts": []string{"create", "read", "delete", "list"},
			}
			permJSON, err := json.Marshal(permissions)
			if err != nil {
				return err
			}
			permStr := string(permJSON)
			displayName := "Gateway Admin"
			description := "Granted administrator: read/update one gateway (no delete); can grant admin and viewer to others"
			role := Role{
				Model:       db.Model{ID: api.NewID()},
				Name:        RoleGatewayAdmin,
				DisplayName: &displayName,
				Description: &description,
				Permissions: &permStr,
				BuiltIn:     true,
			}
			return tx.Create(&role).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Where("name = ?", RoleGatewayAdmin).Delete(&Role{}).Error
		},
	}
}

// migrationAddGatewayDedicatedRouteCreatorRole seeds the
// gateway:dedicated-route-creator built-in role idempotently. It is a
// platform-wide (global-scope) entitlement that gates
// GatewayCreateRequest.dedicated_route; per-gateway certificates are a rate-limited
// resource, so it is separate from gateway:creator. Like platform:admin and
// gateway:creator it is JWT-synced (assigned through the identity provider), and no
// existing binding is touched. Re-running is safe: an existing row short-circuits.
func migrationAddGatewayDedicatedRouteCreatorRole() *gormigrate.Migration {
	type Role struct {
		db.Model
		Name        string `gorm:"uniqueIndex"`
		DisplayName *string
		Description *string
		Permissions *string `gorm:"type:jsonb"`
		BuiltIn     bool
	}

	return &gormigrate.Migration{
		ID: "2026102812000002",
		Migrate: func(tx *gorm.DB) error {
			var existing Role
			lookupErr := tx.Where("name = ?", RoleGatewayDedicatedRouteCreator).First(&existing).Error
			if lookupErr == nil {
				return nil
			}
			// Only a missing row means "seed it"; a real lookup failure must not be
			// swallowed into a duplicate-key create error.
			if !stderrors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return fmt.Errorf("look up role %s: %w", RoleGatewayDedicatedRouteCreator, lookupErr)
			}
			permissions := map[string]interface{}{
				"gateways": []string{"request_dedicated_route"},
			}
			permJSON, err := json.Marshal(permissions)
			if err != nil {
				return fmt.Errorf("marshal permissions for %s: %w", RoleGatewayDedicatedRouteCreator, err)
			}
			permStr := string(permJSON)
			displayName := "Gateway Dedicated Route Creator"
			description := "Allows requesting a dedicated per-gateway route and certificate at gateway create; per-gateway certificates are a rate-limited resource."
			role := Role{
				Model:       db.Model{ID: api.NewID()},
				Name:        RoleGatewayDedicatedRouteCreator,
				DisplayName: &displayName,
				Description: &description,
				Permissions: &permStr,
				BuiltIn:     true,
			}
			return tx.Create(&role).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Where("name = ?", RoleGatewayDedicatedRouteCreator).Delete(&Role{}).Error
		},
	}
}

// migrationAddCIFederationAdminRole seeds the ci-federation:admin built-in role
// idempotently (SEC-RBAC-21). It is a platform-wide (global-scope) entitlement
// that authorizes only the ci_providers ext kind; it is deliberately narrower
// than platform:admin so the CI-federation seeder holds least privilege. Like
// platform:admin and gateway:dedicated-route-creator it is JWT-synced (assigned
// through Keycloak), and no existing binding is touched. Re-running is safe: an
// existing row short-circuits.
func migrationAddCIFederationAdminRole() *gormigrate.Migration {
	type Role struct {
		db.Model
		Name        string `gorm:"uniqueIndex"`
		DisplayName *string
		Description *string
		Permissions *string `gorm:"type:jsonb"`
		BuiltIn     bool
	}

	return &gormigrate.Migration{
		ID: "2026102912000001",
		Migrate: func(tx *gorm.DB) error {
			var existing Role
			lookupErr := tx.Where("name = ?", RoleCIFederationAdmin).First(&existing).Error
			if lookupErr == nil {
				return nil
			}
			// Only a missing row means "seed it"; a real lookup failure must not be
			// swallowed into a duplicate-key create error.
			if !stderrors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return fmt.Errorf("look up role %s: %w", RoleCIFederationAdmin, lookupErr)
			}
			permissions := map[string]interface{}{
				"ci_providers": []string{"create", "read", "update", "delete"},
			}
			permJSON, err := json.Marshal(permissions)
			if err != nil {
				return fmt.Errorf("marshal permissions for %s: %w", RoleCIFederationAdmin, err)
			}
			permStr := string(permJSON)
			displayName := "CI Federation Administrator"
			description := "Authorizes registering CI OIDC forges (ci_providers) and nothing else; a least-privilege alternative to platform:admin for the CI-federation seeder."
			role := Role{
				Model:       db.Model{ID: api.NewID()},
				Name:        RoleCIFederationAdmin,
				DisplayName: &displayName,
				Description: &description,
				Permissions: &permStr,
				BuiltIn:     true,
			}
			return tx.Create(&role).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Where("name = ?", RoleCIFederationAdmin).Delete(&Role{}).Error
		},
	}
}

func SeedRoles(ctx context.Context, dao RoleDao) error {
	for _, seed := range builtInRoleSeeds {
		_, err := dao.GetByName(ctx, seed.Name)
		if err == nil {
			continue
		}

		permJSON, jsonErr := json.Marshal(seed.Permissions)
		if jsonErr != nil {
			return fmt.Errorf("marshal permissions for %s: %w", seed.Name, jsonErr)
		}
		permStr := string(permJSON)
		displayName := seed.DisplayName
		description := seed.Description
		role := &Role{
			Meta:        api.Meta{ID: api.NewID()},
			Name:        seed.Name,
			DisplayName: &displayName,
			Description: &description,
			Permissions: &permStr,
			BuiltIn:     true,
		}
		if _, createErr := dao.Create(ctx, role); createErr != nil {
			return fmt.Errorf("seed role %s: %w", seed.Name, createErr)
		}
	}
	return nil
}
