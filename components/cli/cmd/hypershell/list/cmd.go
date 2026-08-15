package list

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/list/fleets"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/list/gateways"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/list/gatewayNetworks"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/list/gatewayReleases"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/list/managedClusters"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/list/managedDatabases"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/list/roles"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/list/roleBindings"
)

var Cmd = &cobra.Command{
	Use:   "list RESOURCE",
	Short: "List all resources of a specific type",
	Long:  "List all resources of a specific type",
}

func init() {
	Cmd.AddCommand(fleets.Cmd)
	Cmd.AddCommand(gateways.Cmd)
	Cmd.AddCommand(gatewayNetworks.Cmd)
	Cmd.AddCommand(gatewayReleases.Cmd)
	Cmd.AddCommand(managedClusters.Cmd)
	Cmd.AddCommand(managedDatabases.Cmd)
	Cmd.AddCommand(roles.Cmd)
	Cmd.AddCommand(roleBindings.Cmd)
}
