package get

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/fleet"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/gateway"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/gatewayNetwork"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/gatewayRelease"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/managedCluster"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/managedDatabase"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/role"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/roleBinding"
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
}
