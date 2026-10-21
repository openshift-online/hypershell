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
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/rbac"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

func TestMain(m *testing.M) {
	flag.Parse()
	slog.Info("Starting rbac integration test", "go_version", runtime.Version())
	helper := test.NewHelper(&testing.T{})
	exitCode := m.Run()
	helper.Teardown()
	os.Exit(exitCode)
}
