package revoke

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/revoke/serviceAccount"
)

var Cmd = &cobra.Command{
	Use:   "revoke RESOURCE ID",
	Short: "Permanently revoke a resource",
}

func init() {
	Cmd.AddCommand(serviceAccount.Cmd)
}
