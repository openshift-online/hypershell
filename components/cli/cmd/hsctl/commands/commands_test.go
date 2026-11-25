package commands

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func leaf(use, short string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, RunE: func(*cobra.Command, []string) error { return nil }}
}

func tree(children ...*cobra.Command) *cobra.Command {
	root := newRootCommand()
	root.AddCommand(children...)
	return root
}

// Verifies: CLI-WASM-06.01
func TestTreeDriftNamesANativeOnlyCommand(t *testing.T) {
	list := &cobra.Command{Use: "list"}
	list.AddCommand(leaf("gateways", "List gateways"))
	native := Describe(tree(list, leaf("frobnicate", "Native only")))

	list2 := &cobra.Command{Use: "list"}
	list2.AddCommand(leaf("gateways", "List gateways"))
	browser := Describe(tree(list2))

	drift := TreeDrift(native, browser)
	found := false
	for _, d := range drift {
		if strings.Contains(d, "hsctl frobnicate is in the native command tree but not in the browser module") {
			found = true
		}
	}
	if !found {
		t.Errorf("drift = %q, want a message naming hsctl frobnicate", drift)
	}
}

// Verifies: CLI-WASM-06, CLI-WASM-01.01
func TestTreeDriftFindsFlagAndHelpDifferences(t *testing.T) {
	nativeGateways := leaf("gateways", "List gateways")
	nativeGateways.Flags().String("search", "", "Search filter expression.")
	browserGateways := leaf("gateways", "List gateways")

	drift := TreeDrift(Describe(tree(nativeGateways)), Describe(tree(browserGateways)))
	if len(drift) == 0 || !strings.Contains(strings.Join(drift, "\n"), "hsctl gateways differs") {
		t.Errorf("drift = %q, want hsctl gateways named for its missing --search flag", drift)
	}

	drift = TreeDrift(Describe(tree(leaf("version", "Prints the version"))), Describe(tree(leaf("version", "Prints it"))))
	if len(drift) == 0 || !strings.Contains(strings.Join(drift, "\n"), "hsctl version differs") {
		t.Errorf("drift = %q, want hsctl version named for its different help", drift)
	}
}

// Verifies: CLI-WASM-06, CLI-WASM-04
func TestTreeDriftAllowsLocalOnlyDifferences(t *testing.T) {
	nativeTUI := leaf("tui", "Browse and operate the service from a terminal UI")
	nativeTUI.Flags().String("theme", "", "Theme.")
	nativeLogin := leaf("login", "Log in to the API server")
	nativeLogin.Flags().String("token-file", "", "File.")

	native := Describe(tree(nativeTUI, nativeLogin))
	browser := Describe(tree(leaf("tui", "Browse and operate the service from a terminal UI"), leaf("login", "Log in to the API server")))
	if drift := TreeDrift(native, browser); len(drift) != 0 {
		t.Errorf("drift = %q, want none for local-only commands", drift)
	}
}

// Verifies: CLI-WASM-02.01
func TestResetFlagsRestoresDefaults(t *testing.T) {
	var (
		search string
		yes    bool
		scopes []string
		params []string
	)
	cmd := leaf("gateways", "List gateways")
	cmd.Flags().StringVar(&search, "search", "", "")
	cmd.Flags().BoolVar(&yes, "yes", false, "")
	cmd.Flags().StringSliceVar(&scopes, "scope", []string{"openid", "email"}, "")
	cmd.Flags().StringArrayVar(&params, "parameter", nil, "")
	root := tree(cmd)

	root.SetArgs([]string{"gateways", "--search", "name like 'prod%'", "--yes", "--scope", "x", "--parameter", "a=b"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	ResetFlags(root)

	if search != "" || yes || len(params) != 0 {
		t.Errorf("after reset search=%q yes=%t parameter=%q, want defaults", search, yes, params)
	}
	if strings.Join(scopes, ",") != "openid,email" {
		t.Errorf("after reset scope = %q, want the default openid,email", scopes)
	}
	if cmd.Flags().Lookup("search").Changed {
		t.Error("--search still marked as set")
	}
}
