package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Describe records, for every command in the tree, what a user can see of it:
// its usage line, aliases, help text, the rendered usage, and each flag's name,
// shorthand, type, default and description. It keys the result by the command
// path without "hsctl"; the root command has the empty key.
func Describe(root *cobra.Command) map[string]string {
	result := map[string]string{}
	describe(root, "", result)
	return result
}

func describe(cmd *cobra.Command, path string, result map[string]string) {
	var b strings.Builder
	fmt.Fprintf(&b, "use: %s\naliases: %s\nhidden: %t\nshort: %s\nlong: %s\nexample: %s\n",
		cmd.Use, strings.Join(cmd.Aliases, ","), cmd.Hidden, cmd.Short, cmd.Long, cmd.Example)
	var flags []string
	cmd.NonInheritedFlags().VisitAll(func(f *pflag.Flag) {
		flags = append(flags, fmt.Sprintf("flag: --%s -%s %s default=%q hidden=%t: %s",
			f.Name, f.Shorthand, f.Value.Type(), f.DefValue, f.Hidden, f.Usage))
	})
	sort.Strings(flags)
	for _, f := range flags {
		b.WriteString(f + "\n")
	}
	b.WriteString("usage:\n" + cmd.UsageString())
	result[path] = b.String()

	for _, c := range cmd.Commands() {
		child := c.Name()
		if path != "" {
			child = path + " " + child
		}
		describe(c, child, result)
	}
}

// TreeDrift compares a browser command tree with the native one, both from
// Describe, and returns one message per command that differs outside the
// local-only set of CLI-WASM-04. Each message names the command.
func TreeDrift(native, browser map[string]string) []string {
	paths := map[string]bool{}
	for path := range native {
		paths[path] = true
	}
	for path := range browser {
		paths[path] = true
	}
	sorted := make([]string, 0, len(paths))
	for path := range paths {
		sorted = append(sorted, path)
	}
	sort.Strings(sorted)

	var drift []string
	for _, path := range sorted {
		if IsLocalOnlyPath(path) {
			continue
		}
		name := strings.TrimSpace("hsctl " + path)
		n, inNative := native[path]
		b, inBrowser := browser[path]
		switch {
		case !inBrowser:
			drift = append(drift, fmt.Sprintf("%s is in the native command tree but not in the browser module; "+
				"register it in the shared registration (cmd/hsctl/commands) or list it in LocalOnlyCommands", name))
		case !inNative:
			drift = append(drift, fmt.Sprintf("%s is in the browser module but not in the native command tree", name))
		case n != b:
			drift = append(drift, fmt.Sprintf("%s differs between the native command tree and the browser module:\n%s",
				name, firstDifference(n, b)))
		}
	}
	return drift
}

func firstDifference(native, browser string) string {
	n, b := strings.Split(native, "\n"), strings.Split(browser, "\n")
	for i := 0; i < len(n) || i < len(b); i++ {
		var nl, bl string
		if i < len(n) {
			nl = n[i]
		}
		if i < len(b) {
			bl = b[i]
		}
		if nl != bl {
			return fmt.Sprintf("  native:  %s\n  browser: %s", nl, bl)
		}
	}
	return ""
}
