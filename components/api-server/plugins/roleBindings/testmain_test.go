package roleBindings_test

import (
	"flag"
	"log/slog"
	"os"
	"runtime"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/rbac"
	"github.com/openshift-online/hypershell/components/api-server/test"

	// Register the rbac plugin so that UserProvisioningMiddleware is wired onto
	// apiV1Router for HTTP-path integration tests.
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/rbac"
)

func TestMain(m *testing.M) {
	flag.Parse()
	// Enforcement follows authentication (SEC-RBAC-14), and the test
	// environment authenticates with mocked JWT, so authorization would
	// be enforced by default. These tests exercise plugin behavior, not
	// authorization; test/authz and plugins/users run enforced.
	rbac.SetEnforcementForTest(false)
	slog.Info("Starting roleBindings integration test", "go_version", runtime.Version())
	helper := test.NewHelper(&testing.T{})
	exitCode := m.Run()
	helper.Teardown()
	os.Exit(exitCode)
}
