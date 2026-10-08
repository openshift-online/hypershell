package authz_test

import (
	"flag"
	"os"
	"runtime"
	"testing"

	"github.com/golang/glog"

	// Link every plugin the real server links (cmd/hypershell/main.go) so REST
	// routes, gRPC services, and the RBAC middleware and interceptors all run.
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/rbac"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/users"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

// allowlistedAccount is the RBAC_SERVICE_ACCOUNTS entry these tests configure:
// the bootstrap allowlist a hub ships with for its co-located control plane.
const allowlistedAccount = "service-account-authz-allowlisted"

// TestMain runs the api-server the way the GitOps hubs run it: RBAC enforced,
// RBAC_DEFAULT_ROLES unset (so every authenticated principal holds
// gateway:creator, as the deployed realm's default role also grants), and a
// one-entry control-plane allowlist. The environment must be set before the
// helper starts the server, because the RBAC middleware reads it at startup.
func TestMain(m *testing.M) {
	flag.Parse()
	_ = os.Setenv("RBAC_ENFORCE", "true")
	_ = os.Unsetenv("RBAC_DEFAULT_ROLES")
	_ = os.Setenv("RBAC_SERVICE_ACCOUNTS", allowlistedAccount)
	glog.Infof("Starting authz integration test using go version %s", runtime.Version())
	helper := test.NewHelper(&testing.T{})
	exitCode := m.Run()
	helper.Teardown()
	os.Exit(exitCode)
}
