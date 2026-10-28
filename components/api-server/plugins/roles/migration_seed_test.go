package roles

import (
	"context"
	"strings"
	"testing"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

// TestDedicatedRouteCreatorRoleSeedIsIdempotent proves the new built-in role
// migration (2026102812000002) creates gateway:dedicated-route-creator with the
// expected built-in properties and that re-running it is a no-op: an existing row
// short-circuits, so the migration is safe on every environment, including one
// where the runtime seed already inserted the role.
//
// It lives in the internal test package to reach the unexported migration, and uses
// plain assertions because importing the shared test helper would close an import
// cycle (the helper imports this plugin). The environment and its database are
// initialized by the package TestMain.
func TestDedicatedRouteCreatorRoleSeedIsIdempotent(t *testing.T) {
	env := environments.Environment()
	if env == nil || env.Database.SessionFactory == nil {
		t.Fatal("test environment is not initialized")
	}
	session := env.Database.SessionFactory.New(context.Background())

	// The migration ran at startup, so clear the row first: that exercises the
	// create path here and the idempotent path on the second run. The delete is
	// unscoped because the unique name index counts soft-deleted rows too.
	if err := session.Unscoped().Where("name = ?", RoleGatewayDedicatedRouteCreator).Delete(&Role{}).Error; err != nil {
		t.Fatalf("clear the seeded role: %s", err)
	}

	migration := migrationAddGatewayDedicatedRouteCreatorRole()
	if err := migration.Migrate(session); err != nil {
		t.Fatalf("first migration run: %s", err)
	}
	if err := migration.Migrate(session); err != nil {
		t.Fatalf("second migration run must be a no-op: %s", err)
	}

	var seeded []Role
	if err := session.Where("name = ?", RoleGatewayDedicatedRouteCreator).Find(&seeded).Error; err != nil {
		t.Fatalf("read the seeded role: %s", err)
	}
	if len(seeded) != 1 {
		t.Fatalf("seeded role rows = %d, want exactly 1", len(seeded))
	}
	role := seeded[0]
	if !role.BuiltIn {
		t.Error("seeded role must be built-in")
	}
	if role.DisplayName == nil || *role.DisplayName == "" {
		t.Error("seeded role must have a display name")
	}
	if role.Description == nil ||
		!strings.Contains(*role.Description, "dedicated per-gateway route") ||
		!strings.Contains(*role.Description, "rate-limited") {
		t.Errorf("seeded role description must explain the entitlement and the rate-limit rationale, got %v", role.Description)
	}
	if role.Permissions == nil || !strings.Contains(*role.Permissions, "request_dedicated_route") {
		t.Errorf("seeded role permissions must carry the marker permission, got %v", role.Permissions)
	}

	// The runtime seed list covers the same role, so an environment that seeds
	// through SeedRoles (a fresh database) ends in the same state.
	found := false
	for _, seed := range builtInRoleSeeds {
		if seed.Name == RoleGatewayDedicatedRouteCreator {
			found = true
		}
	}
	if !found {
		t.Errorf("builtInRoleSeeds must include %s", RoleGatewayDedicatedRouteCreator)
	}

	// The entitlement is JWT-synced (assigned through the identity provider) for
	// the same reason platform:admin and gateway:creator are: its bindings are
	// reconciled from the caller's realm roles.
	if !JWTSyncedRoles[RoleGatewayDedicatedRouteCreator] {
		t.Errorf("%s must be JWT-synced", RoleGatewayDedicatedRouteCreator)
	}
}
