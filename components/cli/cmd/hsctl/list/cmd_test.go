package list

import (
	"testing"

	"github.com/spf13/cobra"
)

var coreKinds = []string{"gateways", "managedClusters", "roles", "roleBindings", "users", "serviceAccounts"}
var extKinds = []string{"agentRuntimes", "sandboxTemplates", "providerSpecs", "providerBindings", "inferenceRoutes", "secretSources"}

func TestListSubcommandsRegistered(t *testing.T) {
	all := append(coreKinds, extKinds...)
	registered := make(map[string]*cobra.Command)
	for _, sub := range Cmd.Commands() {
		registered[sub.Name()] = sub
	}
	for _, name := range all {
		if _, ok := registered[name]; !ok {
			t.Errorf("list subcommand %q not registered", name)
		}
	}
}

func TestListCoreCommandsInCoreGroup(t *testing.T) {
	for _, sub := range Cmd.Commands() {
		name := sub.Name()
		for _, k := range coreKinds {
			if k == name && sub.GroupID != groupCore {
				t.Errorf("list %q: GroupID = %q, want %q", name, sub.GroupID, groupCore)
			}
		}
	}
}

func TestListExtCommandsInExtGroup(t *testing.T) {
	for _, sub := range Cmd.Commands() {
		name := sub.Name()
		for _, k := range extKinds {
			if k == name && sub.GroupID != groupExt {
				t.Errorf("list %q: GroupID = %q, want %q", name, sub.GroupID, groupExt)
			}
		}
	}
}
