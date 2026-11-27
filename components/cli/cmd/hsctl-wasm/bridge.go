//go:build js && wasm

package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall/js"

	"github.com/spf13/cobra"

	"github.com/openshift-online/hypershell/components/cli/cmd/hsctl/commands"
	"github.com/openshift-online/hypershell/components/cli/pkg/config"
	"github.com/openshift-online/hypershell/components/cli/pkg/connection"
)

const (
	usageMessage = "Error: hsctlRun(argv) needs argv to be an array of strings: " +
		"the hsctl arguments without the program name, for example [\"list\", \"gateways\"]\n"
	busyMessage = "Error: another hsctl command is still running; wait for it to finish or cancel it, then try again\n"
)

// result is what hsctlRun resolves to (CLI-WASM-02).
type result struct {
	stdout   string
	stderr   string
	exitCode int
	mutated  bool
}

func (r result) js() js.Value {
	return js.ValueOf(map[string]any{
		"stdout":   r.stdout,
		"stderr":   r.stderr,
		"exitCode": r.exitCode,
		"mutated":  r.mutated,
	})
}

var (
	rootOnce    sync.Once
	browserRoot *cobra.Command
)

// sharedRoot returns the browser command tree. The commands are package-level
// singletons, so the tree is built once and each call resets its flags.
func sharedRoot() *cobra.Command {
	rootOnce.Do(func() { browserRoot = commands.NewBrowserRoot() })
	return browserRoot
}

// bridge runs hsctl commands for hsctlRun, one at a time.
type bridge struct {
	root      *cobra.Command
	transport http.RoundTripper
	busy      atomic.Bool
}

func newBridge(transport http.RoundTripper) *bridge {
	return &bridge{root: sharedRoot(), transport: transport}
}

// jsFunc returns the hsctlRun function. The worker never releases it.
func (b *bridge) jsFunc() js.Func {
	return js.FuncOf(b.jsRun)
}

func (b *bridge) jsRun(_ js.Value, args []js.Value) any {
	argv, ok := parseArgv(args)
	if !ok {
		return resolved(result{stderr: usageMessage, exitCode: 2})
	}
	if !b.busy.CompareAndSwap(false, true) {
		return resolved(result{stderr: busyMessage, exitCode: 1})
	}

	var executor js.Func
	executor = js.FuncOf(func(_ js.Value, promise []js.Value) any {
		resolve := promise[0]
		// A JavaScript callback must not block, and a command waits for fetch
		// responses, so the command runs on its own goroutine.
		go func() {
			defer executor.Release()
			res := b.execute(argv)
			b.busy.Store(false)
			resolve.Invoke(res.js())
		}()
		return nil
	})
	return js.Global().Get("Promise").New(executor)
}

// parseArgv accepts exactly one argument: an array of strings.
func parseArgv(args []js.Value) ([]string, bool) {
	if len(args) != 1 || !js.Global().Get("Array").Call("isArray", args[0]).Bool() {
		return nil, false
	}
	n := args[0].Length()
	argv := make([]string, 0, n)
	for i := 0; i < n; i++ {
		v := args[0].Index(i)
		if v.Type() != js.TypeString {
			return nil, false
		}
		argv = append(argv, v.String())
	}
	return argv, true
}

func resolved(r result) js.Value {
	return js.Global().Get("Promise").Call("resolve", r.js())
}

// execute runs one command the way the native main does: "Error: <err>" on
// standard error and exit code 1 when the command fails.
func (b *bridge) execute(argv []string) (res result) {
	out, err := startCapture()
	if err != nil {
		return result{stderr: fmt.Sprintf("Error: can't capture the command output: %s\n", err), exitCode: 1}
	}

	call := &callState{}
	defer func() {
		// A panic in a command would end the worker's Go program and leave the
		// call unresolved. Report it like the Go runtime reports a crash of the
		// native binary: on standard error with exit code 2.
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "panic: %v\n", r)
			res.exitCode = 2
		}
		res.stdout, res.stderr = out.stop()
		res.mutated = call.mutated.Load()
	}()

	origin := pageOrigin()
	config.SetStore(sessionStore{origin: origin})
	connection.SetTokenProviderFactory(func(*config.Config) connection.TokenProvider { return noToken{} })
	connection.SetHTTPClientFactory(func(*config.Config) connection.HTTPClient {
		return newSessionClient(origin, b.transport, call)
	})

	commands.ResetFlags(b.root)
	b.root.SetArgs(append([]string{}, argv...))
	// Standard input is empty and not a terminal, so a command that asks for
	// confirmation needs --yes, as in a script (CLI-WASM-02).
	b.root.SetIn(strings.NewReader(""))
	if err := b.root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		res.exitCode = 1
	}
	return res
}

// pageOrigin is the origin of the page that started the worker. A dedicated
// worker's location shares the page origin.
func pageOrigin() string {
	location := js.Global().Get("location")
	if location.Type() != js.TypeObject {
		return ""
	}
	origin := location.Get("origin")
	if origin.Type() != js.TypeString {
		return ""
	}
	return origin.String()
}
