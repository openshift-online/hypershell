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

// TestHelpDocumentsLateBoundSurface pins CLI-PLC-04.01: the help documents the
// late-bound selector surface and the --dry-run pre-check, keeps the
// awaiting-placement meaning of an empty cluster_id FIELD, and does NOT advertise the
// hidden --cluster-id flag or a pinned --cluster-id example.
func TestHelpDocumentsLateBoundSurface(t *testing.T) {
	help := Cmd.Long
	for _, want := range []string{
		"--placement-selector network/public --placement-selector provider/ibm",
		"--placement-selector region/us-east-1",
		"--placement-selector provider/kind",
		"--dry-run",
		"empty cluster_id",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("command help is missing %q", want)
		}
	}
	if strings.Contains(help, "--cluster-id") {
		t.Errorf("command help must not advertise the hidden --cluster-id flag")
	}
}

// TestClusterIDFlagHiddenButFunctional pins CLI-PLC-05: --cluster-id stays registered
// and accepted (so the pinned contract still works) but is hidden from --help.
func TestClusterIDFlagHiddenButFunctional(t *testing.T) {
	flag := newCreateGatewayCommand().Flags().Lookup("cluster-id")
	if flag == nil {
		t.Fatal("--cluster-id must remain registered and accepted")
	}
	if !flag.Hidden {
		t.Error("--cluster-id must be hidden from help (cobra Hidden: true)")
	}
}

// TestPlacementFlagsMutuallyExclusive executes fresh command instances with
// conflicting flags: cobra rejects the combination before RunE (and therefore
// before any config load or connection dial), so no test server is needed.
func TestPlacementFlagsMutuallyExclusive(t *testing.T) {
	cases := [][]string{
		{"--cluster-id", "abc", "--placement-selector", "network/public"},
		{"--cluster-id", "abc", "--placement-exclude-selector", "runtime/kata"},
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
