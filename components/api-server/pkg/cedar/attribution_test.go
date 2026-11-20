package cedar

import (
	"strings"
	"testing"
)

// PLAT-PGP-14: the control plane owns @id; Annotate stamps it onto every
// statement so Cedar reports which policy decided a result.
func TestAnnotateAddsID(t *testing.T) {
	policy := `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuRequestMillis > 4000 };

forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.gpuCount > 1 };`
	out, err := Annotate(policy, "sandbox-resource-baseline")
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if got := strings.Count(out, `@id("sandbox-resource-baseline")`); got != 2 {
		t.Fatalf("expected 2 @id annotations, got %d in:\n%s", got, out)
	}
}

// Annotate overwrites an author-written annotation rather than keeping it.
func TestAnnotateOverwritesAuthorAnnotation(t *testing.T) {
	policy := `@id("author-owned")
forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuRequestMillis > 4000 };`
	out, err := Annotate(policy, "platform-owned")
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	if strings.Contains(out, "author-owned") {
		t.Fatalf("author annotation survived:\n%s", out)
	}
	if !strings.Contains(out, `@id("platform-owned")`) {
		t.Fatalf("control-plane @id missing:\n%s", out)
	}
}

// Statement order is preserved so output is deterministic (PLAT-PGP-11/12).
func TestAnnotateIsDeterministic(t *testing.T) {
	policy := `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuRequestMillis > 4000 };

forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.memoryRequestBytes > 17179869184 };`
	first, err := Annotate(policy, "p")
	if err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	for i := 0; i < 5; i++ {
		again, err := Annotate(policy, "p")
		if err != nil {
			t.Fatalf("Annotate: %v", err)
		}
		if again != first {
			t.Fatalf("Annotate not deterministic:\n%s\n---\n%s", first, again)
		}
	}
	if strings.Index(first, "cpuRequestMillis") > strings.Index(first, "memoryRequestBytes") {
		t.Fatalf("statement order not preserved:\n%s", first)
	}
}
