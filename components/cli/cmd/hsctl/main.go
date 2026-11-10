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
	updateserviceaccount "github.com/openshift-online/hypershell/components/cli/cmd/hsctl/update/serviceAccount"
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
	update.Cmd.AddCommand(updateserviceaccount.Cmd)

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

	// Resource-dispatcher commands (get, list, create, delete, update) have no
	// Run of their own, so a mistyped resource name (e.g. "get gatway ID") is
	// left unconsumed instead of being rejected: cobra's built-in
	// unknown-command check only fires at the root command. Left alone, the
	// dispatcher either prints its own help with exit 0, or - if a flag meant
	// for the mistyped subcommand was also passed - fails with a confusing
	// "unknown flag" error instead of naming the real problem.
	rejectUnknownSubcommands(root)
}

// rejectUnknownSubcommands makes every non-leaf command without its own Run
// reject a mistyped child command name, instead of silently falling through
// to the parent.
func rejectUnknownSubcommands(cmd *cobra.Command) {
	for _, c := range cmd.Commands() {
		rejectUnknownSubcommands(c)
	}
	if !cmd.HasSubCommands() || cmd.Runnable() || cmd.Args != nil {
		return
	}
	cmd.Args = func(c *cobra.Command, args []string) error {
		fmt.Fprintf(os.Stderr, "DEBUG NoArgs called for %s with args=%v\n", c.CommandPath(), args)
		return cobra.NoArgs(c, args)
	}
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		if args := c.Flags().Args(); len(args) > 0 {
			return fmt.Errorf("unknown command %q for %q", args[0], c.CommandPath())
		}
		return err
	})
}

func main() {
	root.SetArgs(os.Args[1:])
	if foundCmd, remaining, ferr := root.Find(os.Args[1:]); true {
		fmt.Fprintf(os.Stderr, "DEBUG Find -> cmd=%s remaining=%v err=%v argsField=%v\n", foundCmd.CommandPath(), remaining, ferr, foundCmd.Args != nil)
	}
	err := root.Execute()
	if err == nil {
		os.Exit(0)
	}

	fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	os.Exit(1)
}
