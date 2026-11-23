package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	sdktypes "github.com/openshift-online/hypershell/components/sdk-go/types"
	"github.com/openshift-online/hypershell/tests/e2e/apiclient"
	"github.com/openshift-online/hypershell/tests/e2e/driver"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// This file holds the platform gateway policy e2e coverage: the short-mode
// enforcement canary inside P2.1 (E2E-CORE-49, PLAT-PGP-20) and the long-only
// P2.12 policy step (E2E-CORE-50).

const (
	// canaryPolicyName is the built-in policy seeded by the api-server migration
	// (PLAT-PGP-20). It is enforced on every gateway and forbids more than 1 CPU
	// for sandboxes whose name starts with canarySandboxPrefix.
	canaryPolicyName = "e2e-enforcement-canary"
	// canarySandboxPrefix is the reserved sandbox-name prefix the canary applies
	// to. It is 22 characters long.
	canarySandboxPrefix = "hypershell-e2e-policy-"
	// gatewaySandboxNameMax is the OpenShell gateway's sandbox name limit
	// (MAX_ROUTABLE_NAME_LEN in openshell-server at the pinned release). The
	// gateway enforces it in the CreateSandbox handler, AFTER the gateway
	// interceptors run, so a reserved-prefix name (always longer than 19) still
	// reaches the policy interceptor and can be denied by the canary, but a
	// reserved-prefix create the canary allows is then refused by the name check.
	gatewaySandboxNameMax = 19
	// canaryDisabledMarker is the controller setting named in the canary's
	// per-gateway status message when it is disabled in an environment
	// ("disabled: HYPERSHELL_POLICY_CANARY_ENABLED=false in this environment").
	canaryDisabledMarker = "HYPERSHELL_POLICY_CANARY_ENABLED=false"
	// canaryCPUBound is the bound text the interceptor's display renderer adds to
	// a denial by a `context.cpuRequestMillis > 1000` statement ("(CPU request is
	// 2; allowed: at most 1)"). The "allowed: " part matters: the canary's user
	// message itself says "at most 1 CPU", so the bare phrase proves nothing.
	canaryCPUBound = "allowed: at most 1"
	// collateralSandboxCPU is the CPU request of the P2.1 sandbox that does NOT
	// use the reserved prefix; the canary must not reject it (E2E-CORE-49.02).
	collateralSandboxCPU = "2"
	// policyPropagationSLO bounds how long an enforcement-mode change may take to
	// reach a running gateway's interceptor, measured from the PATCH to the first
	// rejected create (PLAT-PGP-06, PLAT-PGP-12, E2E-CORE-50.01).
	policyPropagationSLO = 60 * time.Second

	policyConditionApplied     = "Applied"
	policyConditionUnsupported = "Unsupported"
	policyModeEnforce          = "enforce"
	policyModeAudit            = "audit"
	findingOutcomeWouldReject  = "would_reject"
	platformPolicyAdminRole    = "platform-policy:admin"
	policyInterceptorContainer = "hypershell-policy-interceptor"

	// cgroupMemoryLimitCommand prints the sandbox container's memory limit on
	// cgroup v2 and falls back to cgroup v1.
	cgroupMemoryLimitCommand = "cat /sys/fs/cgroup/memory.max 2>/dev/null || cat /sys/fs/cgroup/memory/memory.limit_in_bytes"
)

// --- API shapes (decoded locally so the checks do not depend on generated
// SDK types for the sub-resource the typed SDK does not model) ---

type fleetDefaults struct {
	CPURequest         string `json:"cpu_request"`
	CPULimit           string `json:"cpu_limit"`
	MemoryRequest      string `json:"memory_request"`
	MemoryLimit        string `json:"memory_limit"`
	CPURequestMillis   int64  `json:"cpu_request_millis"`
	CPULimitMillis     int64  `json:"cpu_limit_millis"`
	MemoryRequestBytes int64  `json:"memory_request_bytes"`
	MemoryLimitBytes   int64  `json:"memory_limit_bytes"`
}

type effectivePolicy struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	UserMessage     string          `json:"user_message"`
	EnforcementMode string          `json:"enforcement_mode"`
	EffectiveMode   string          `json:"effective_mode"`
	BuiltIn         bool            `json:"built_in"`
	Condition       string          `json:"condition"`
	Revision        string          `json:"revision"`
	Target          json.RawMessage `json:"target,omitempty"`
}

type policyFinding struct {
	ID          string    `json:"id"`
	OccurredAt  time.Time `json:"occurred_at"`
	GatewayID   string    `json:"gateway_id"`
	PolicyNames []string  `json:"policy_names"`
	Operation   string    `json:"operation"`
	Outcome     string    `json:"outcome"`
}

// effectivePolicies is GET /gateways/{id}/effective_policies (PLAT-PGP-19).
type effectivePolicies struct {
	GatewayID  string            `json:"gateway_id"`
	GlobalMode string            `json:"global_mode"`
	Defaults   fleetDefaults     `json:"defaults"`
	Policies   []effectivePolicy `json:"policies"`
	Findings   []policyFinding   `json:"findings"`
}

func (e effectivePolicies) policy(name string) (effectivePolicy, bool) {
	for _, p := range e.Policies {
		if p.Name == name {
			return p, true
		}
	}
	return effectivePolicy{}, false
}

type policyTarget struct {
	All        bool     `json:"all,omitempty"`
	GatewayIDs []string `json:"gateway_ids,omitempty"`
}

// policyWrite is the PlatformGatewayPolicy create body.
type policyWrite struct {
	Name        string        `json:"name"`
	Cedar       string        `json:"cedar"`
	UserMessage string        `json:"user_message,omitempty"`
	Description string        `json:"description,omitempty"`
	Target      *policyTarget `json:"target,omitempty"`
}

// --- pure helpers (unit-tested in policy_phases_test.go) ---

// canaryState classifies the canary as seen through effective_policies.
type canaryState int

const (
	canaryApplied canaryState = iota
	canaryPending
	canaryMissing
	canaryNotEnforced
	canaryUnsupported
)

func (c canaryState) String() string {
	switch c {
	case canaryApplied:
		return "applied"
	case canaryPending:
		return "pending"
	case canaryMissing:
		return "missing"
	case canaryNotEnforced:
		return "not-enforced"
	case canaryUnsupported:
		return "unsupported"
	default:
		return "unknown"
	}
}

