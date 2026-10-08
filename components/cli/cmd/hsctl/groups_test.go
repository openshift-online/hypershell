package main

import (
	"strings"
	"testing"
)

// The extension kinds are hand-written and registered by extgroup, and main
// groups every parent's help into core and extension sections. These checks
// cover the generated parents (get, list, create, delete) and the hand-written
// service account commands registered on them.
func TestParentCommandsSplitCoreAndExtensionHelp(t *testing.T) {
	ext := []string{"agentRuntime", "sandboxTemplate", "providerSpec", "providerBinding", "inferenceRoute", "secretSource"}
	extPlural := []string{"agentRuntimes", "sandboxTemplates", "providerSpecs", "providerBindings", "inferenceRoutes", "secretSources"}

	for _, tc := range []struct {
		verb string
		core []string
		ext  []string
	}{
		{"get", []string{"gateway", "managedCluster", "role", "roleBinding", "user", "serviceAccount"}, ext},
		{"list", []string{"gateways", "managedClusters", "roles", "roleBindings", "users", "serviceAccounts"}, extPlural},
		{"create", []string{"gateway", "managedCluster", "role", "roleBinding", "user", "serviceAccount"}, ext},
		{"delete", []string{"gateway", "managedCluster", "roleBinding", "serviceAccount"}, ext},
	} {
		t.Run(tc.verb, func(t *testing.T) {
			parent := subcommand(root, tc.verb)
			if parent == nil {
				t.Fatalf("%s is not registered", tc.verb)
			}

			groups := map[string]string{}
			for _, g := range parent.Groups() {
				groups[g.ID] = g.Title
			}
			if len(groups) != 2 {
				t.Fatalf("groups = %v, want a core and an extension group", groups)
			}
			var extID string
			for id, title := range groups {
				if strings.Contains(title, "experimental") && strings.Contains(title, "unsupported") {
					extID = id
				}
			}
			if extID == "" {
				t.Fatalf("no group title warns that extensions are experimental and unsupported: %v", groups)
			}

			inGroup := func(names []string, wantExt bool) {
				for _, name := range names {
					sub := subcommand(parent, name)
					if sub == nil {
						t.Errorf("%s %s is not registered", tc.verb, name)
						continue
					}
					if (sub.GroupID == extID) != wantExt {
						t.Errorf("%s %s: group %q, extension group is %q, want extension=%v", tc.verb, name, sub.GroupID, extID, wantExt)
					}
				}
			}
			inGroup(tc.core, false)
			inGroup(tc.ext, true)

			// Every subcommand belongs to a group, so none lands under "Additional Commands".
			for _, sub := range parent.Commands() {
				if sub.GroupID == "" {
					t.Errorf("%s %s has no group", tc.verb, sub.Name())
				}
			}
		})
	}
}

// The extension kinds have no update command: a generated one would call /v1/,
// where these kinds are not served.
func TestExtensionKindsHaveNoGeneratedUpdate(t *testing.T) {
	update := subcommand(root, "update")
	for _, name := range []string{"agentRuntime", "sandboxTemplate", "providerSpec", "providerBinding", "inferenceRoute", "secretSource"} {
		if subcommand(update, name) != nil {
			t.Errorf("update %s exists; a generated one would call the /v1/ prefix", name)
		}
	}
}
