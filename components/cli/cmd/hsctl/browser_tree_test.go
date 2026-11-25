package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/commands"
)

const describeBrowserTreeEnv = "HSCTL_TEST_DESCRIBE_BROWSER_TREE"

// TestHelperDescribeBrowserTree prints the browser command tree for
// TestBrowserTreeMatchesNativeTree. NewBrowserRoot takes over the shared command
// singletons, so it runs in a child process and leaves this process's native
// tree alone.
func TestHelperDescribeBrowserTree(t *testing.T) {
	if os.Getenv(describeBrowserTreeEnv) != "1" {
		t.Skip("helper for TestBrowserTreeMatchesNativeTree")
	}
	if err := json.NewEncoder(os.Stdout).Encode(commands.Describe(commands.NewBrowserRoot())); err != nil {
		t.Fatal(err)
	}
}

// The native hsctl and the browser module build their trees from one
// registration. Any command or flag that only the native tree has, outside the
// local-only set, fails here with the command named.
// Verifies: CLI-WASM-01, CLI-WASM-01.01, CLI-WASM-06, CLI-WASM-06.01
func TestBrowserTreeMatchesNativeTree(t *testing.T) {
	native := commands.Describe(root)

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperDescribeBrowserTree$")
	cmd.Env = append(os.Environ(), describeBrowserTreeEnv+"=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("describe the browser tree: %v\n%s", err, out)
	}
	// The test binary may print PASS after the JSON line.
	line, _, _ := strings.Cut(string(out), "\n")
	var browser map[string]string
	if err := json.Unmarshal([]byte(line), &browser); err != nil {
		t.Fatalf("parse the browser tree: %v\n%s", err, out)
	}

	for _, drift := range commands.TreeDrift(native, browser) {
		t.Error(drift)
	}
	for _, path := range []string{"list gateways", "create gateway", "delete gateway", "secret-source"} {
		if _, ok := browser[path]; !ok {
			t.Errorf("hsctl %s is missing from the browser tree", path)
		}
	}
	if _, ok := native["tui"]; !ok {
		t.Error("the native tree has no hsctl tui")
	}
}

// The browser module is built without hsctl tui, so it links no terminal UI or
// clipboard library; the native binary keeps hsctl tui.
// Verifies: CLI-WASM-01.02
func TestBrowserModuleLinksNoTerminalUI(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on PATH")
	}
	list := func(goos, goarch, pkg string) string {
		t.Helper()
		cmd := exec.Command(gobin, "list", "-deps", pkg)
		cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go list -deps %s for %s/%s: %v\n%s", pkg, goos, goarch, err, out)
		}
		return string(out)
	}
	const module = "github.com/openshift-online/hypershell/components/cli"
	browser := list("js", "wasm", module+"/cmd/hsctl-wasm")
	for _, forbidden := range []string{"github.com/charmbracelet/bubbletea", "github.com/atotto/clipboard"} {
		if strings.Contains(browser, forbidden+"\n") {
			t.Errorf("the browser module links %s", forbidden)
		}
	}
	if !strings.Contains(list("linux", "amd64", module+"/cmd/hsctl"), "github.com/charmbracelet/bubbletea\n") {
		t.Error("the native hsctl no longer links bubbletea; hsctl tui is missing")
	}
	if subcommand(root, "tui") == nil {
		t.Error("the native tree has no hsctl tui")
	}
}

// fileFlagPatterns are name parts of flags that read or write a local file or
// read standard input.
var fileFlagPatterns = []string{"file", "stdin", "body", "kustomize", "filename", "dir"}

// notFileFlags match fileFlagPatterns but take their value inline.
var notFileFlags = map[string]string{
	"profile": "create providerSpec: profile configuration as an inline JSON string",
}

// A new flag that reads a local file must be local-only in the browser, or the
// browser module would fail on it with a file error instead of naming the
// installed hsctl.
// Verifies: CLI-WASM-04
func TestFileFlagsAreLocalOnlyInTheBrowser(t *testing.T) {
	var walk func(cmd *cobra.Command, path string)
	walk = func(cmd *cobra.Command, path string) {
		if path == "" || !commands.IsLocalOnlyPath(path) {
			cmd.NonInheritedFlags().VisitAll(func(f *pflag.Flag) {
				if _, ok := commands.LocalOnlyFlags[f.Name]; ok {
					return
				}
				if _, ok := notFileFlags[f.Name]; ok {
					return
				}
				for _, pattern := range fileFlagPatterns {
					if strings.Contains(f.Name, pattern) {
						t.Errorf("hsctl %s --%s looks like it reads a local file or standard input; list it in "+
							"commands.LocalOnlyFlags, or in notFileFlags if it takes its value inline", path, f.Name)
						return
					}
				}
			})
		}
		for _, c := range cmd.Commands() {
			child := c.Name()
			if path != "" {
				child = path + " " + child
			}
			walk(c, child)
		}
	}
	walk(root, "")
}
