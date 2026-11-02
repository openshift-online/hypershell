package rbac_test

import (
	"flag"
	"log/slog"
	"os"
	"runtime"
	"testing"

	// The gateways plugin and RBAC plugin, linked as in the real server
	// (cmd/hypershell/main.go), so the RBAC post-auth interceptors run in
	// front of them.
	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/rbac"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

func TestMain(m *testing.M) {
	flag.Parse()
	// Enforcement follows authentication (SEC-RBAC-14), and the test
	// environment authenticates with mocked JWT, so authorization would
	// be enforced by default. These tests exercise plugin behavior, not
	// authorization; test/authz and plugins/users run enforced.
	rbac.SetEnforcementForTest(false)
	slog.Info("Starting rbac integration test", "go_version", runtime.Version())
	helper := test.NewHelper(&testing.T{})
	exitCode := m.Run()
	helper.Teardown()
	os.Exit(exitCode)
}
