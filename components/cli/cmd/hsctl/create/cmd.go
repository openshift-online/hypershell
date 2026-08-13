package create

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/fleet"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/gateway"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/gatewayNetwork"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/gatewayRelease"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/managedCluster"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/managedDatabase"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/role"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/roleBinding"
)

var Cmd = &cobra.Command{
	Use:   "create RESOURCE",
	Short: "Create a resource",
	Long:  "Create a resource",
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
