package delete

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/fleet"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/gateway"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/gatewayNetwork"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/gatewayRelease"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/managedCluster"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/managedDatabase"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/role"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/roleBinding"
)

var Cmd = &cobra.Command{
	Use:   "delete RESOURCE ID",
	Short: "Delete a resource",
	Long:  "Delete a resource by ID",
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
