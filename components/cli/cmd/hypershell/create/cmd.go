package create

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/create/fleet"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/create/gateway"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/create/gatewayNetwork"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/create/gatewayRelease"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/create/managedCluster"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/create/managedDatabase"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/create/role"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/create/roleBinding"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/create/serviceAccount"
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
	Cmd.AddCommand(serviceAccount.Cmd)
}
