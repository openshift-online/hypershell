package main

import (
	"flag"
	"testing"
)

// mustLookupFlag returns the named flag, failing the test immediately if it
// is not registered. Isolating the nil check in its own function (rather than
// inline before later dereferences in the caller) keeps staticcheck's SA5011
// flow analysis from flagging those dereferences as a possible nil pointer.
func mustLookupFlag(t *testing.T, name string) *flag.Flag {
	t.Helper()
	f := flag.Lookup(name)
	if f == nil {
		t.Fatalf("glog flag %q is not registered", name)
	}
	return f
}

func TestEnforceSafeLogVerbosity(t *testing.T) {
	verbosity := mustLookupFlag(t, "v")
	vmodule := mustLookupFlag(t, "vmodule")
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
