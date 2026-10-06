package update

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/update/gatewayAccess"
)

var Cmd = &cobra.Command{
	Use:   "update RESOURCE",
	Short: "Update a resource",
	Long:  "Update a resource",
}

func init() {
	Cmd.AddCommand(gatewayAccess.Cmd)
}
