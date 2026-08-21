package get

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/get/fleet"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/get/gateway"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/get/gatewayNetwork"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/get/gatewayRelease"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/get/managedCluster"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/get/managedDatabase"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/get/role"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/get/roleBinding"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/get/serviceAccount"
)

var Cmd = &cobra.Command{
	Use:   "get RESOURCE ID",
	Short: "Get a specific resource by ID",
	Long:  "Get a specific resource by ID",
}

func init() {
	Cmd.AddCommand(fleet.Cmd)
	Cmd.AddCommand(gateway.Cmd)
	Cmd.AddCommand(gatewayNetwork.Cmd)
	Cmd.AddCommand(gatewayRelease.Cmd)
	Cmd.AddCommand(managedCluster.Cmd)
	Cmd.AddCommand(managedDatabase.Cmd)
	Cmd.AddCommand(role.Cmd)
	Cmd.AddCommand(roleBinding.Cmd)
	Cmd.AddCommand(serviceAccount.Cmd)
}
