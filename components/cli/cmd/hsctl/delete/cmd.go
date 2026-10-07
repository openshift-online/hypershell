package delete

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/agentRuntime"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/gateway"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/gatewayAccess"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/inferenceRoute"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/managedCluster"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/providerBinding"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/providerSpec"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/role"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/roleBinding"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/sandboxTemplate"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/secretSource"
)

const groupCore = "core"
const groupExt = "ext"

var Cmd = &cobra.Command{
	Use:   "delete RESOURCE ID",
	Short: "Delete a resource",
	Long:  "Delete a resource by ID",
}

func init() {
	Cmd.AddGroup(
		&cobra.Group{ID: groupCore, Title: "Core (hypershell/v1/):"},
		&cobra.Group{ID: groupExt, Title: "Extensions (hypershell/ext/) - experimental, unsupported, subject to change:"},
	)

	for _, c := range []*cobra.Command{gateway.Cmd, gatewayAccess.Cmd, managedCluster.Cmd, role.Cmd, roleBinding.Cmd} {
		c.GroupID = groupCore
		Cmd.AddCommand(c)
	}
	for _, c := range []*cobra.Command{agentRuntime.Cmd, sandboxTemplate.Cmd, providerSpec.Cmd, providerBinding.Cmd, inferenceRoute.Cmd, secretSource.Cmd} {
		c.GroupID = groupExt
		Cmd.AddCommand(c)
	}
}
