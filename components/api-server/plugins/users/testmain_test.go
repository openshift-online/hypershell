package users_test

import (
	"flag"
	"os"
	"runtime"
	"testing"

	"github.com/golang/glog"

	_ "github.com/openshift-online/hypershell/components/api-server/plugins/rbac"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

func TestMain(m *testing.M) {
	flag.Parse()
	_ = os.Setenv("API_ENV", "integration_testing")
	_ = os.Setenv("DB_FACTORY_MODE", "external")
	_ = os.Setenv("RBAC_ENFORCE", "true")
	glog.Infof("Starting users integration test using go version %s", runtime.Version())
	helper := test.NewHelper(&testing.T{})
	exitCode := m.Run()
	helper.Teardown()
	os.Exit(exitCode)
}
