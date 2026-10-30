package authz_test

import (
	"flag"
	"log/slog"
	"os"
	"runtime"
	"testing"

	// Link every plugin the real server links (cmd/hypershell/main.go) so REST
	// routes, gRPC services, and the RBAC middleware and interceptors all run.
	// The /ext plugin set matters as much as the /v1 one: the ext regression
	// tests exercise the Extension API kinds' middleware gate and per-gateway
	// tenancy.
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/agentRuntimes"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/agentWorkspaces"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gatewayAccess"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/inferenceRoutes"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/providerBindings"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/providerSpecs"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/rbac"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/repositories"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/sandboxTemplates"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/secretSources"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/serviceAccounts"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/users"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/workspaceMemberships"
	"github.com/openshift-online/hypershell/components/api-server/test"
	_ "github.com/openshift-online/rh-trex-ai/components/api-server/plugins/events"
	_ "github.com/openshift-online/rh-trex-ai/components/api-server/plugins/generic"
)

// allowlistedAccount is the RBAC_SERVICE_ACCOUNTS entry these tests configure:
// the bootstrap allowlist a hub ships with for its co-located control plane.
const allowlistedAccount = "service-account-authz-allowlisted"

// TestMain runs the api-server the way the GitOps hubs run it: RBAC enforced
// and a one-entry control-plane allowlist. The API server merges no role of
// its own (SEC-RBAC-13); every test principal that needs gateway:creator
// carries it as a realm role on its JWT via newPrincipal, simulating the
// hub's Keycloak realm default role (default-roles-hypershell already
// includes gateway:creator). The environment must be set before the helper
// starts the server, because the RBAC middleware reads it at startup.
func TestMain(m *testing.M) {
	flag.Parse()
	_ = os.Setenv("RBAC_ENFORCE", "true")
	_ = os.Setenv("RBAC_SERVICE_ACCOUNTS", allowlistedAccount)
	// The ext regression tests exercise secret value submission; the in-memory
	// store lets those writes succeed without an external secret manager.
	_ = os.Setenv("HYPERSHELL_SECRET_STORE_BACKEND", "memory")
	slog.Info("Starting authz integration test", "go_version", runtime.Version())
	helper := test.NewHelper(&testing.T{})
	exitCode := m.Run()
	helper.Teardown()
	os.Exit(exitCode)
}
