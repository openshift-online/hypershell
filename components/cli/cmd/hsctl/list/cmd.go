package list

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/fleets"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/gatewayNetworks"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/gatewayReleases"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/gateways"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/managedClusters"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/managedDatabases"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/roleBindings"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/roles"
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
