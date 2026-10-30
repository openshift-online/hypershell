package test

import (
	"context"
	"log/slog"
	"testing"

	gm "github.com/onsi/gomega"

	"github.com/openshift-online/hypershell/components/api-server/pkg/api/openapi"
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
}
