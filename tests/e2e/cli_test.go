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
		"quay.io/opendatahub/odh-openshell-cli:v0.1.2-rhaiv.7",
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
		"quay.io/opendatahub/odh-openshell-cli:v0.1.2-rhaiv.7",
		"-g", "openshell-example-openshell", "status",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("openShiftCLICommand() = %q, want %q", got, want)
	}
}
