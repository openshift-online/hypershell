package delete

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/delete/gateway"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/delete/gatewayNetwork"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/delete/gatewayProfile"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/delete/gatewayRelease"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/delete/managedCluster"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/delete/managedDatabase"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/delete/role"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/delete/roleBinding"
)

var Cmd = &cobra.Command{
	Use:   "delete RESOURCE ID",
	Short: "Delete a resource",
	Long:  "Delete a resource by ID",
}

func init() {
	Cmd.AddCommand(gateway.Cmd)
	Cmd.AddCommand(gatewayNetwork.Cmd)
	Cmd.AddCommand(gatewayProfile.Cmd)
	Cmd.AddCommand(gatewayRelease.Cmd)
	Cmd.AddCommand(managedCluster.Cmd)
	Cmd.AddCommand(managedDatabase.Cmd)
	Cmd.AddCommand(role.Cmd)
	Cmd.AddCommand(roleBinding.Cmd)
}
