package main

import (
	"fmt"
	"os"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/commands"
)

// root is the native command tree. The browser module builds the same tree from
// the same registration (cmd/hsctl/commands); register commands there, not here,
// or the browser drift test fails (CLI-WASM-01, CLI-WASM-06).
var root = commands.NewRoot()

func main() {
	root.SetArgs(os.Args[1:])
	err := root.Execute()
	if err == nil {
		os.Exit(0)
	}

	fmt.Fprintf(os.Stderr, "Error: %s\n", err)
	os.Exit(1)
}
