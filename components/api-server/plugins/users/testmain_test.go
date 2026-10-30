package users_test

import (
	"flag"
	"log/slog"
	"os"
	"runtime"
	"testing"

	_ "github.com/openshift-online/hypershell/components/api-server/plugins/rbac"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

func TestMain(m *testing.M) {
	flag.Parse()
	// Enforcement needs no opt-in: it follows the mocked JWT authentication
	// the helper enables (SEC-RBAC-14).
	slog.Info("Starting users integration test", "go_version", runtime.Version())
	helper := test.NewHelper(&testing.T{})
	exitCode := m.Run()
	helper.Teardown()
	os.Exit(exitCode)
}
