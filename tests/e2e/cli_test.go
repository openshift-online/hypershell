package e2e

import (
	"reflect"
	"testing"
)

func TestOpenShiftCLICommandSharesCallerConfig(t *testing.T) {
	got := openShiftCLICommand(
		"/usr/bin/podman",
		"/home/runner",
		1001,
		1001,
		"quay.io/opendatahub/odh-openshell-cli:v0.1.3-rhaiv.3",
		"openshell-example-openshell",
		false,
		"status",
	)
	want := []string{
		"run", "--rm",
		"--userns=keep-id",
		"--user", "1001:1001",
		"-e", "HOME=/home/cli",
		"-v", "/home/runner/.config/openshell:/home/cli/.config/openshell:z",
		"quay.io/opendatahub/odh-openshell-cli:v0.1.3-rhaiv.3",
		"-g", "openshell-example-openshell", "status",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("openShiftCLICommand() = %q, want %q", got, want)
	}
}

func TestSandboxCLIStepsStopBeforeDelete(t *testing.T) {
	tests := []struct {
		action string
		want   [][]string
	}{
		{"create", [][]string{{"sandbox", "create", "--name", "sb-1"}}},
		{"delete", [][]string{{"sandbox", "stop", "sb-1"}, {"sandbox", "delete", "sb-1"}}},
		{"stop", [][]string{{"sandbox", "stop", "sb-1"}}},
	}
	for _, tt := range tests {
		if got := sandboxCLISteps(tt.action, "sb-1"); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("sandboxCLISteps(%q) = %q, want %q", tt.action, got, tt.want)
		}
	}
}
