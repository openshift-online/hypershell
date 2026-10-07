package list

import (
	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/agentRuntimes"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/gateways"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/inferenceRoutes"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/managedClusters"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/providerBindings"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/providerSpecs"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/roleBindings"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/roles"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/sandboxTemplates"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/secretSources"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/serviceAccounts"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/users"
)

const groupCore = "core"
const groupExt = "ext"

var Cmd = &cobra.Command{
	Use:   "list RESOURCE",
	Short: "List all resources of a specific type",
	Long:  "List all resources of a specific type",
}

func init() {
	Cmd.AddGroup(
		&cobra.Group{ID: groupCore, Title: "Core (hypershell/v1/):"},
		&cobra.Group{ID: groupExt, Title: "Extensions (hypershell/ext/) - experimental, unsupported, subject to change:"},
	)

	for _, c := range []*cobra.Command{gateways.Cmd, managedClusters.Cmd, roles.Cmd, roleBindings.Cmd, users.Cmd, serviceAccounts.Cmd} {
		c.GroupID = groupCore
		Cmd.AddCommand(c)
	}
	for _, c := range []*cobra.Command{agentRuntimes.Cmd, sandboxTemplates.Cmd, providerSpecs.Cmd, providerBindings.Cmd, inferenceRoutes.Cmd, secretSources.Cmd} {
		c.GroupID = groupExt
		Cmd.AddCommand(c)
	}
}
