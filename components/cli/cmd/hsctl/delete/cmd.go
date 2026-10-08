package delete

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/gateway"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/managedCluster"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/roleBinding"
)

var Cmd = &cobra.Command{
	Use:   "delete RESOURCE ID",
	Short: "Delete a resource by ID",
	Long:  "Delete a resource by ID after asking for confirmation. Pass --yes to skip the prompt.",
}

func init() {
	Cmd.AddCommand(gateway.Cmd)
	Cmd.AddCommand(managedCluster.Cmd)
	Cmd.AddCommand(roleBinding.Cmd)
}
