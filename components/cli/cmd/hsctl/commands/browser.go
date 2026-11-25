package commands

import (
	"encoding/csv"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// InstallPage is the console page that explains how to install hsctl.
const InstallPage = "/docs/cli/install-hsctl"

// LocalOnlyCommands are the commands that need a local machine: they log in or
// out, read the saved login or configuration, write shell completion scripts,
// take over a terminal, or read only local files or standard input (CLI-WASM-04).
// Paths leave out "hsctl"; a listed command makes its subcommands local-only too.
var LocalOnlyCommands = []string{
	"login",
	"logout",
	"whoami",
	"config",
	"completion",
	"tui",
	// apply needs -f or -k: a local file, directory, kustomization or stdin.
	"apply",
	// upload needs --value-file or --value-stdin (CLI-EXT-05).
	"secret-source upload",
}

// signInCommands are the local-only commands that the console sign-in replaces.
var signInCommands = map[string]bool{"login": true, "logout": true, "whoami": true}

// LocalOnlyFlags are the flags that read or write a local file or read standard
// input. A browser command fails when one of them is set (CLI-WASM-04). The
// value maps to the one flag value that stays in the browser, if any.
var LocalOnlyFlags = map[string]string{
	"body":        "",  // create and update: request body file
	"cedar-file":  "",  // platform gateway policies: Cedar file or '-' for stdin
	"filename":    "",  // apply -f
	"kustomize":   "",  // apply -k
	"value-file":  "",  // secret-source upload
	"value-stdin": "",  // secret-source upload
	"token-file":  "",  // login
	"output-file": "-", // create serviceAccount: a new file; '-' is stdout
}

// NewBrowserRoot builds the browser module's command tree: the shared commands,
// with the local-only commands and flags answering that they need the installed
// hsctl (CLI-WASM-04). It changes the shared command singletons, so a process
// that calls it must not run the native tree afterwards.
func NewBrowserRoot() *cobra.Command {
	root := newRootCommand()
	addSharedCommands(root)
	root.AddCommand(newTUIStub())
	rejectUnknownSubcommands(root)
	applyLocalOnly(root)
	return root
}

// newTUIStub stands in for hsctl tui (CLI-TUI-01), which links a terminal UI
// library the browser module must not include. Use and Short match the native
// command so the command list in help is the same.
func newTUIStub() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Browse and operate the service from a terminal UI",
		Long:  "Browse and operate the service through its OpenAPI-derived terminal UI.",
	}
}

// IsLocalOnlyPath reports whether a command path (without "hsctl") is a
// local-only command or below one.
func IsLocalOnlyPath(path string) bool {
	return localOnlyTop(path) != ""
}

func localOnlyTop(path string) string {
	for _, top := range LocalOnlyCommands {
		if path == top || strings.HasPrefix(path, top+" ") {
			return top
		}
	}
	return ""
}

// relativePath is the command path without the root command name.
func relativePath(cmd *cobra.Command) string {
	path := cmd.CommandPath()
	if i := strings.IndexByte(path, ' '); i >= 0 {
		return path[i+1:]
	}
	return ""
}

func applyLocalOnly(cmd *cobra.Command) {
	for _, c := range cmd.Commands() {
		applyLocalOnly(c)
	}
	if cmd.Parent() == nil {
		return
	}
	if top := localOnlyTop(relativePath(cmd)); top != "" {
		makeLocalOnly(cmd, top)
		return
	}
	if cmd.Runnable() {
		guardLocalOnlyFlags(cmd)
	}
}

// makeLocalOnly turns a command into one that only reports that it needs the
// installed hsctl. Help still works: cobra answers --help before running it.
func makeLocalOnly(cmd *cobra.Command, top string) {
	cmd.Args = cobra.ArbitraryArgs
	cmd.FParseErrWhitelist.UnknownFlags = true
	cmd.PersistentPreRun, cmd.PersistentPreRunE = nil, nil
	cmd.PreRun, cmd.PreRunE = nil, nil
	cmd.PostRun, cmd.PostRunE = nil, nil
	cmd.PersistentPostRun, cmd.PersistentPostRunE = nil, nil
	cmd.Run = nil
	cmd.RunE = func(*cobra.Command, []string) error {
		return localOnlyCommandError(top)
	}
	// Required and grouped flags would fail before the command runs, with an
	// error that does not say why the command cannot run here.
	clearGroups := func(f *pflag.Flag) {
		for key := range f.Annotations {
			if strings.HasPrefix(key, "cobra_annotation_") {
				delete(f.Annotations, key)
			}
		}
	}
	cmd.Flags().VisitAll(clearGroups)
	cmd.PersistentFlags().VisitAll(clearGroups)
}

func localOnlyCommandError(top string) error {
	if signInCommands[top] {
		return fmt.Errorf("hsctl %s needs the installed hsctl: the browser terminal uses the console sign-in, "+
			"so it has no saved login; install hsctl from the console page %s to run it locally", top, InstallPage)
	}
	return fmt.Errorf("hsctl %s needs the installed hsctl: it uses the local machine, which the browser terminal "+
		"cannot reach; install hsctl from the console page %s to run it locally", top, InstallPage)
}

// guardLocalOnlyFlags makes a command fail before it runs when a local-only flag
// is set.
func guardLocalOnlyFlags(cmd *cobra.Command) {
	preE, pre := cmd.PreRunE, cmd.PreRun
	cmd.PreRun = nil
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if err := localOnlyFlagError(c); err != nil {
			return err
		}
		if preE != nil {
			return preE(c, args)
		}
		if pre != nil {
			pre(c, args)
		}
		return nil
	}
}

func localOnlyFlagError(cmd *cobra.Command) error {
	var err error
	cmd.Flags().Visit(func(f *pflag.Flag) {
		allowed, local := LocalOnlyFlags[f.Name]
		if !local || err != nil || (allowed != "" && f.Value.String() == allowed) {
			return
		}
		err = fmt.Errorf("%s --%s needs the installed hsctl: it reads or writes a local file or standard input, "+
			"which the browser terminal cannot reach; install hsctl from the console page %s to run it locally",
			cmd.CommandPath(), f.Name, InstallPage)
	})
	return err
}

// ResetFlags restores every flag in the tree to its default and marks it unset,
// so a flag given to one run of a package-level command does not leak into the
// next run (CLI-WASM-02).
func ResetFlags(cmd *cobra.Command) {
	reset := func(f *pflag.Flag) {
		if slice, ok := f.Value.(pflag.SliceValue); ok {
			_ = slice.Replace(sliceDefault(f.DefValue))
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	}
	cmd.Flags().VisitAll(reset)
	cmd.PersistentFlags().VisitAll(reset)
	for _, sub := range cmd.Commands() {
		ResetFlags(sub)
	}
}

// sliceDefault parses the "[a,b]" default pflag records for slice flags.
func sliceDefault(def string) []string {
	def = strings.TrimSuffix(strings.TrimPrefix(def, "["), "]")
	if def == "" {
		return []string{}
	}
	values, err := csv.NewReader(strings.NewReader(def)).Read()
	if err != nil {
		return []string{def}
	}
	return values
}
