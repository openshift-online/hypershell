package cedar

import (
	"errors"
	"strings"
	"testing"
)

// testDefaults mirrors the example fleet defaults from PLAT-PGP-08:
// CPU request 500m, CPU limit 2, memory request 1Gi, memory limit 4Gi.
func testDefaults(t *testing.T) Defaults {
	t.Helper()
	d, err := ParseDefaults("500m", "2", "1Gi", "4Gi")
	if err != nil {
		t.Fatalf("ParseDefaults: %v", err)
	}
	return d
}

func TestValidatePolicyAcceptsForbid(t *testing.T) {
	d := testDefaults(t)
	policy := `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuRequestMillis < 250 || context.cpuRequestMillis > 4000 };

forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.memoryRequestBytes > 17179869184 };`
	if err := ValidatePolicy(policy, d); err != nil {
		t.Fatalf("ValidatePolicy valid forbid policy: %v", err)
	}
}

// PLAT-PGP-04.01 / PLAT-PGP-02: a permit is rejected.
func TestValidatePolicyRejectsPermit(t *testing.T) {
	d := testDefaults(t)
	err := ValidatePolicy(`permit (principal, action == OpenShell::Action::"CreateSandbox", resource);`, d)
	assertValidationErrorContains(t, err, "permit is not allowed")
}

// PLAT-PGP-04.01: an unknown attribute is rejected with the cedar diagnostic.
func TestValidatePolicyRejectsUnknownAttribute(t *testing.T) {
	d := testDefaults(t)
	err := ValidatePolicy(`forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuRequest > 4000 };`, d)
	assertValidationErrorContains(t, err, "cpuRequest")
}

// PLAT-PGP-04.01: a wrong type is rejected with the cedar diagnostic.
func TestValidatePolicyRejectsWrongType(t *testing.T) {
	d := testDefaults(t)
	err := ValidatePolicy(`forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuRequestMillis > "big" };`, d)
	assertValidationErrorContains(t, err, "type")
}

// PLAT-PGP-04.02: a policy that forbids the default request is rejected.
func TestValidatePolicyRejectsForbiddenDefaults(t *testing.T) {
	d := testDefaults(t)
	// Default CPU request is 500m; forbidding < 600 forbids the default.
	err := ValidatePolicy(`forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuRequestMillis < 600 };`, d)
	assertValidationErrorContains(t, err, "default sandbox request")
}

// PLAT-PGP-04 rule 4: author-written annotations are rejected.
func TestValidatePolicyRejectsAuthorAnnotation(t *testing.T) {
	d := testDefaults(t)
	err := ValidatePolicy(`@id("author-owned")
forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuRequestMillis > 4000 };`, d)
	assertValidationErrorContains(t, err, "annotation")
}

func TestValidatePolicyRejectsEmpty(t *testing.T) {
	d := testDefaults(t)
	err := ValidatePolicy("   ", d)
	if err == nil {
		t.Fatal("expected error for empty policy text")
	}
}

// The e2e enforcement canary (PLAT-PGP-20) must validate.
func TestValidatePolicyAcceptsCanary(t *testing.T) {
	d := testDefaults(t)
	policy := `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.sandboxName like "hypershell-e2e-policy-*" && context.cpuRequestMillis > 1000 };`
	if err := ValidatePolicy(policy, d); err != nil {
		t.Fatalf("ValidatePolicy canary: %v", err)
	}
}

func assertValidationErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a validation error containing %q, got nil", want)
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
		t.Fatalf("validation error %q does not contain %q", err.Error(), want)
	}
}
