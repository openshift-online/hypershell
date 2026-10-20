package update

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/update/gateway"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/update/managedCluster"
)

var Cmd = &cobra.Command{
	Use:   "update RESOURCE ID",
	Short: "Update a resource by ID",
	Long:  "Update a resource by ID, sending only the fields that were set.",
}

func init() {
	Cmd.AddCommand(gateway.Cmd)
	Cmd.AddCommand(managedCluster.Cmd)
}
