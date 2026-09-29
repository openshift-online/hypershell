package rbac_test

import (
	"flag"
	"os"
	"runtime"
	"testing"

	"github.com/golang/glog"

	// Every Watch* service and the gateways plugin, linked as in the real
	// server (cmd/hypershell/main.go), so the RBAC post-auth interceptors
	// registered by this package run in front of them.
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gatewayNetworks"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gatewayReleases"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/rbac"
	"github.com/openshift-online/hypershell/components/api-server/test"
)

func TestMain(m *testing.M) {
	flag.Parse()
	glog.Infof("Starting rbac integration test using go version %s", runtime.Version())
	helper := test.NewHelper(&testing.T{})
	exitCode := m.Run()
	helper.Teardown()
	os.Exit(exitCode)
}