// classifyCanary reports the canary's state on a gateway. Only an Applied,
// enforced canary lets the short-mode policy checks run; a missing, audit-only,
// or Unsupported canary is a failure, and a Pending one is either a disabled
// canary (skip) or a broken bundle path (fail), decided by its status message.
func classifyCanary(ep effectivePolicies) (effectivePolicy, canaryState) {
	p, ok := ep.policy(canaryPolicyName)
	switch {
	case !ok:
		return p, canaryMissing
	case p.EffectiveMode != policyModeEnforce:
		return p, canaryNotEnforced
	case p.Condition == policyConditionUnsupported:
		return p, canaryUnsupported
	case p.Condition == policyConditionApplied:
		return p, canaryApplied
	default:
		return p, canaryPending
	}
}

// policyGatewayStatusMessage returns the per-gateway status message from a
// PlatformGatewayPolicy GET body. status.gateways is a list of
// {gateway_id, message, ...}; a map keyed by gateway id is accepted too.
func policyGatewayStatusMessage(body []byte, gatewayID string) (string, bool) {
	var doc struct {
		Status struct {
			Gateways json.RawMessage `json:"gateways"`
		} `json:"status"`
	}
	if err := json.Unmarshal(body, &doc); err != nil || len(doc.Status.Gateways) == 0 {
		return "", false
	}
	type entry struct {
		GatewayID string `json:"gateway_id"`
		Message   string `json:"message"`
	}
	var list []entry
	if err := json.Unmarshal(doc.Status.Gateways, &list); err == nil {
		for _, e := range list {
			if e.GatewayID == gatewayID {
				return e.Message, true
			}
		}
		return "", false
	}
	var byID map[string]entry
	if err := json.Unmarshal(doc.Status.Gateways, &byID); err == nil {
		if e, ok := byID[gatewayID]; ok {
			return e.Message, true
		}
	}
	return "", false
}

// canaryDisabledByMessage reports whether a canary status message says the
// canary is disabled by the controller setting (PLAT-PGP-20.02).
func canaryDisabledByMessage(message string) bool {
	return strings.Contains(message, canaryDisabledMarker)
}

// canarySandboxName returns a reserved-prefix sandbox name for the canary
// checks: hypershell-e2e-policy-<token>-<suffix>.
func canarySandboxName(token, suffix string) string {
	return canarySandboxPrefix + token + "-" + suffix
}

// fitsGatewaySandboxName reports whether name is within the gateway's sandbox
// name limit.
func fitsGatewaySandboxName(name string) bool { return len(name) <= gatewaySandboxNameMax }

// sandboxToken is a short per-check unique token for sandbox names, which must
// fit gatewaySandboxNameMax.
func sandboxToken() string { return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000) }

