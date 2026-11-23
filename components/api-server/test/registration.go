package test

import (
	"context"
	"log/slog"
	"testing"

	gm "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
	"github.com/openshift-online/hypershell/components/api-server/plugins/platformGatewayPolicies"
	"github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/environments"
)

func RegisterIntegration(t *testing.T) (*Helper, *openapi.APIClient) {
	gm.RegisterTestingT(t)
	helper := NewHelper(t)
	helper.DBFactory.ResetDB()
	seedBuiltInRoles()
	client := helper.NewApiClient()

	return helper, client
}

func seedBuiltInRoles() {
	env := environments.Environment()
	if env == nil {
		return
	}
	roleDao := roles.NewRoleDao(&env.Database.SessionFactory)
	if err := roles.SeedRoles(context.Background(), roleDao); err != nil {
		slog.Warn("failed to seed built-in roles after ResetDB", "error", err)
	}
	// The built-in e2e enforcement canary (PLAT-PGP-20) is a migration seed;
	// ResetDB truncates it, so re-seed it like the built-in roles.
	if err := platformGatewayPolicies.SeedCanary(env.Database.SessionFactory.New(context.Background())); err != nil {
		slog.Warn("failed to seed the built-in canary policy after ResetDB", "error", err)
	}
}
