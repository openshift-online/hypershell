package main

import (
	"flag"
	"testing"
)

func TestEnforceSafeLogVerbosity(t *testing.T) {
	verbosity := flag.Lookup("v")
	vmodule := flag.Lookup("vmodule")
	if verbosity == nil || vmodule == nil {
		t.Fatal("glog verbosity flags are not registered")
	}
	originalVerbosity := verbosity.Value.String()
	originalVModule := vmodule.Value.String()
	t.Cleanup(func() {
		if err := verbosity.Value.Set(originalVerbosity); err != nil {
			t.Errorf("restore verbosity: %v", err)
		}
		if err := vmodule.Value.Set(originalVModule); err != nil {
			t.Errorf("restore vmodule: %v", err)
		}
	})

	if err := verbosity.Value.Set("10"); err != nil {
		t.Fatalf("set verbosity: %v", err)
	}
	if err := vmodule.Value.Set("formatter_json=10"); err != nil {
		t.Fatalf("set vmodule: %v", err)
	}
	if err := enforceSafeLogVerbosity(); err != nil {
		t.Fatalf("enforceSafeLogVerbosity: %v", err)
	}
	if got := verbosity.Value.String(); got != "9" {
		t.Fatalf("verbosity = %q, want 9", got)
	}
	if got := vmodule.Value.String(); got != "" {
		t.Fatalf("vmodule = %q, want empty", got)
	}

	if err := verbosity.Value.Set("4"); err != nil {
		t.Fatalf("set safe verbosity: %v", err)
	}
	if err := enforceSafeLogVerbosity(); err != nil {
		t.Fatalf("enforce safe verbosity: %v", err)
	}
	if got := verbosity.Value.String(); got != "4" {
		t.Fatalf("safe verbosity changed to %q", got)
	}
}
