package gateway

import (
	"strings"
	"testing"
)

func TestPlacementFlagsExist(t *testing.T) {
	flags := Cmd.Flags()
	for name, expectedType := range map[string]string{
		"cluster-id":                 "string",
		"dedicated-route":            "bool",
		"name":                       "string",
		"body":                       "string",
		"placement-selector":         "stringArray",
		"placement-exclude-selector": "stringArray",
		"local-kind":                 "bool",
		"dry-run":                    "bool",
	} {
		flag := flags.Lookup(name)
		if flag == nil {
			t.Errorf("flag --%s is not registered", name)
			continue
		}
		if flag.Value.Type() != expectedType {
			t.Errorf("flag --%s has type %q, want %q", name, flag.Value.Type(), expectedType)
		}
	}
}

func TestHelpDocumentsBothShapesAndDryRun(t *testing.T) {
	help := Cmd.Long
	for _, want := range []string{
		"--cluster-id",
		"--placement-selector network/public --placement-selector provider/ibm",
		"--placement-selector region/us-east-1",
		"--local-kind",
		"--dry-run",
		"empty cluster_id",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("command help is missing %q", want)
		}
	}
}

// TestPlacementFlagsMutuallyExclusive executes fresh command instances with
// conflicting flags: cobra rejects the combination before RunE (and therefore
// before any config load or connection dial), so no test server is needed.
func TestPlacementFlagsMutuallyExclusive(t *testing.T) {
	cases := [][]string{
		{"--cluster-id", "abc", "--placement-selector", "network/public"},
		{"--cluster-id", "abc", "--placement-exclude-selector", "runtime/kata"},
		{"--cluster-id", "abc", "--local-kind"},
		{"--local-kind", "--placement-selector", "network/public"},
		{"--local-kind", "--placement-exclude-selector", "runtime/kata"},
		{"--dry-run", "--body", "request.json"},
	}
	for _, argv := range cases {
		cmd := newCreateGatewayCommand()
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs(argv)
		err := cmd.Execute()
		if err == nil {
			t.Errorf("Execute(%v) succeeded, want a mutual-exclusion error", argv)
			continue
		}
		// cobra's message: "if any flags in the group [a b] are set none of the others can be"
		if !strings.Contains(err.Error(), "none of the others can be") {
			t.Errorf("Execute(%v) error = %q, want a mutual-exclusion error", argv, err.Error())
		}
	}
}

// TestInvalidPlacementFlagFailsBeforeConfigLoad runs a command with an
// invalid placement flag value and asserts the BuildIntent error surfaces
// (RunE runs, but fails fast before loading config, which would otherwise
// error first in a bare test environment).
func TestInvalidPlacementFlagFailsBeforeConfigLoad(t *testing.T) {
	cases := []struct {
		argv []string
		want string
	}{
		{
			[]string{"--placement-selector", "network/public,provider/ibm"},
			"must not contain a comma",
		},
		{
			[]string{"--placement-selector", "cloud/aws"},
			"unknown namespace",
		},
		{
			[]string{"--placement-exclude-selector", "runtime/kata"},
			"--placement-exclude-selector requires",
		},
	}
	for _, tc := range cases {
		cmd := newCreateGatewayCommand()
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs(tc.argv)
		err := cmd.Execute()
		if err == nil {
			t.Errorf("Execute(%v) succeeded, want error containing %q", tc.argv, tc.want)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Execute(%v) error = %q, want error containing %q", tc.argv, err.Error(), tc.want)
		}
	}
}
