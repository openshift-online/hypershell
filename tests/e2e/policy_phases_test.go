package e2e

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
)

const testCanaryMessage = "Used by HyperShell release checks. Applies only to sandboxes named hypershell-e2e-policy-*: they may request at most 1 CPU."

// cliDenial renders a denial the way the pinned openshell CLI prints a tonic
// PERMISSION_DENIED status: the code by description and the message quoted
// with Rust Debug escaping.
func cliDenial(reason string) string {
	quoted := strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`).Replace(reason)
	return "Error:   x code: 'The caller does not have permission to execute the specified operation', message: \"" + quoted + "\"\n"
}

// Verifies: E2E-CORE-49.01, PLAT-PGP-20.01
func TestCheckPolicyDenialCanary(t *testing.T) {
	reason := "Rejected by platform policy:\n- e2e-enforcement-canary: " + testCanaryMessage + " (CPU request is 2; allowed: at most 1)\nMore information: https://console.example/gateways/g1/policies"
	want := []policyDenialExpectation{{Name: canaryPolicyName, UserMessage: testCanaryMessage, Bound: canaryCPUBound}}
	if err := checkPolicyDenial(cliDenial(reason), want); err != nil {
		t.Fatalf("CLI canary denial rejected: %v", err)
	}
	if err := checkPolicyDenial(reason+"\nstatus: PERMISSION_DENIED", want); err != nil {
		t.Fatalf("unescaped canary denial rejected: %v", err)
	}

	for name, out := range map[string]string{
		"not permission denied": strings.Replace(cliDenial(reason), "does not have permission to execute the specified operation", "argument is invalid", 1),
		"no bound":              cliDenial(strings.Replace(reason, " (CPU request is 2; allowed: at most 1)", "", 1)),
		"no user message":       cliDenial(strings.Replace(reason, ": "+testCanaryMessage, "", 1)),
		"other policy":          cliDenial(strings.Replace(reason, "e2e-enforcement-canary", "cpu-cap", 1)),
		"name length refusal":   "Error: code: 'Client specified an invalid argument', message: \"name exceeds maximum length (27 > 19)\"",
	} {
		t.Run(name, func(t *testing.T) {
			if err := checkPolicyDenial(out, want); err == nil {
				t.Fatalf("denial accepted:\n%s", out)
			}
		})
	}
}

// Verifies: E2E-CORE-50.01, PLAT-PGP-14.01
func TestCheckPolicyDenialListsEveryPolicy(t *testing.T) {
	reason := "Rejected by 2 platform policies:\n- e2e-pgp-cpu-1: CPU cap. (CPU request is 1.5; allowed: at most 1)\n- e2e-pgp-mem-1: Memory cap.\nMore information: https://console.example/policies"
	both := []policyDenialExpectation{
		{Name: "e2e-pgp-cpu-1", UserMessage: "CPU cap.", Bound: canaryCPUBound},
		{Name: "e2e-pgp-mem-1", UserMessage: "Memory cap."},
	}
	if err := checkPolicyDenial(cliDenial(reason), both); err != nil {
		t.Fatalf("two-policy denial rejected: %v", err)
	}
	if err := checkPolicyDenial(cliDenial(reason), both[:1]); err == nil {
		t.Fatal("a two-policy denial must not satisfy a single-policy expectation (header count)")
	}
	single := "Rejected by platform policy:\n- e2e-pgp-cpu-1: CPU cap. (CPU request is 1.5; allowed: at most 1)"
	if err := checkPolicyDenial(cliDenial(single), both); err == nil {
		t.Fatal("a single-policy denial must not satisfy a two-policy expectation")
	}
	// A policy whose name is a prefix of another must not match the longer line.
	if _, ok := policyDenialLine(strings.Split(reason, "\n"), "e2e-pgp-cpu"); ok {
		t.Fatal("policy name prefix matched a longer policy name")
	}
}

// Verifies: E2E-CORE-49.02, E2E-CORE-50.02
func TestPolicyOutputClassifiers(t *testing.T) {
	denial := cliDenial("Rejected by platform policy:\n- p: m")
	if !isPolicyRejection(denial) || !isPermissionDenied(denial) {
		t.Fatal("policy denial not classified")
	}
	if !isPolicyRejection(cliDenial("Rejected by 3 platform policies:\n- a\n- b\n- c")) {
		t.Fatal("multi-policy denial not classified")
	}
	failedClosed := "Error: code: 'The caller does not have permission to execute the specified operation', message: \"gateway interceptor 'hypershell-policy' failed closed: deadline has elapsed\""
	if !isInterceptorFailedClosed(failedClosed) || isPolicyRejection(failedClosed) || !isPermissionDenied(failedClosed) {
		t.Fatal("fail-closed outage must be PERMISSION_DENIED, failed closed, and not a policy decision")
	}
	nameLength := "Error: code: 'Client specified an invalid argument', message: \"name exceeds maximum length (28 > 19)\""
	if !isNameLengthRejection(nameLength) || isPolicyRejection(nameLength) || isPermissionDenied(nameLength) {
		t.Fatal("name-length refusal misclassified")
	}
	if isPolicyRejection("Created sandbox sb-1") {
		t.Fatal("success output classified as a policy rejection")
	}
}

// Verifies: E2E-CORE-49.01, E2E-CORE-49.02
func TestCanarySandboxNamesKeepReservedPrefix(t *testing.T) {
	token := sandboxToken()
	if len(token) != 6 {
		t.Fatalf("sandbox token %q is not 6 digits", token)
	}
	for _, suffix := range []string{"deny", "allow"} {
		name := canarySandboxName(token, suffix)
		if !strings.HasPrefix(name, canarySandboxPrefix) || !strings.HasSuffix(name, "-"+suffix) {
			t.Fatalf("canary sandbox name %q does not keep the reserved prefix and suffix", name)
		}
		// The reserved prefix alone exceeds the gateway's sandbox name limit; the
		// checks rely on the interceptor running before the name validation.
		if fitsGatewaySandboxName(name) {
			t.Fatalf("%q unexpectedly fits the %d-character gateway limit; revisit the name-limit handling", name, gatewaySandboxNameMax)
		}
	}
	if len(canarySandboxPrefix) <= gatewaySandboxNameMax {
		t.Fatalf("reserved prefix %q now fits the gateway limit; the allow case can assert success", canarySandboxPrefix)
	}
	if !fitsGatewaySandboxName("pe-123456-12") {
		t.Fatal("long-mode attempt names must fit the gateway limit")
	}
}

// Verifies: E2E-CORE-49.01, E2E-CORE-49.03
func TestClassifyCanary(t *testing.T) {
	canary := effectivePolicy{Name: canaryPolicyName, EffectiveMode: policyModeEnforce, Condition: policyConditionApplied}
	for name, tt := range map[string]struct {
		policies []effectivePolicy
		want     canaryState
	}{
		"applied":     {[]effectivePolicy{{Name: "other"}, canary}, canaryApplied},
		"pending":     {[]effectivePolicy{{Name: canaryPolicyName, EffectiveMode: policyModeEnforce, Condition: "Pending"}}, canaryPending},
		"missing":     {[]effectivePolicy{{Name: "other", EffectiveMode: policyModeEnforce, Condition: policyConditionApplied}}, canaryMissing},
		"audit":       {[]effectivePolicy{{Name: canaryPolicyName, EffectiveMode: policyModeAudit, Condition: policyConditionApplied}}, canaryNotEnforced},
		"unsupported": {[]effectivePolicy{{Name: canaryPolicyName, EffectiveMode: policyModeEnforce, Condition: policyConditionUnsupported}}, canaryUnsupported},
	} {
		t.Run(name, func(t *testing.T) {
			if _, got := classifyCanary(effectivePolicies{Policies: tt.policies}); got != tt.want {
				t.Fatalf("classifyCanary = %s, want %s", got, tt.want)
			}
		})
	}
}

// Verifies: E2E-CORE-49.03, PLAT-PGP-20.02
func TestCanaryDisabledStatusMessage(t *testing.T) {
	const disabled = "disabled: HYPERSHELL_POLICY_CANARY_ENABLED=false in this environment"
	list := []byte(`{"name":"e2e-enforcement-canary","status":{"gateways":[{"gateway_id":"g0","message":""},{"gateway_id":"g1","condition":"Pending","message":"` + disabled + `"}]}}`)
	msg, ok := policyGatewayStatusMessage(list, "g1")
	if !ok || !canaryDisabledByMessage(msg) {
		t.Fatalf("disabled message not found in list status: %q %t", msg, ok)
	}
	if msg, ok := policyGatewayStatusMessage(list, "g0"); !ok || canaryDisabledByMessage(msg) {
		t.Fatalf("gateway g0 must not read as disabled: %q %t", msg, ok)
	}
	if _, ok := policyGatewayStatusMessage(list, "missing"); ok {
		t.Fatal("absent gateway reported a status")
	}
	byID := []byte(`{"status":{"gateways":{"g1":{"message":"` + disabled + `"}}}}`)
	if msg, ok := policyGatewayStatusMessage(byID, "g1"); !ok || !canaryDisabledByMessage(msg) {
		t.Fatalf("disabled message not found in map status: %q %t", msg, ok)
	}
	if _, ok := policyGatewayStatusMessage([]byte(`{"status":{}}`), "g1"); ok {
		t.Fatal("empty status reported a message")
	}
	if canaryDisabledByMessage("bundle revision r3 not acknowledged") {
		t.Fatal("a non-disabled pending message read as disabled")
	}
}

// Verifies: E2E-CORE-49.02
func TestParseCgroupLimitBytes(t *testing.T) {
	if got, ok := parseCgroupLimitBytes("4294967296\n"); !ok || got != 4<<30 {
		t.Fatalf("parse memory.max = %d %t", got, ok)
	}
	if got, ok := parseCgroupLimitBytes("\x1b[0m$ sh -c ...\n1073741824\n\n"); !ok || got != 1<<30 {
		t.Fatalf("parse last line = %d %t", got, ok)
	}
	for _, out := range []string{"max\n", "", "cat: /sys/fs/cgroup/memory.max: Permission denied", "-1"} {
		if _, ok := parseCgroupLimitBytes(out); ok {
			t.Fatalf("parseCgroupLimitBytes(%q) reported a limit", out)
		}
	}
}

// Verifies: E2E-CORE-50.01
func TestHasFinding(t *testing.T) {
	since := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	findings := []policyFinding{
		{PolicyNames: []string{"a", "b"}, Outcome: findingOutcomeWouldReject, OccurredAt: since.Add(time.Minute)},
		{PolicyNames: []string{"c"}, Outcome: "would_add_defaults", OccurredAt: since.Add(time.Minute)},
		{PolicyNames: []string{"d"}, Outcome: findingOutcomeWouldReject, OccurredAt: since.Add(-time.Minute)},
	}
	if !hasFinding(findings, "b", findingOutcomeWouldReject, since) {
		t.Fatal("finding for b not found")
	}
	if hasFinding(findings, "c", findingOutcomeWouldReject, since) {
		t.Fatal("outcome mismatch accepted")
	}
	if hasFinding(findings, "d", findingOutcomeWouldReject, since) {
		t.Fatal("finding before the run accepted")
	}
}

// Verifies: E2E-CORE-50.01
func TestPodRestartFingerprint(t *testing.T) {
	pod := func(name, uid string, restarts, sidecarRestarts int32) corev1.Pod {
		return corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, UID: ktypes.UID(uid)},
			Status: corev1.PodStatus{
				ContainerStatuses:     []corev1.ContainerStatus{{Name: "openshell-gateway", RestartCount: restarts}},
				InitContainerStatuses: []corev1.ContainerStatus{{Name: policyInterceptorContainer, RestartCount: sidecarRestarts}},
			},
		}
	}
	before := podRestartFingerprint([]corev1.Pod{pod("gw-a", "u1", 0, 0)})
	if before != podRestartFingerprint([]corev1.Pod{pod("gw-a", "u1", 0, 0)}) {
		t.Fatal("unchanged pod changed fingerprint")
	}
	for name, after := range map[string]corev1.Pod{
		"new pod":           pod("gw-b", "u2", 0, 0),
		"gateway restarted": pod("gw-a", "u1", 1, 0),
		"sidecar restarted": pod("gw-a", "u1", 0, 1),
	} {
		if before == podRestartFingerprint([]corev1.Pod{after}) {
			t.Fatalf("%s not detected", name)
		}
	}
}

// Verifies: E2E-CORE-50.02
func TestContainerdID(t *testing.T) {
	if id, ok := containerdID("containerd://abc123"); !ok || id != "abc123" {
		t.Fatalf("containerdID = %q %t", id, ok)
	}
	for _, in := range []string{"cri-o://abc", "containerd://", ""} {
		if _, ok := containerdID(in); ok {
			t.Fatalf("containerdID(%q) accepted", in)
		}
	}
}

// Verifies: E2E-CORE-50.02
func TestSandboxNamesFromList(t *testing.T) {
	names := sandboxNamesFromList("\x1b[1mpk-1\x1b[0m\n  sb-2  \n\n")
	if _, ok := names["pk-1"]; !ok {
		t.Fatal("pk-1 not parsed")
	}
	if _, ok := names["sb-2"]; !ok || len(names) != 2 {
		t.Fatalf("names = %v", names)
	}
}

// Verifies: E2E-CORE-50.02
func TestPolicyStepIsLongOnly(t *testing.T) {
	for _, mode := range []string{modeShort, modePerf, modeLong} {
		s := &E2ESuite{mode: mode}
		var found bool
		for _, step := range s.p2ParallelSteps() {
			if step.name != "P2.12-platform-gateway-policies" {
				continue
			}
			found = true
			if step.tag != tagLong {
				t.Fatalf("P2.12 tag = %q, want %q", step.tag, tagLong)
			}
			if got, want := s.modeAllows(step.tag), mode == modeLong; got != want {
				t.Fatalf("P2.12 runs in %s mode = %t, want %t (it must t.Skip in short and perf)", mode, got, want)
			}
		}
		if !found {
			t.Fatal("P2.12-platform-gateway-policies is not registered")
		}
	}
	var planned bool
	for _, step := range plannedReportSteps() {
		if step.StepID == "P2.12" && step.Name == "platform-gateway-policies" {
			planned = true
		}
	}
	if !planned {
		t.Fatal("P2.12 is missing from the planned report steps")
	}
}
