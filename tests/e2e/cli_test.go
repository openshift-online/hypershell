package e2e

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSandboxImagePin(t *testing.T) {
	for _, tc := range []struct {
		name, contents, want string
	}{
		{"tag", "# comment\nOPENSHELL_SANDBOX_IMAGE=registry.example:5000/workload:custom\n", "registry.example:5000/workload:custom"},
		{"digest", "OPENSHELL_SANDBOX_IMAGE=example/workload@sha256:abc\n", "example/workload@sha256:abc"},
		{"missing", "OPENSHELL_TAG=v1.2.3\n", ""},
		{"empty", "OPENSHELL_SANDBOX_IMAGE=\n", ""},
		{"quoted", "OPENSHELL_SANDBOX_IMAGE=\"example/workload:custom\"\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "OPENSHELL_VERSION")
			if err := os.WriteFile(path, []byte(tc.contents), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := readSandboxImage(path)
			if tc.want == "" {
				if err == nil {
					t.Fatal("expected invalid pin to fail")
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("readSandboxImage() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestOpenShiftCLICommandSharesCallerConfig(t *testing.T) {
	got := openShiftCLICommand(
		"/usr/bin/podman",
		"/home/runner",
		1001,
		1001,
		"quay.io/opendatahub/odh-openshell-cli:v0.1.6-rhaiv.2",
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
		"quay.io/opendatahub/odh-openshell-cli:v0.1.6-rhaiv.2",
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
		{"create", [][]string{{"sandbox", "create", "--from", "example/workload:test", "--name", "sb-1"}}},
		{"delete", [][]string{{"sandbox", "stop", "sb-1"}, {"sandbox", "delete", "sb-1"}}},
		{"stop", [][]string{{"sandbox", "stop", "sb-1"}}},
	}
	for _, tt := range tests {
		if got := sandboxCLISteps(tt.action, "sb-1", "example/workload:test"); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("sandboxCLISteps(%q) = %q, want %q", tt.action, got, tt.want)
		}
	}
}
