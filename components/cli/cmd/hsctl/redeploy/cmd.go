package redeploy

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/redeploy/gateway"
)

// Cmd is the hand-written redeploy verb. Like revoke it is an action on an
// existing resource, not generated CRUD.
var Cmd = &cobra.Command{
	Use:   "redeploy RESOURCE ID",
	Short: "Redeploy an existing resource",
}

func init() {
	Cmd.AddCommand(gateway.Cmd)
}
