// Package extgroup registers the extension kinds (AgentRuntime and friends) and
// splits the help of each parent command into core and extension sections.
//
// The extension kinds are served under /api/hypershell/ext/. The rh-trex-ai CLI
// generator handles a single API prefix and generates only the /api/hypershell/v1
// kinds, so the extension commands are hand-written and registered here instead
// of by the generated parent commands.
package extgroup

import "github.com/spf13/cobra"

const (
	groupCore = "core"
	groupExt  = "ext"

	annotation = "hsctl.extension"
)

// Add registers extension commands on a generated parent command.
func Add(parent *cobra.Command, commands ...*cobra.Command) {
	for _, command := range commands {
		if command.Annotations == nil {
			command.Annotations = map[string]string{}
		}
		command.Annotations[annotation] = "true"
		parent.AddCommand(command)
	}
}

// Apply gives each parent a core and an extension help section and sorts its
// subcommands into them. Call it after every command is registered.
func Apply(parents ...*cobra.Command) {
	for _, parent := range parents {
		parent.AddGroup(
			&cobra.Group{ID: groupCore, Title: "Core (hypershell/v1/):"},
			&cobra.Group{ID: groupExt, Title: "Extensions (hypershell/ext/) - experimental, unsupported, subject to change:"},
		)
		for _, command := range parent.Commands() {
			if command.Annotations[annotation] == "true" {
				command.GroupID = groupExt
			} else {
				command.GroupID = groupCore
			}
		}
	}
}
