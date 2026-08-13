package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/apply"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/completion"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/config"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/create"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/delete"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/get"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/list"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/login"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/logout"
	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/version"
)

var root = &cobra.Command{
	Use:           "hsctl",
	Short:         "hsctl CLI",
	Long:          "Command line tool for the HyperShell API server.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	root.AddCommand(apply.Cmd)
	root.AddCommand(completion.Cmd)
	root.AddCommand(config.Cmd)
	root.AddCommand(create.Cmd)
	root.AddCommand(delete.Cmd)
	root.AddCommand(get.Cmd)
	root.AddCommand(list.Cmd)
	root.AddCommand(login.Cmd)
	root.AddCommand(logout.Cmd)
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
