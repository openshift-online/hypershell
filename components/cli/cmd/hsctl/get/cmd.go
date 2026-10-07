package get

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/agentRuntime"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/gateway"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/inferenceRoute"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/managedCluster"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/providerBinding"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/providerSpec"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/role"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/roleBinding"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/sandboxTemplate"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/secretSource"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/serviceAccount"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/user"
)

const groupCore = "core"
const groupExt = "ext"

var Cmd = &cobra.Command{
	Use:   "get RESOURCE ID",
	Short: "Get a specific resource by ID",
	Long:  "Get a specific resource by ID",
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
