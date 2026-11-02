package gateway

import (
	"strings"
	"testing"
)

func TestDefaultConsoleImageReadsEnvOnly(t *testing.T) {
	t.Setenv(ConsoleImageEnv, "")
	if got := (StaticImageDefaults{}).DefaultConsoleImage(); got != "" {
		t.Fatalf("DefaultConsoleImage() = %q with %s unset, want empty (no built-in default)", got, ConsoleImageEnv)
	}

	const image = "registry.example.com/openshell-dashboard@sha256:" + "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90"
	t.Setenv(ConsoleImageEnv, image)
	if got := (StaticImageDefaults{}).DefaultConsoleImage(); got != image {
		t.Fatalf("DefaultConsoleImage() = %q, want %q", got, image)
	}
}

func TestValidateConsoleImage(t *testing.T) {
	t.Setenv(ConsoleImageEnv, "")
	err := ValidateConsoleImage()
	if err == nil {
		t.Fatalf("ValidateConsoleImage() = nil with %s unset, want an error", ConsoleImageEnv)
	}
	if !strings.Contains(err.Error(), ConsoleImageEnv) {
		t.Fatalf("ValidateConsoleImage() error %q does not name %s", err, ConsoleImageEnv)
	}

	t.Setenv(ConsoleImageEnv, "registry.example.com/openshell-dashboard:v0.1.1")
	if err := ValidateConsoleImage(); err != nil {
		t.Fatalf("ValidateConsoleImage() = %v with %s set, want nil", err, ConsoleImageEnv)
	}
}
