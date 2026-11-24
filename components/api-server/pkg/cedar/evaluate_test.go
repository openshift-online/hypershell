package cedar

import (
	"strings"
	"testing"
)

const canaryCedar = `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource)
when { context.sandboxName like "e2epol-*" && context.cpuRequestMillis > 1000 };`

func mustEvaluator(t *testing.T, policies ...PolicyInput) *Evaluator {
	t.Helper()
	e, err := NewEvaluator(policies)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	return e
}

// Verifies: PLAT-PGP-02.01
// allow by default; a forbid narrows.
func TestEvaluatorAllowByDefault(t *testing.T) {
	e := mustEvaluator(t)
	if d := e.Evaluate(Request{}); !d.Allowed {
		t.Fatalf("empty set must allow, got %+v", d)
	}
	e = mustEvaluator(t, PolicyInput{Name: "cpu-max", Cedar: `forbid (principal, action, resource) when { context.cpuRequestMillis > 4000 };`})
	if d := e.Evaluate(Request{Context: Context{CPURequestMillis: 1000}}); !d.Allowed {
		t.Fatalf("request under the bound must stay allowed: %+v", d)
	}
	if d := e.Evaluate(Request{Context: Context{CPURequestMillis: 8000}}); d.Allowed {
		t.Fatalf("request over the bound must be rejected")
	}
}

// Verifies: PLAT-PGP-02.02
// one policy cannot re-allow another's forbid.
func TestEvaluatorForbidOverrides(t *testing.T) {
	e := mustEvaluator(t,
		PolicyInput{Name: "a", Cedar: `forbid (principal, action, resource) when { context.gpuCount > 0 };`},
		PolicyInput{Name: "b", Cedar: `forbid (principal, action, resource) when { context.cpuRequestMillis > 100000 };`},
	)
	d := e.Evaluate(Request{Context: Context{GPUCount: 1}})
	if d.Allowed || len(d.MatchedPolicies()) != 1 || d.MatchedPolicies()[0] != "a" {
		t.Fatalf("expected rejection by a only, got %+v", d)
	}
	if _, err := NewEvaluator([]PolicyInput{{Name: "p", Cedar: `permit (principal, action, resource);`}}); err == nil {
		t.Fatalf("a permit must not compile into an evaluator")
	}
}

// Verifies: PLAT-PGP-14.01
// the reason lists every matching policy
// with its message and the value and bound for recognized statements.
func TestDenialReasonListsEveryPolicy(t *testing.T) {
	e := mustEvaluator(t,
		PolicyInput{Name: "cpu-max", Cedar: `forbid (principal, action, resource) when { context.cpuRequestMillis > 1000 };`},
		PolicyInput{Name: "no-gpu", Cedar: `forbid (principal, action, resource) when { context.gpuCount > 0 && principal.roles.contains("x") == false };`},
	)
	d := e.Evaluate(Request{Context: Context{CPURequestMillis: 2000, GPUCount: 2}})
	if d.Allowed {
		t.Fatalf("expected rejection")
	}
	reason := DenialReason(d, map[string]PolicyMessage{
		"cpu-max": {UserMessage: "Keep CPU small.", DocumentationURL: "https://docs.example/cpu"},
		"no-gpu":  {UserMessage: "GPUs need approval."},
	}, "https://console.example/gateways/g/policies")
	for _, want := range []string{"cpu-max: Keep CPU small.", "CPU request is 2; allowed: at most 1", "no-gpu: GPUs need approval.", "GPU count is 2; allowed: at most 0", "Rejected by 2 platform policies", "https://console.example/gateways/g/policies"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason missing %q:\n%s", want, reason)
		}
	}
	if strings.Contains(reason, "docs.example/cpu") {
		t.Errorf("with two matching policies the link must be the Policies tab:\n%s", reason)
	}
}

