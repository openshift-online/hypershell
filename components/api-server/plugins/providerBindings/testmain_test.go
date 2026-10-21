package providerBindings_test

import (
	"flag"
	"log/slog"
	"os"
	"runtime"
	"testing"

	_ "github.com/openshift-online/hypershell/components/api-server/plugins/providerBindings"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

func TestMain(m *testing.M) {
	flag.Parse()
	slog.Info("Starting providerBindings integration test", "go_version", runtime.Version())
	helper := test.NewHelper(&testing.T{})
	exitCode := m.Run()
	helper.Teardown()
	os.Exit(exitCode)
}
