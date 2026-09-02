//go:build linked_metadata

package api

import (
	"os"
	"testing"

	trexapi "github.com/openshift-online/rh-trex-ai/pkg/api"
)

func TestLinkedBuildIdentity(t *testing.T) {
	expectedVersion := os.Getenv("HYPERSHELL_EXPECTED_BUILD_VERSION")
	if expectedVersion == "" {
		t.Fatal("HYPERSHELL_EXPECTED_BUILD_VERSION is not set")
	}
	expectedBuildTime := os.Getenv("HYPERSHELL_EXPECTED_BUILD_TIME")
	if expectedBuildTime == "" {
		t.Fatal("HYPERSHELL_EXPECTED_BUILD_TIME is not set")
	}

	if trexapi.Version != expectedVersion {
		t.Errorf("linked version = %q, want %q", trexapi.Version, expectedVersion)
	}
	if trexapi.BuildTime != expectedBuildTime {
		t.Errorf("linked build time = %q, want %q", trexapi.BuildTime, expectedBuildTime)
	}
}