// Verifies: PLAT-PGP-20.01
// canary rejects only the reserved prefix
// and its denial carries the CPU bound (E2E-CORE-49).
func TestCanaryScopedToReservedPrefix(t *testing.T) {
	e := mustEvaluator(t, PolicyInput{Name: "e2e-enforcement-canary", Cedar: canaryCedar})
	deny := e.Evaluate(Request{Context: Context{SandboxName: "e2epol-run1-deny", CPURequestMillis: 2000}})
	if deny.Allowed {
		t.Fatalf("canary must reject the reserved prefix over 1 CPU")
	}
	reason := DenialReason(deny, map[string]PolicyMessage{"e2e-enforcement-canary": {UserMessage: "Release check."}}, "")
	if !strings.Contains(reason, "e2e-enforcement-canary: Release check.") || !strings.Contains(reason, "allowed: at most 1") {
		t.Fatalf("canary reason must carry name, message, and bound:\n%s", reason)
	}
	if d := e.Evaluate(Request{Context: Context{SandboxName: "my-sandbox", CPURequestMillis: 2000}}); !d.Allowed {
		t.Fatalf("canary must not reject a sandbox without the reserved prefix")
	}
}

// Verifies: PLAT-PGP-14.02
// the reason carries only normalized values.
func TestDenialReasonCarriesNoRequestFields(t *testing.T) {
	e := mustEvaluator(t, PolicyInput{Name: "p", Cedar: `forbid (principal, action, resource) when { context.image like "*secret-token*" };`})
	d := e.Evaluate(Request{Context: Context{Image: "registry/secret-token-abc"}})
	reason := DenialReason(d, map[string]PolicyMessage{"p": {UserMessage: "Image not allowed."}}, "")
	if strings.Contains(reason, "secret-token-abc") {
		t.Fatalf("reason leaked a request field: %s", reason)
	}
}

// Verifies: PLAT-PGP-11.02
// evaluation is deterministic.
func TestEvaluatorDeterministic(t *testing.T) {
	e := mustEvaluator(t,
		PolicyInput{Name: "z", Cedar: `forbid (principal, action, resource) when { context.cpuRequestMillis > 1 };
forbid (principal, action, resource) when { context.memoryRequestBytes > 1 };`},
		PolicyInput{Name: "a", Cedar: `forbid (principal, action, resource) when { context.cpuLimitMillis > 1 };`},
	)
	req := Request{Context: Context{CPURequestMillis: 5, MemoryRequestBytes: 5, CPULimitMillis: 5}}
	first := DenialReason(e.Evaluate(req), nil, "x")
	for i := 0; i < 20; i++ {
		if got := DenialReason(e.Evaluate(req), nil, "x"); got != first {
			t.Fatalf("non-deterministic reason:\n%s\nvs\n%s", first, got)
		}
	}
	if !strings.HasPrefix(strings.Split(first, "\n")[1], "- z") {
		t.Fatalf("policies must be listed in input order:\n%s", first)
	}
}

// Verifies: PLAT-PGP-17
// structured bounds accompany the plain-words summary.
func TestRenderPolicyBounds(t *testing.T) {
	rules, err := RenderPolicy(`forbid (principal, action, resource) when { context.cpuRequestMillis < 250 || context.cpuRequestMillis > 4000 };`)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || len(rules[0].Bounds) != 2 || rules[0].Bounds[1].Display != "4" || rules[0].Bounds[0].Display != "250m" {
		t.Fatalf("unexpected bounds: %+v", rules)
	}
}

// Verifies: PLAT-PGP-04.01, WEB-PADM-05
// Diagnostics name the line and suggest a fix.
func TestValidatePolicyDiagnosticsNameLineAndFix(t *testing.T) {
	err := ValidatePolicy("forbid (principal, action, resource) when { context.cpuLimitMillis > 9000 };\n\nforbid (principal, action, resource)\nwhen { context.cpuRequest > 4 };", Defaults{})
	ve, ok := err.(*ValidationError)
	if !ok || len(ve.Details) != 1 {
		t.Fatalf("expected one structured diagnostic, got %v", err)
	}
	d := ve.Details[0]
	if d.Line != 3 || d.Statement != 1 || !strings.Contains(d.Suggestion, "context.cpuRequestMillis") {
		t.Fatalf("unexpected diagnostic: %+v", d)
	}
	err = ValidatePolicy("\n\npermit (principal, action, resource);", Defaults{})
	ve, ok = err.(*ValidationError)
	if !ok || ve.Details[0].Line != 3 || !strings.Contains(ve.Details[0].Suggestion, "forbid") {
		t.Fatalf("permit diagnostic must name line 3 and suggest forbid: %+v", err)
	}
}
