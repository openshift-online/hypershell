package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/apply"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create"
	creategatewayaccess "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/gatewayAccess"
	createserviceaccount "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/serviceAccount"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete"
	deletegatewayaccess "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete/gatewayAccess"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/extgroup"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get"
	getserviceaccount "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/serviceAccount"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list"
	listgatewayaccess "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/gatewayAccess"
	listserviceaccounts "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/serviceAccounts"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/revoke"
	tuicmd "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/tui"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/update"
	updategatewayaccess "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/update/gatewayAccess"
)

var root = &cobra.Command{
	Use:           "hsctl",
	Short:         "hsctl CLI",
	Long:          "Command line tool for the HyperShell API server.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	// Service account commands are hand-maintained; the generated parents do not know them.
	create.Cmd.AddCommand(createserviceaccount.Cmd)
	get.Cmd.AddCommand(getserviceaccount.Cmd)
	list.Cmd.AddCommand(listserviceaccounts.Cmd)

	// Gateway access is a scoped sub-collection (/gateways/{id}/access), like
	// service accounts, so its commands are hand-maintained too.
	create.Cmd.AddCommand(creategatewayaccess.Cmd)
	list.Cmd.AddCommand(listgatewayaccess.Cmd)
	update.Cmd.AddCommand(updategatewayaccess.Cmd)
	delete.Cmd.AddCommand(deletegatewayaccess.Cmd)

	// Split the help of each parent into core and extension sections. Every command,
	// generated or hand-written, must be registered before this.
	extgroup.Apply(create.Cmd, delete.Cmd, get.Cmd, list.Cmd)

	// Generated commands: completion, config, create, delete, get, list, login, logout,
	// update, version and whoami.
	addGeneratedCommands(root)

	// Hand-written commands.
	root.AddCommand(apply.Cmd)
	root.AddCommand(revoke.Cmd)
	root.AddCommand(tuicmd.Cmd)
}

func main() {
	root.SetArgs(os.Args[1:])
	err := root.Execute()
	if err == nil {
		os.Exit(0)
	}

	fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	os.Exit(1)
}
