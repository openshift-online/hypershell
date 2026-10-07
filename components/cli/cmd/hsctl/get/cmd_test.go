package get

import (
	"testing"

	"github.com/spf13/cobra"
)

var coreKinds = []string{"gateway", "managedCluster", "role", "roleBinding", "user", "serviceAccount"}
var extKinds = []string{"agentRuntime", "sandboxTemplate", "providerSpec", "providerBinding", "inferenceRoute", "secretSource"}

func TestGetSubcommandsRegistered(t *testing.T) {
	all := append(coreKinds, extKinds...)
	registered := make(map[string]*cobra.Command)
	for _, sub := range Cmd.Commands() {
		registered[sub.Name()] = sub
	}
	for _, name := range all {
		if _, ok := registered[name]; !ok {
			t.Errorf("get subcommand %q not registered", name)
		}
	}
}

func TestGetCoreCommandsInCoreGroup(t *testing.T) {
	for _, sub := range Cmd.Commands() {
		name := sub.Name()
		isCoreKind := false
		for _, k := range coreKinds {
			if k == name {
				isCoreKind = true
				break
			}
		}
		if isCoreKind && sub.GroupID != groupCore {
			t.Errorf("get %q: GroupID = %q, want %q", name, sub.GroupID, groupCore)
		}
	}
}

func TestGetExtCommandsInExtGroup(t *testing.T) {
	for _, sub := range Cmd.Commands() {
		name := sub.Name()
		isExtKind := false
		for _, k := range extKinds {
			if k == name {
				isExtKind = true
				break
			}
		}
		if isExtKind && sub.GroupID != groupExt {
			t.Errorf("get %q: GroupID = %q, want %q", name, sub.GroupID, groupExt)
		}
	}
}

func TestGetGroupsRegistered(t *testing.T) {
	groups := make(map[string]string)
	for _, g := range Cmd.Groups() {
		groups[g.ID] = g.Title
	}
	if _, ok := groups[groupCore]; !ok {
		t.Errorf("group %q not registered on get command", groupCore)
	}
	if _, ok := groups[groupExt]; !ok {
		t.Errorf("group %q not registered on get command", groupExt)
	}
	if title := groups[groupExt]; title == "" {
		t.Errorf("ext group title is empty")
	}
	// Verify ext group title mentions experimental
	for _, keyword := range []string{"experimental", "unsupported"} {
		found := false
		for _, g := range Cmd.Groups() {
			if g.ID == groupExt {
				for i := 0; i+len(keyword) <= len(g.Title); i++ {
					if g.Title[i:i+len(keyword)] == keyword {
						found = true
					}
				}
			}
		}
		if !found {
			t.Errorf("ext group title does not contain %q: got %q", keyword, groups[groupExt])
		}
	}
}
