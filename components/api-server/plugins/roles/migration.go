package roles

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openshift-online/rh-trex-ai/pkg/api"
	"github.com/openshift-online/rh-trex-ai/pkg/db"
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
