package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/completion"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/config"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/create"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/delete"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/get"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/list"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/login"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/logout"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/revoke"
	"github.com/openshift-online/hypershell/components/cli/cmd/hypershell/version"
)

var root = &cobra.Command{
	Use:           "hypershell",
	Short:         "hypershell CLI",
	Long:          "Command line tool for the hypershell API server.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	root.AddCommand(completion.Cmd)
	root.AddCommand(config.Cmd)
	root.AddCommand(create.Cmd)
	root.AddCommand(delete.Cmd)
	root.AddCommand(get.Cmd)
	root.AddCommand(list.Cmd)
	root.AddCommand(login.Cmd)
	root.AddCommand(logout.Cmd)
	root.AddCommand(revoke.Cmd)
	root.AddCommand(version.Cmd)
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
