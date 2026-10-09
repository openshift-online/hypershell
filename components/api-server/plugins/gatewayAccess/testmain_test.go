package gatewayAccess_test

import (
	"flag"
	"os"
	"runtime"
	"testing"

	"github.com/golang/glog"

	"github.com/openshift-online/hypershell/components/api-server/test"

	// Register the rbac plugin so UserProvisioningMiddleware is wired for the
	// HTTP server the helper starts.
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/rbac"
)

func TestMain(m *testing.M) {
	flag.Parse()
	glog.Infof("Starting gatewayAccess integration test using go version %s", runtime.Version())
	helper := test.NewHelper(&testing.T{})
	exitCode := m.Run()
	helper.Teardown()
	os.Exit(exitCode)
}
