package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/apply"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create"
	createserviceaccount "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create/serviceAccount"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/extgroup"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get"
	getserviceaccount "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get/serviceAccount"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list"
	listserviceaccounts "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list/serviceAccounts"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/revoke"
	tuicmd "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/tui"
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