// cliStatusUnescaper undoes the Rust `{:?}` quoting the openshell CLI applies to
// a gRPC status message (tonic prints `message: "line1\nline2"`).
var cliStatusUnescaper = strings.NewReplacer(`\\`, `\`, `\n`, "\n", `\"`, `"`)

// normalizeCLIStatusText strips ANSI color and unescapes the quoted gRPC status
// message so a multi-line denial reason can be matched line by line.
func normalizeCLIStatusText(out string) string {
	return cliStatusUnescaper.Replace(stripANSI(out))
}

// isPermissionDenied reports whether CLI output carries a PERMISSION_DENIED
// gRPC status. tonic prints the code by its description ("The caller does not
// have permission to execute the specified operation"), other clients by name.
func isPermissionDenied(out string) bool {
	lower := strings.ToLower(normalizeCLIStatusText(out))
	for _, marker := range []string{"permission_denied", "permissiondenied", "permission denied", "does not have permission to execute"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

var policyRejectionRE = regexp.MustCompile(`Rejected by (\d+ )?platform polic(y|ies):`)

// isPolicyRejection reports whether CLI output carries a platform-policy denial
// reason (PLAT-PGP-14).
func isPolicyRejection(out string) bool {
	return policyRejectionRE.MatchString(normalizeCLIStatusText(out))
}

// isInterceptorFailedClosed reports whether CLI output carries the gateway's
// fail-closed status for an unavailable interceptor (PLAT-PGP-11).
func isInterceptorFailedClosed(out string) bool {
	return strings.Contains(normalizeCLIStatusText(out), "failed closed")
}

// isNameLengthRejection reports whether CLI output carries the gateway's
// sandbox-name length refusal.
func isNameLengthRejection(out string) bool {
	return strings.Contains(normalizeCLIStatusText(out), "name exceeds maximum length")
}

// policyDenialExpectation is one policy a denial must name.
type policyDenialExpectation struct {
	Name        string
	UserMessage string
	// Bound, when set, must appear on the policy's line (the renderer's
	// "<value>; allowed: <bound>" text).
	Bound string
}

// checkPolicyDenial verifies CLI output is a PERMISSION_DENIED platform-policy
// denial that names exactly the expected policies, each on its own line with
// its user message and (when set) its bound (PLAT-PGP-14.01).
func checkPolicyDenial(out string, want []policyDenialExpectation) error {
	if len(want) == 0 {
		return fmt.Errorf("no expected policies")
	}
	norm := normalizeCLIStatusText(out)
	if !isPermissionDenied(norm) {
		return fmt.Errorf("output is not a PERMISSION_DENIED status")
	}
	header := "Rejected by platform policy:"
	if len(want) > 1 {
		header = fmt.Sprintf("Rejected by %d platform policies:", len(want))
	}
	if !strings.Contains(norm, header) {
		return fmt.Errorf("output does not contain %q", header)
	}
	lines := strings.Split(norm, "\n")
	for _, w := range want {
		line, ok := policyDenialLine(lines, w.Name)
		if !ok {
			return fmt.Errorf("denial does not list policy %q", w.Name)
		}
		if msg := strings.TrimSpace(w.UserMessage); msg != "" && !strings.Contains(line, msg) {
			return fmt.Errorf("denial line for %q does not carry its user message %q: %q", w.Name, msg, line)
		}
		if w.Bound != "" && !strings.Contains(line, w.Bound) {
			return fmt.Errorf("denial line for %q does not state the bound %q: %q", w.Name, w.Bound, line)
		}
	}
	return nil
}

// policyDenialLine finds the "- <name>: ..." line for a policy.
func policyDenialLine(lines []string, name string) (string, bool) {
	for _, l := range lines {
		idx := strings.Index(l, "- "+name)
		if idx < 0 {
			continue
		}
		rest := l[idx+2+len(name):]
		if rest == "" || rest[0] == ':' || rest[0] == ' ' || rest[0] == '"' {
			return l[idx:], true
		}
	}
	return "", false
}

// parseCgroupLimitBytes parses a cgroup memory limit (memory.max or
// memory.limit_in_bytes). "max" (unlimited) and unparsable output report false.
func parseCgroupLimitBytes(out string) (int64, bool) {
	var last string
	for _, l := range strings.Split(stripANSI(out), "\n") {
		if t := strings.TrimSpace(l); t != "" {
			last = t
		}
	}
	if last == "" || last == "max" {
		return 0, false
	}
	n, err := strconv.ParseInt(last, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// hasFinding reports whether findings holds an outcome for policy recorded at
// or after since.
func hasFinding(findings []policyFinding, policy, outcome string, since time.Time) bool {
	for _, f := range findings {
		if f.Outcome != outcome || f.OccurredAt.Before(since) {
			continue
		}
		for _, n := range f.PolicyNames {
			if n == policy {
				return true
			}
		}
	}
	return false
}

// podRestartFingerprint summarizes pod identity and container restarts, so a
// before/after comparison detects a gateway restart (a new pod or a restarted
// container, init and sidecar containers included).
func podRestartFingerprint(pods []corev1.Pod) string {
	parts := make([]string, 0, len(pods))
	for _, p := range pods {
		var restarts int32
		for _, cs := range p.Status.ContainerStatuses {
			restarts += cs.RestartCount
		}
		for _, cs := range p.Status.InitContainerStatuses {
			restarts += cs.RestartCount
		}
		parts = append(parts, fmt.Sprintf("%s/%s:%d", p.Name, p.UID, restarts))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// sandboxNamesFromList parses `openshell sandbox list --names` output.
func sandboxNamesFromList(out string) map[string]struct{} {
	names := make(map[string]struct{})
	for _, line := range strings.Split(stripANSI(out), "\n") {
		if n := strings.TrimSpace(line); n != "" {
			names[n] = struct{}{}
		}
	}
	return names
}

// containerdID strips the runtime scheme from a container status id; only
// containerd ids are usable with the Kind node's ctr.
func containerdID(statusID string) (string, bool) {
	id, ok := strings.CutPrefix(statusID, "containerd://")
	return id, ok && id != ""
}

// --- suite helpers ---

func (s *E2ESuite) getEffectivePolicies(ctx context.Context, api *apiclient.Client, gatewayID string) (effectivePolicies, int, []byte, error) {
	var ep effectivePolicies
	status, body, err := api.RawJSON(ctx, http.MethodGet, "/gateways/"+gatewayID+"/effective_policies", nil)
	if err != nil || status != http.StatusOK {
		return ep, status, body, err
	}
	if err := json.Unmarshal(body, &ep); err != nil {
		return ep, status, body, fmt.Errorf("decode effective_policies: %w", err)
	}
	return ep, status, body, nil
}

// sandboxListed reports whether name is in the gateway's sandbox list.
func (s *E2ESuite) sandboxListed(t *testing.T, local, name string) (bool, error) {
	out, err := s.cli(t, local, "sandbox", "list", "--names")
	if err != nil {
		return false, fmt.Errorf("sandbox list: %w", err)
	}
	_, ok := sandboxNamesFromList(out)[name]
	return ok, nil
}

// waitSandboxListed polls the sandbox list for up to timeout. `sandbox create`
// can return a stream error while the image pulls even though the sandbox was
// created, so existence, not the exit status, decides admission.
func (s *E2ESuite) waitSandboxListed(t *testing.T, local, name string, timeout time.Duration) bool {
	return pollNoCtx(timeout, 3*time.Second, func() bool {
		ok, err := s.sandboxListed(t, local, name)
		return err == nil && ok
	}) == nil
}

// deleteSandboxIfPresent stops and deletes name when it exists. Cleanup only:
// errors are logged.
func (s *E2ESuite) deleteSandboxIfPresent(t *testing.T, local, name string) {
	ok, err := s.sandboxListed(t, local, name)
	if err != nil || !ok {
		return
	}
	for _, args := range sandboxCLISteps("delete", name, s.sandboxImage) {
		if out, err := s.cli(t, local, args...); err != nil {
			s.Logf("cleanup %q returned an error: %v\n%s", args, err, out)
		}
	}
}

// --- E2E-CORE-49: short-mode enforcement canary (inside P2.1) ---

// policyCanaryChecks runs the short-mode policy checks on the P2.1 gateway as a
// named subtest, so a disabled canary reports skipped, never passed. It acts as
// the run's own identity only, uses only the API and the CLI (the pod-spec
// read-back is a long-only sub-check), and mutates no shared policy setting.
// collateral is the P2.1 sandbox created without the reserved prefix and with a
// CPU request of collateralSandboxCPU.
//
// Verifies: E2E-CORE-49.01, E2E-CORE-49.02, E2E-CORE-49.03, PLAT-PGP-20.01, PLAT-PGP-20.02
func (s *E2ESuite) policyCanaryChecks(t *testing.T, gw *sdktypes.Gateway, local, collateral string) {
	s.Run("policy-canary", func() {
		t := s.T()
		ctx := t.Context()
		timeout := durationSecondsEnv("E2E_PROVISION_TIMEOUT", 180*time.Second)

		// 1. Bundle applied: the canary is listed, enforced, and Applied.
		var ep effectivePolicies
		var canary effectivePolicy
		state := canaryMissing
		var lastErr error
		_ = harness.Poll(ctx, 5*time.Second, timeout, func(ctx context.Context) (bool, error) {
			got, status, body, err := s.getEffectivePolicies(ctx, s.admin, gw.ID)
			if err != nil || status != http.StatusOK {
				lastErr = fmt.Errorf("effective_policies: status %d err %v body %s", status, err, string(body))
				return false, nil
			}
			lastErr = nil
			ep = got
			canary, state = classifyCanary(got)
			return state == canaryApplied || state == canaryUnsupported, nil
		})
		s.Require().NoError(lastErr, "read effective_policies for gateway %s", gw.ID)
		switch state {
		case canaryApplied:
		case canaryPending:
			s.skipOrFailPendingCanary(t, canary, gw.ID, timeout)
		default:
			s.Require().Failf("canary not usable", "%s on gateway %s is %s (effective_mode=%q condition=%q); it is seeded enforced with target all (PLAT-PGP-20)",
				canaryPolicyName, gw.ID, state, canary.EffectiveMode, canary.Condition)
		}
		s.Logf("%s is enforced and Applied on gateway %s (revision %q)", canaryPolicyName, gw.ID, canary.Revision)

		token := sandboxToken()

		// 2. Rejection: a reserved-prefix create requesting 2 CPUs is denied by the
		// canary and the reason reaches the CLI. The gateway runs interceptors
		// before its own name validation, so the 19-character name limit does not
		// mask the denial.
		denyName := canarySandboxName(token, "deny")
		defer s.deleteSandboxIfPresent(t, local, denyName)
		out, err := s.cli(t, local, "sandbox", "create", "--from", s.sandboxImage, "--name", denyName, "--cpu", collateralSandboxCPU)
		s.Require().Errorf(err, "E2E-CORE-49.01: %s with --cpu %s must be rejected by %s; output:\n%s", denyName, collateralSandboxCPU, canaryPolicyName, out)
		s.Require().NoErrorf(checkPolicyDenial(out, []policyDenialExpectation{{
			Name: canaryPolicyName, UserMessage: canary.UserMessage, Bound: canaryCPUBound,
		}}), "E2E-CORE-49.01: denial text for %s; output:\n%s", denyName, out)

		// 3. Allowed: a reserved-prefix create with no resources passes the canary
		// (the injected default request, 500m on Kind, is within 1 CPU).
		s.assertCanaryAllowsDefaults(t, local, canarySandboxName(token, "allow"))

		// Default injection is observable on the collateral sandbox, which set only
		// a CPU value: the interceptor adds the fleet default memory request and
		// limit (modify_operation, PLAT-PGP-08).
		s.assertDefaultInjection(t, gw, local, collateral, ep.Defaults)

		// 4. No collateral rejection: asserted when P2.1 created the collateral
		// sandbox (--cpu 2, no reserved prefix) and reached it with exec.
		s.Logf("sandbox %s (no reserved prefix, --cpu %s) was admitted and reached with exec; the canary did not reject it", collateral, collateralSandboxCPU)
	})
}

// skipOrFailPendingCanary decides a canary that never reached Applied: skipped
// when its status says the controller setting disabled it, or when this
// identity cannot read the status (a live-environment minimal identity), and a
// failure when the status is readable and does not say disabled. It never
// returns normally (E2E-CORE-49.03).
func (s *E2ESuite) skipOrFailPendingCanary(t *testing.T, canary effectivePolicy, gatewayID string, timeout time.Duration) {
	status, body, err := s.admin.RawExtJSON(t.Context(), http.MethodGet, "/platform_gateway_policies/"+canary.ID, nil)
	if err != nil || status != http.StatusOK {
		t.Skipf("E2E-CORE-49.03: %s is listed on gateway %s but not Applied within %s (condition %q); this identity cannot read its status (HTTP %d, err %v), so a disabled canary cannot be told apart from a broken bundle path; the policy check is reported skipped, not passed",
			canaryPolicyName, gatewayID, timeout, canary.Condition, status, err)
	}
	message, found := policyGatewayStatusMessage(body, gatewayID)
	if found && canaryDisabledByMessage(message) {
		t.Skipf("E2E-CORE-49.03: %s is disabled in this environment (%q); the policy check is reported skipped, not passed", canaryPolicyName, message)
	}
	s.Require().Failf("canary not applied", "%s never reached Applied on gateway %s within %s (condition %q, status message %q); the bundle path or the sidecar acknowledgement is broken",
		canaryPolicyName, gatewayID, timeout, canary.Condition, message)
}

// assertCanaryAllowsDefaults creates a reserved-prefix sandbox with no
// resources. The canary must not reject it. At the pinned OpenShell release the
// reserved prefix alone (22 characters) exceeds the gateway's 19-character
// sandbox-name limit, which the gateway checks after the interceptors allow the
// call, so the create then fails with the name-length refusal. That outcome
// still proves the interceptor allowed the request; the success-and-read-back
// half is only reachable when the name fits, and the default-injection read-back
// therefore runs on the collateral sandbox instead.
func (s *E2ESuite) assertCanaryAllowsDefaults(t *testing.T, local, allowName string) {
	defer s.deleteSandboxIfPresent(t, local, allowName)
	out, err := s.cli(t, local, "sandbox", "create", "--from", s.sandboxImage, "--name", allowName)
	s.Require().Falsef(isPolicyRejection(out), "E2E-CORE-49.02: %s with no resources was rejected by a platform policy:\n%s", allowName, out)
	s.Require().Falsef(isInterceptorFailedClosed(out), "E2E-CORE-49.02: the policy interceptor failed closed for %s:\n%s", allowName, out)
	if err != nil && isNameLengthRejection(out) && !fitsGatewaySandboxName(allowName) {
		s.Logf("E2E-CORE-49.02 name-limit conflict: %q is %d characters; the reserved prefix %q is %d characters and the gateway limits sandbox names to %d. The canary allowed the create (no policy rejection), then the gateway's name validation refused it, so the reserved-prefix sandbox cannot exist on this gateway release; default injection is read back on the collateral sandbox.",
			allowName, len(allowName), canarySandboxPrefix, len(canarySandboxPrefix), gatewaySandboxNameMax)
		return
	}
	s.Require().Truef(s.waitSandboxListed(t, local, allowName, 60*time.Second),
		"E2E-CORE-49.02: %s with no resources was neither created nor refused for its name length (err %v):\n%s", allowName, err, out)
	s.Logf("%s with no resources was admitted by the canary", allowName)
}

// assertDefaultInjection proves the interceptor added the fleet default memory
// limit to a sandbox created without memory resources, by reading the cgroup
// limit through the CLI. When the limit is not observable through the CLI the
// sub-check reports skipped. Long mode additionally reads the pod spec.
func (s *E2ESuite) assertDefaultInjection(t *testing.T, gw *sdktypes.Gateway, local, sandbox string, defaults fleetDefaults) {
	s.Run("default-injection", func() {
		t := s.T()
		s.Require().Positivef(defaults.MemoryLimitBytes, "effective_policies defaults carry memory_limit_bytes (got %+v)", defaults)
		out, err := s.cli(t, local, "sandbox", "exec", "-n", sandbox, "--", "sh", "-c", cgroupMemoryLimitCommand)
		if err != nil {
			t.Skipf("the sandbox memory limit is not readable through the CLI (%v); default injection is not observable in short mode:\n%s", err, out)
		}
		got, ok := parseCgroupLimitBytes(out)
		if !ok {
			t.Skipf("the sandbox cgroup memory limit is not a byte count (%q); default injection is not observable in short mode", strings.TrimSpace(stripANSI(out)))
		}
		s.Assert().Equalf(defaults.MemoryLimitBytes, got,
			"E2E-CORE-49.02: sandbox %s set no memory, so its limit must be the fleet default %s (%d bytes)", sandbox, defaults.MemoryLimit, defaults.MemoryLimitBytes)

		s.longOnly("pod-spec-defaults", func(t *testing.T) {
			ns := sandboxPodNamespace(gw.WorkspaceMode, gw.Namespace, "default")
			podName := sandboxPodName(gw.WorkspaceMode, "default", sandbox)
			pods, err := s.clients.Kube.CoreV1().Pods(ns).List(t.Context(), metav1.ListOptions{})
			s.Require().NoError(err, "list sandbox pods in %s", ns)
			var found bool
			for _, p := range pods.Items {
				if !strings.Contains(p.Name, podName) {
					continue
				}
				for _, c := range p.Spec.Containers {
					if c.Image != s.sandboxImage {
						continue
					}
					found = true
					s.Assert().Equalf(defaults.MemoryRequestBytes, c.Resources.Requests.Memory().Value(), "pod %s memory request is the fleet default %s", p.Name, defaults.MemoryRequest)
					s.Assert().Equalf(defaults.MemoryLimitBytes, c.Resources.Limits.Memory().Value(), "pod %s memory limit is the fleet default %s", p.Name, defaults.MemoryLimit)
				}
			}
			s.Require().Truef(found, "sandbox pod %s with workload image %s in %s", podName, s.sandboxImage, ns)
		})
	})
}

// --- E2E-CORE-50: long-mode policy coverage (P2.12) ---

// p2_12PlatformGatewayPolicies is the long-only policy step. It owns a dedicated
// gateway so its policies (targeted at that gateway only) and its sidecar
// outage never affect another step, and acts as a platform-policy:admin granted
// through the RoleBinding API by the platform admin, plus the developer as a
// gateway user.
//
// Verifies: E2E-CORE-50.01, E2E-CORE-50.02, PLAT-PGP-14.01
func (s *E2ESuite) p2_12PlatformGatewayPolicies(t *testing.T) {
	if s.driver.Name() != "kind" || envOrDefault("E2E_OIDC_GRANT", "password") != "password" {
		t.Skip("the policy-admin and gateway-user identities require the Kind password-grant environment (runtime test users)")
	}
	ctx := t.Context()
	token := sandboxToken()
	started := time.Now().Add(-time.Minute) // tolerate clock skew between runner and cluster

	gw := s.createGateway(t, "e2e-pgp-"+s.runID)
	s.trackGateway(gw.ID)
	running := s.waitGatewayRunning(t, gw.ID)
	ref := driver.GatewayRef{Name: running.Name, Namespace: running.Namespace, ID: running.ID, ClientID: s.requireGatewayClientID(running)}
	endpoint, err := gatewayEndpointFromRouteAddress(running.RouteAddress)
	s.Require().NoError(err, "derive policy gateway endpoint from API route_address")
	tok := s.requireGatewayToken(t, ctx, ref, "policy gateway admin token")
	local := s.registerGateway(t, ref, endpoint, tok.AccessToken)
	s.cliStatusConnected(t, local, durationSecondsEnv("E2E_CONNECT_TIMEOUT", 90*time.Second))

	adminCreds, cleanupAdmin := s.grantPolicyAdmin(t)
	defer cleanupAdmin()
	policyAPI := func() *apiclient.Client { return s.apiClientForCreds(t, adminCreds) }

	var createdPolicies []string
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		api, err := s.cleanupAPIClient(cleanupCtx, adminCreds)
		if err != nil {
			t.Errorf("policy cleanup: %v (policies left: %v)", err, createdPolicies)
			return
		}
		for _, id := range createdPolicies {
			status, body, err := api.RawExtJSON(cleanupCtx, http.MethodDelete, "/platform_gateway_policies/"+id, nil)
			if err != nil || (status != http.StatusNoContent && status != http.StatusNotFound) {
				t.Errorf("delete policy %s: status %d err %v body %s", id, status, err, string(body))
			}
		}
	}()
	create := func(body policyWrite) (int, string, []byte) {
		status, resp, err := policyAPI().RawExtJSON(ctx, http.MethodPost, "/platform_gateway_policies", body)
		s.Require().NoErrorf(err, "create policy %s", body.Name)
		var created struct {
			ID string `json:"id"`
		}
		if status == http.StatusCreated && json.Unmarshal(resp, &created) == nil && created.ID != "" {
			createdPolicies = append(createdPolicies, created.ID)
		}
		return status, created.ID, resp
	}
	target := &policyTarget{GatewayIDs: []string{gw.ID}}

	// E2E-CORE-50.02 (validation, PLAT-PGP-04): a permit, an unknown attribute,
	// and a policy that forbids the fleet defaults are rejected at write time.
	for _, bad := range []policyWrite{
		{Name: "e2e-pgp-permit-" + token, Cedar: `permit (principal, action, resource);`},
		{Name: "e2e-pgp-unknown-" + token, Cedar: `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource) when { context.hypershellE2eUnknown > 1 };`},
		{Name: "e2e-pgp-defaults-" + token, Cedar: `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource) when { context.cpuRequestMillis >= 1 };`},
	} {
		bad.Target = target
		bad.UserMessage = "HyperShell e2e invalid policy; must be rejected at write time."
		status, _, body := create(bad)
		s.Assert().Equalf(http.StatusBadRequest, status, "E2E-CORE-50.02: invalid policy %s must be rejected with 400 (body: %s)", bad.Name, string(body))
	}

	// An audit policy is only audit-effective while the global toggle is audit;
	// this step does not move the shared toggle.
	current, status, body, err := s.getEffectivePolicies(ctx, policyAPI(), gw.ID)
	s.Require().NoError(err, "read effective_policies")
	s.Require().Equalf(http.StatusOK, status, "policy admin reads effective_policies (body: %s)", string(body))
	if current.GlobalMode == policyModeEnforce {
		t.Skip("the global policy mode is enforce, so an audit policy is enforced; the audit-to-enforce coverage needs the global mode at audit and this step does not change the shared setting")
	}

	// Restart evidence before any policy reaches this gateway.
	podsBefore := s.gatewayPodFingerprint(t, running.Namespace)

	// E2E-CORE-50.01: create in audit, run a violating create, and see it allowed
	// and recorded as a would_reject finding.
	cpuPolicy := policyWrite{
		Name:        "e2e-pgp-cpu-" + token,
		Cedar:       `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource) when { context.cpuRequestMillis > 1000 };`,
		UserMessage: "HyperShell e2e: sandboxes on this gateway may request at most 1 CPU.",
		Target:      target,
	}
	status, cpuPolicyID, body := create(cpuPolicy)
	s.Require().Equalf(http.StatusCreated, status, "create audit policy (body: %s)", string(body))
	s.waitPolicyCondition(t, policyAPI(), gw.ID, cpuPolicy.Name, policyModeAudit, durationSecondsEnv("E2E_PROVISION_TIMEOUT", 180*time.Second))

	auditName := "pa-" + token
	out, err := s.cli(t, local, "sandbox", "create", "--from", s.sandboxImage, "--name", auditName, "--cpu", "1500m")
	s.Require().Falsef(isPolicyRejection(out), "E2E-CORE-50.01: an audit-mode policy must not reject %s:\n%s", auditName, out)
	s.Require().Truef(s.waitSandboxListed(t, local, auditName, 60*time.Second), "E2E-CORE-50.01: audit-mode violating create %s must be admitted (err %v):\n%s", auditName, err, out)
	s.deleteSandboxIfPresent(t, local, auditName)

	findingsTimeout := durationSecondsEnv("E2E_POLICY_FINDINGS_TIMEOUT", 120*time.Second)
	findErr := harness.Poll(ctx, 5*time.Second, findingsTimeout, func(ctx context.Context) (bool, error) {
		ep, status, _, err := s.getEffectivePolicies(ctx, policyAPI(), gw.ID)
		return err == nil && status == http.StatusOK && hasFinding(ep.Findings, cpuPolicy.Name, findingOutcomeWouldReject, started), nil
	})
	s.Require().NoErrorf(findErr, "E2E-CORE-50.01: a %s finding for %s must appear in effective_policies within %s", findingOutcomeWouldReject, cpuPolicy.Name, findingsTimeout)

	// Switch to enforce: the next violating create is rejected within the
	// propagation SLO, without a gateway restart.
	patchStatus, patchBody, err := policyAPI().RawExtJSON(ctx, http.MethodPatch, "/platform_gateway_policies/"+cpuPolicyID, map[string]string{"enforcement_mode": policyModeEnforce})
	s.Require().NoError(err, "patch policy to enforce")
	s.Require().Equalf(http.StatusOK, patchStatus, "patch policy to enforce (body: %s)", string(patchBody))
	enforcedAt := time.Now()
	s.waitPolicyCondition(t, policyAPI(), gw.ID, cpuPolicy.Name, policyModeEnforce, policyPropagationSLO)
	denyOut := s.createUntilPolicyDenied(t, local, "pe-"+token, enforcedAt.Add(policyPropagationSLO), isPolicyRejection, "--cpu", "1500m")
	elapsed := time.Since(enforcedAt)
	s.Require().NoErrorf(checkPolicyDenial(denyOut, []policyDenialExpectation{{Name: cpuPolicy.Name, UserMessage: cpuPolicy.UserMessage, Bound: canaryCPUBound}}),
		"E2E-CORE-50.01: enforced denial text:\n%s", denyOut)
	s.Assert().LessOrEqualf(elapsed, policyPropagationSLO, "E2E-CORE-50.01: enforcement reached the gateway in %s; the propagation SLO is %s", elapsed, policyPropagationSLO)
	s.Assert().Equalf(podsBefore, s.gatewayPodFingerprint(t, running.Namespace), "E2E-CORE-50.01: the gateway must not restart for a policy change (pods and restart counts)")
	s.Logf("audit -> enforce reached gateway %s in %s without a restart", gw.ID, elapsed.Round(time.Second))

	// PLAT-PGP-14.01: a second enforced policy; a request violating both lists both.
	memPolicy := policyWrite{
		Name:        "e2e-pgp-mem-" + token,
		Cedar:       `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource) when { context.memoryLimitBytes > 6442450944 };`,
		UserMessage: "HyperShell e2e: sandboxes on this gateway may use at most 6Gi of memory.",
		Target:      target,
	}
	status, memPolicyID, body := create(memPolicy)
	s.Require().Equalf(http.StatusCreated, status, "create second policy (body: %s)", string(body))
	patchStatus, patchBody, err = policyAPI().RawExtJSON(ctx, http.MethodPatch, "/platform_gateway_policies/"+memPolicyID, map[string]string{"enforcement_mode": policyModeEnforce})
	s.Require().NoError(err, "patch second policy to enforce")
	s.Require().Equalf(http.StatusOK, patchStatus, "patch second policy to enforce (body: %s)", string(patchBody))
	s.waitPolicyCondition(t, policyAPI(), gw.ID, memPolicy.Name, policyModeEnforce, policyPropagationSLO)
	both := []policyDenialExpectation{
		{Name: cpuPolicy.Name, UserMessage: cpuPolicy.UserMessage},
		{Name: memPolicy.Name, UserMessage: memPolicy.UserMessage},
	}
	bothOut := s.createUntilPolicyDenied(t, local, "pm-"+token, time.Now().Add(policyPropagationSLO),
		func(out string) bool { return checkPolicyDenial(out, both) == nil }, "--cpu", "1500m", "--memory", "8Gi")
	s.Require().NoErrorf(checkPolicyDenial(bothOut, both), "PLAT-PGP-14.01: a denial by two policies lists both:\n%s", bothOut)

	// E2E-CORE-50.02 (masking): a gateway user reads the policies and findings of
	// its gateway, without targets, and gets 403 on every policy write.
	s.assertGatewayUserPolicyAccess(t, gw.ID, cpuPolicy.Name, cpuPolicyID, started, func(id string) { createdPolicies = append(createdPolicies, id) })

	// E2E-CORE-50.02 (failure modes, PLAT-PGP-11): with the sidecar stopped,
	// create fails closed and delete fails open.
	s.assertSidecarFailureModes(t, running.Namespace, local, token)
}

// grantPolicyAdmin creates a runtime user and grants it platform-policy:admin
// through a global RoleBinding created by the platform admin (the role is
// API-assignable and granted only by platform:admin). The returned cleanup
// deletes the binding and the user.
func (s *E2ESuite) grantPolicyAdmin(t *testing.T) (driver.Credentials, func()) {
	ctx := t.Context()
	creds := driver.Credentials{Username: "e2e-pgp-admin-" + s.runID, Password: "E2e-pgp-admin-" + s.runID + "!"}
	s.Require().NoError(s.driver.CreateTestUser(ctx, creds.Username, creds.Password), "create policy-admin test user")
	var bindingID string
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if bindingID != "" {
			if api, err := s.cleanupAPIClient(cleanupCtx, s.platformAdminCreds()); err != nil {
				t.Errorf("revoke %s binding: %v", platformPolicyAdminRole, err)
			} else if err := api.RoleBindings().Delete(cleanupCtx, bindingID); err != nil {
				t.Errorf("revoke %s binding %s: %v", platformPolicyAdminRole, bindingID, err)
			}
		}
		if err := s.driver.DeleteTestUser(cleanupCtx, creds.Username); err != nil {
			t.Errorf("delete policy-admin test user: %v", err)
		}
	}
	// The /users record is provisioned on the user's first authenticated
	// request; a 403 still provisions it.
	userAPI := s.apiClientForCreds(t, creds)
	_, _, err := userAPI.RawJSON(ctx, http.MethodGet, "/gateways", nil)
	if err != nil {
		cleanup()
		s.Require().NoError(err, "policy-admin authenticated request triggers user provisioning")
	}
	userID := s.findUserIDByUsername(t, creds.Username)
	roleID := s.findRoleIDByName(t, platformPolicyAdminRole)
	s.Require().NoError(s.driver.AssignRealmRole(ctx, s.platformAdminCreds().Username, "platform:admin"), "grant platform:admin")
	platformAPI := s.apiClientForCreds(t, s.platformAdminCreds())
	binding, err := platformAPI.RoleBindings().Create(ctx, &sdktypes.RoleBinding{Scope: "global", RoleID: roleID, UserID: userID})
	if err != nil {
		cleanup()
		s.Require().NoErrorf(err, "platform admin grants %s", platformPolicyAdminRole)
	}
	bindingID = binding.ID
	return creds, cleanup
}

// cleanupAPIClient builds an API client from a fresh token on a context that
// outlives the test's own.
func (s *E2ESuite) cleanupAPIClient(ctx context.Context, creds driver.Credentials) (*apiclient.Client, error) {
	tok, err := s.driver.AcquireOIDCToken(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("acquire token for %s: %w", creds.Username, err)
	}
	return s.driver.APIClient(ctx, tok)
}

// waitPolicyCondition waits until the named policy is reported Applied on the
// gateway in mode. After a mode change the Applied condition may still describe
// the previous bundle, so callers prove propagation with a real create.
func (s *E2ESuite) waitPolicyCondition(t *testing.T, api *apiclient.Client, gatewayID, name, mode string, timeout time.Duration) effectivePolicy {
	var last effectivePolicy
	err := harness.Poll(t.Context(), 2*time.Second, timeout, func(ctx context.Context) (bool, error) {
		ep, status, _, err := s.getEffectivePolicies(ctx, api, gatewayID)
		if err != nil || status != http.StatusOK {
			return false, nil
		}
		p, ok := ep.policy(name)
		if !ok {
			return false, nil
		}
		last = p
		return p.EffectiveMode == mode && p.Condition == policyConditionApplied, nil
	})
	s.Require().NoErrorf(err, "policy %s never reached %s/%s on gateway %s within %s (last: mode %q condition %q revision %q)",
		name, mode, policyConditionApplied, gatewayID, timeout, last.EffectiveMode, last.Condition, last.Revision)
	return last
}

// createUntilPolicyDenied attempts a violating create until it is rejected by a
// platform policy with a denial accept approves, or deadline passes, deleting
// any sandbox an attempt admitted. It returns the accepted denial output.
func (s *E2ESuite) createUntilPolicyDenied(t *testing.T, local, prefix string, deadline time.Time, accept func(out string) bool, flags ...string) string {
	for attempt := 1; ; attempt++ {
		name := fmt.Sprintf("%s-%d", prefix, attempt)
		s.Require().Truef(fitsGatewaySandboxName(name), "sandbox name %q exceeds %d characters", name, gatewaySandboxNameMax)
		args := append([]string{"sandbox", "create", "--from", s.sandboxImage, "--name", name}, flags...)
		out, err := s.cli(t, local, args...)
		if err != nil && isPolicyRejection(out) && accept(out) {
			return out
		}
		s.Logf("violating create %s was not rejected yet (err %v); retrying until %s", name, err, deadline.Format(time.RFC3339))
		s.deleteSandboxIfPresent(t, local, name)
		if time.Now().After(deadline) {
			s.Require().Failf("policy not enforced", "violating create was never rejected as expected by a platform policy before %s; last output:\n%s", deadline.Format(time.RFC3339), out)
		}
		time.Sleep(3 * time.Second)
	}
}

// gatewayPodFingerprint returns the restart fingerprint of the gateway
// Deployment's pods.
func (s *E2ESuite) gatewayPodFingerprint(t *testing.T, ns string) string {
	pods := s.gatewayPods(t, ns)
	s.Require().NotEmpty(pods, "gateway pods in %s", ns)
	return podRestartFingerprint(pods)
}

func (s *E2ESuite) gatewayPods(t *testing.T, ns string) []corev1.Pod {
	dep, err := s.clients.Kube.AppsV1().Deployments(ns).Get(t.Context(), "openshell-gateway", metav1.GetOptions{})
	s.Require().NoError(err, "get gateway deployment in %s", ns)
	s.Require().NotNil(dep.Spec.Selector, "gateway deployment selector")
	selector := labels.SelectorFromSet(dep.Spec.Selector.MatchLabels).String()
	list, err := s.clients.Kube.CoreV1().Pods(ns).List(t.Context(), metav1.ListOptions{LabelSelector: selector})
	s.Require().NoError(err, "list gateway pods in %s", ns)
	return list.Items
}

// assertGatewayUserPolicyAccess grants the developer gateway-user access and
// checks it reads the gateway's policies and findings (without targets) and is
// refused every policy write (SEC-RBAC-30, SEC-RBAC-31).
//
// Each write is a no-op or test-owned if it were wrongly accepted (the policy
// PATCH keeps its mode, the settings PATCH keeps the global mode, and a wrongly
// created policy is passed to track for cleanup), so a regression fails the
// check without changing shared enforcement state.
func (s *E2ESuite) assertGatewayUserPolicyAccess(t *testing.T, gatewayID, policyName, policyID string, since time.Time, track func(id string)) {
	ctx := t.Context()
	devUsername := s.devCreds().Username
	grant, err := s.admin.GatewayAccesses().Create(ctx, gatewayID, &sdktypes.GatewayAccessGrantRequest{
		Username: devUsername,
		Role:     sdktypes.GatewayAccessRoleUser,
	})
	s.Require().NoError(err, "owner grants the developer gateway-user access")
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		api, err := s.cleanupAPIClient(cleanupCtx, s.adminCreds())
		if err == nil {
			_, err = api.GatewayAccesses().Delete(cleanupCtx, gatewayID, grant.UserID)
		}
		if err != nil {
			t.Errorf("revoke developer gateway access: %v", err)
		}
	}()

	devAPI := s.apiClientForCreds(t, s.devCreds())
	var ep effectivePolicies
	readErr := harness.Poll(ctx, 2*time.Second, durationSecondsEnv("E2E_ROLE_GRANT_TIMEOUT", 60*time.Second), func(ctx context.Context) (bool, error) {
		got, status, _, err := s.getEffectivePolicies(ctx, devAPI, gatewayID)
		if err != nil || status != http.StatusOK {
			return false, nil
		}
		ep = got
		return true, nil
	})
	s.Require().NoError(readErr, "E2E-CORE-50.02: the gateway user reads effective_policies")
	p, ok := ep.policy(policyName)
	s.Require().Truef(ok, "E2E-CORE-50.02: the gateway user sees policy %s", policyName)
	s.Assert().Emptyf(strings.TrimSpace(string(p.Target)), "SEC-RBAC-31: the gateway user must not see policy targets (got %s)", string(p.Target))
	s.Assert().Truef(hasFinding(ep.Findings, policyName, findingOutcomeWouldReject, since), "E2E-CORE-50.02: the gateway user sees the %s finding", findingOutcomeWouldReject)

	writes := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/platform_gateway_policies", policyWrite{
			Name: "e2e-pgp-user-" + sandboxToken(), Cedar: `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource) when { context.gpuCount > 8 };`,
			Target: &policyTarget{GatewayIDs: []string{gatewayID}},
		}},
		{http.MethodPatch, "/platform_gateway_policies/" + policyID, map[string]string{"enforcement_mode": p.EnforcementMode}},
		{http.MethodPatch, "/platform_policy_settings", map[string]string{"mode": ep.GlobalMode}},
		{http.MethodDelete, "/platform_gateway_policies/" + policyID, nil},
	}
	for _, w := range writes {
		status, body, err := devAPI.RawExtJSON(ctx, w.method, w.path, w.body)
		s.Require().NoErrorf(err, "gateway user %s %s", w.method, w.path)
		var created struct {
			ID string `json:"id"`
		}
		if w.method == http.MethodPost && status == http.StatusCreated && json.Unmarshal(body, &created) == nil && created.ID != "" {
			track(created.ID)
		}
		s.Assert().Equalf(http.StatusForbidden, status, "E2E-CORE-50.02: the gateway user must get 403 on %s %s (body: %s)", w.method, w.path, string(body))
	}
}

// kindContainerEngine is the engine that runs the Kind node containers, chosen
// the way the Makefile and the CLI wrapper choose it.
func kindContainerEngine() string {
	if engine := os.Getenv("CONTAINER_ENGINE"); engine != "" {
		return engine
	}
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman"
	}
	return "docker"
}

// assertSidecarFailureModes stops the policy sidecar and checks the gateway's
// failure policies (PLAT-PGP-11): CreateSandbox fails closed, StopSandbox and
// DeleteSandbox fail open. The sidecar is a native sidecar container, so it
// cannot be scaled; deleting it would restart the pod, and a pod without it
// cannot start (the gateway calls Describe at startup). The least disruptive
// stop is to freeze the sidecar's containerd task on the Kind node
// (`ctr task pause`), which makes every interceptor call time out exactly as an
// outage would, and to thaw it afterwards. The check runs only on Kind, where
// the suite can reach the node container; elsewhere it reports skipped.
func (s *E2ESuite) assertSidecarFailureModes(t *testing.T, ns, local, token string) {
	s.Run("sidecar-failure-modes", func() {
		t := s.T()
		if s.driver.Name() != "kind" {
			t.Skip("freezing the policy sidecar needs the Kind node's containerd; no safe stop mechanism exists elsewhere")
		}
		var pod *corev1.Pod
		var containerID string
		for _, p := range s.gatewayPods(t, ns) {
			if p.Status.Phase != corev1.PodRunning || p.DeletionTimestamp != nil {
				continue
			}
			for _, cs := range p.Status.InitContainerStatuses {
				if cs.Name != policyInterceptorContainer || cs.State.Running == nil {
					continue
				}
				if id, ok := containerdID(cs.ContainerID); ok {
					pod, containerID = &p, id
				}
			}
		}
		if pod == nil {
			t.Skipf("no running %s sidecar with a containerd id in %s", policyInterceptorContainer, ns)
		}

		keep := "pk-" + token
		out, err := s.cli(t, local, "sandbox", "create", "--from", s.sandboxImage, "--name", keep)
		s.Require().Truef(s.waitSandboxListed(t, local, keep, 60*time.Second), "sandbox %s for the fail-open delete exists (err %v):\n%s", keep, err, out)

		engine := kindContainerEngine()
		ctx := t.Context()
		if out, err := s.runCmdEnv(ctx, os.Environ(), engine, "exec", pod.Spec.NodeName, "ctr", "-n", "k8s.io", "task", "pause", containerID); err != nil {
			s.deleteSandboxIfPresent(t, local, keep)
			t.Skipf("cannot freeze the policy sidecar through %s on node %s (%v): %s", engine, pod.Spec.NodeName, err, out)
		}
		resumed := false
		resume := func() {
			if resumed {
				return
			}
			resumed = true
			resumeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if out, err := s.runCmdEnv(resumeCtx, os.Environ(), engine, "exec", pod.Spec.NodeName, "ctr", "-n", "k8s.io", "task", "resume", containerID); err != nil {
				// The kubelet may already have restarted the frozen sidecar after
				// its liveness probe failed; the gateway is deleted at teardown.
				s.Logf("resume policy sidecar (best effort): %v\n%s", err, out)
			}
		}
		defer resume()

		closedName := "pc-" + token
		out, err = s.cli(t, local, "sandbox", "create", "--from", s.sandboxImage, "--name", closedName)
		s.Assert().Errorf(err, "E2E-CORE-50.02: CreateSandbox must fail closed with the sidecar stopped:\n%s", out)
		s.Assert().Truef(isPermissionDenied(out) && isInterceptorFailedClosed(out), "E2E-CORE-50.02: create reports the interceptor failed closed:\n%s", out)

		stopOut, stopErr := s.cli(t, local, "sandbox", "stop", keep)
		s.Assert().Falsef(isInterceptorFailedClosed(stopOut), "E2E-CORE-50.02: StopSandbox must fail open (err %v):\n%s", stopErr, stopOut)
		delOut, delErr := s.cli(t, local, "sandbox", "delete", keep)
		s.Assert().NoErrorf(delErr, "E2E-CORE-50.02: DeleteSandbox must fail open with the sidecar stopped:\n%s", delOut)
		s.Assert().Falsef(isInterceptorFailedClosed(delOut), "E2E-CORE-50.02: DeleteSandbox must not fail closed:\n%s", delOut)

		resume()
		s.deleteSandboxIfPresent(t, local, closedName)
		s.waitSandboxesFullyDeleted(t, local, []string{keep}, durationSecondsEnv("E2E_SANDBOX_DELETE_TIMEOUT", 120*time.Second))
	})
}
