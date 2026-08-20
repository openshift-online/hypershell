package main

import (
	"github.com/golang/glog"

	localapi "github.com/openshift-online/hypershell/components/api-server/pkg/api"
	pkgcmd "github.com/openshift-online/rh-trex-ai/pkg/cmd"

	_ "github.com/openshift-online/hypershell/components/api-server/cmd/hypershell/environments"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/fleets"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gatewayNetworks"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gatewayReleases"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/gateways"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/managedClusters"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/managedDatabases"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/otel"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/rbac"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/roleBindings"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/roles"
	_ "github.com/openshift-online/hypershell/components/api-server/plugins/users"
	_ "github.com/openshift-online/rh-trex-ai/plugins/events"
	_ "github.com/openshift-online/rh-trex-ai/plugins/generic"
)

func main() {
	rootCmd := pkgcmd.NewRootCommand("hypershell", "My service built with TRex library")
	rootCmd.AddCommand(
		pkgcmd.NewMigrateCommand("hypershell"),
		pkgcmd.NewServeCommand(localapi.GetOpenAPISpec),
	)

	if err := rootCmd.Execute(); err != nil {
		glog.Fatalf("error running command: %v", err)
	}
}
