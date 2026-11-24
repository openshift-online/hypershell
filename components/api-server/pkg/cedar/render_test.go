package cedar

import (
	"strings"
	"testing"
)

// PLAT-PGP-17.01: a recognized shape renders as plain words.
func TestRenderRecognizedShape(t *testing.T) {
	rules, err := RenderPolicy(`forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuRequestMillis > 4000 };`)
	if err != nil {
		t.Fatalf("RenderPolicy: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if !rules[0].Recognized {
		t.Fatalf("expected recognized rule, got: %+v", rules[0])
	}
	if want := "CPU request: at most 4"; rules[0].Summary[0] != want {
		t.Fatalf("summary = %q, want %q", rules[0].Summary[0], want)
	}
}

func TestRenderComparisons(t *testing.T) {
	tests := []struct {
		name string
		when string
		want string
	}{
		{"cpu request max", "context.cpuRequestMillis > 4000", "CPU request: at most 4"},
		{"cpu request min", "context.cpuRequestMillis < 250", "CPU request: at least 250m"},
		{"cpu limit ge", "context.cpuLimitMillis >= 8000", "CPU limit: less than 8"},
		{"memory request max", "context.memoryRequestBytes > 17179869184", "Memory request: at most 16Gi"},
		{"memory limit mi", "context.memoryLimitBytes > 536870912", "Memory limit: at most 512Mi"},
		{"gpu max", "context.gpuCount > 1", "GPU count: at most 1"},
		{"mirrored literal on left", "4000 < context.cpuRequestMillis", "CPU request: at most 4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := RenderPolicy(`forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { ` + tt.when + ` };`)
			if err != nil {
				t.Fatalf("RenderPolicy: %v", err)
			}
			if !rules[0].Recognized {
				t.Fatalf("expected recognized rule for %q", tt.when)
			}
			if rules[0].Summary[0] != tt.want {
				t.Fatalf("summary = %q, want %q", rules[0].Summary[0], tt.want)
			}
		})
	}
}

func TestRenderMultipleComparisons(t *testing.T) {
	rules, err := RenderPolicy(`forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuRequestMillis < 250 || context.cpuRequestMillis > 4000 };`)
	if err != nil {
		t.Fatalf("RenderPolicy: %v", err)
	}
	if !rules[0].Recognized {
		t.Fatalf("expected recognized rule")
	}
	if len(rules[0].Summary) != 2 {
		t.Fatalf("expected 2 summary lines, got %d: %v", len(rules[0].Summary), rules[0].Summary)
	}
}

// A conjunction (&&) of bounds in a forbid cannot be shown as a list of
// independent allowed bounds, so it falls back to the Cedar source rather than
// rendering a misleading summary.
func TestRenderConjunctionUnrecognized(t *testing.T) {
	src := `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuRequestMillis > 4000 && context.gpuCount > 1 };`
	rules, err := RenderPolicy(src)
	if err != nil {
		t.Fatalf("RenderPolicy: %v", err)
	}
	if rules[0].Recognized {
		t.Fatalf("expected && forbid to be unrecognized, got summary %v", rules[0].Summary)
	}
}

// Multiple when clauses are AND-combined and are shown as Cedar source.
func TestRenderMultipleWhenUnrecognized(t *testing.T) {
	src := `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.cpuRequestMillis > 4000 }
when { context.gpuCount > 1 };`
	rules, err := RenderPolicy(src)
	if err != nil {
		t.Fatalf("RenderPolicy: %v", err)
	}
	if rules[0].Recognized {
		t.Fatalf("expected multiple-when forbid to be unrecognized, got summary %v", rules[0].Summary)
	}
}

// PLAT-PGP-17.01: an unrecognized shape is shown as its Cedar source.
func TestRenderUnrecognizedShape(t *testing.T) {
	src := `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.sandboxName like "e2epol-*" && context.cpuRequestMillis > 1000 };`
	rules, err := RenderPolicy(src)
	if err != nil {
		t.Fatalf("RenderPolicy: %v", err)
	}
	if rules[0].Recognized {
		t.Fatalf("expected unrecognized rule, got summary %v", rules[0].Summary)
	}
	if !strings.Contains(rules[0].Cedar, "sandboxName") {
		t.Fatalf("Cedar source missing:\n%s", rules[0].Cedar)
	}
}
