package create

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/agentRuntime"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/gateway"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/inferenceRoute"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/managedCluster"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/providerBinding"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/providerSpec"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/role"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/roleBinding"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/sandboxTemplate"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/secretSource"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/serviceAccount"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/user"
)

const groupCore = "core"
const groupExt = "ext"

var Cmd = &cobra.Command{
	Use:   "create RESOURCE",
	Short: "Create a resource",
	Long:  "Create a resource",
}

func init() {
	Cmd.AddGroup(
		&cobra.Group{ID: groupCore, Title: "Core (hypershell/v1/):"},
		&cobra.Group{ID: groupExt, Title: "Extensions (hypershell/ext/) - experimental, unsupported, subject to change:"},
	)

	for _, c := range []*cobra.Command{gateway.Cmd, managedCluster.Cmd, role.Cmd, roleBinding.Cmd, user.Cmd, serviceAccount.Cmd} {
		c.GroupID = groupCore
		Cmd.AddCommand(c)
	}
	for _, c := range []*cobra.Command{agentRuntime.Cmd, sandboxTemplate.Cmd, providerSpec.Cmd, providerBinding.Cmd, inferenceRoute.Cmd, secretSource.Cmd} {
		c.GroupID = groupExt
		Cmd.AddCommand(c)
	}
}
