package policyinterceptor

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"github.com/openshift-online/hypershell/components/control-plane/internal/policybundle"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/openshift-online/hypershell/components/api-server/pkg/cedar"
	extv1 "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/extensionv1"
	gi "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/gatewayinterceptorv1"
)

func gatewayMeta(major uint32, required ...string) *gi.DescribeRequest {
	return &gi.DescribeRequest{Gateway: &extv1.PeerMetadata{
		ProtocolVersion:       &extv1.ProtocolVersion{Major: major, Minor: 3},
		ImplementationName:    "openshell/gateway",
		SupportedCapabilities: []string{CapabilityContract},
		RequiredCapabilities:  required,
	}}
}

// Verifies: PLAT-PGP-09.02
func TestGatewayJWTVerifiedOverUnixSocket(t *testing.T) {
	kit := newJWTKit(t)
	client := startGRPC(t, newTestServer(t), kit)
	ctx := context.Background()

	// A valid token: Describe works before any bundle loads.
	m, err := client.Describe(withToken(ctx, kit.token(t, nil)), gatewayMeta(1))
	if err != nil {
		t.Fatalf("Describe with valid token: %v", err)
	}
	if len(m.GetBindings()) != 30 {
		t.Fatalf("got %d bindings", len(m.GetBindings()))
	}

	_, otherKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	bad := map[string]string{
		"missing":      "",
		"wrong typ":    kit.token(t, func(o *tokenOpts) { o.typ = "JWT" }),
		"wrong issuer": kit.token(t, func(o *tokenOpts) { o.iss = "openshell-gateway:other" }),
		"wrong aud":    kit.token(t, func(o *tokenOpts) { o.aud = "urn:openshell:extension:interceptor:other" }),
		"expired":      kit.token(t, func(o *tokenOpts) { o.exp = time.Now().Add(-time.Hour) }),
		"supervisor":   kit.token(t, func(o *tokenOpts) { o.callerKind = "supervisor" }),
		"wrong key":    kit.token(t, func(o *tokenOpts) { o.key = otherKey }),
		"not a jwt":    "abc.def.ghi",
		"alg none":     "eyJhbGciOiJub25lIiwidHlwIjoib3BlbnNoZWxsLWV4dCtqd3QifQ.eyJpc3MiOiJvcGVuc2hlbGwtZ2F0ZXdheTpndy10ZXN0LTEifQ.",
		"empty issuer": kit.token(t, func(o *tokenOpts) { o.iss = "" }),
	}
	for name, tok := range bad {
		callCtx := ctx
		if tok != "" {
			callCtx = withToken(ctx, tok)
		}
		if _, err := client.Describe(callCtx, gatewayMeta(1)); status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s: Describe got %v, want Unauthenticated", name, err)
		}
		if _, err := client.Evaluate(callCtx, validateEval(t, "CreateSandbox", createOp("x", nil, nil))); status.Code(err) != codes.Unauthenticated {
			t.Errorf("%s: Evaluate got %v, want Unauthenticated", name, err)
		}
	}
}

// Verifies: OSG-CORE-19.01
func TestDescribeNegotiatesProtocol(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()
	if _, err := srv.Describe(ctx, gatewayMeta(1)); err != nil {
		t.Fatalf("compatible gateway rejected: %v", err)
	}
	if _, err := srv.Describe(ctx, gatewayMeta(2)); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("major 2: got %v, want FailedPrecondition", err)
	}
	if _, err := srv.Describe(ctx, gatewayMeta(1, "openshell.gateway-interceptor.future")); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unmet capability: got %v, want FailedPrecondition", err)
	}
	if _, err := srv.Describe(ctx, &gi.DescribeRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("missing metadata: got %v, want FailedPrecondition", err)
	}
	if _, err := srv.Describe(ctx, gatewayMeta(1, CapabilityContract)); err != nil {
		t.Fatalf("supported required capability rejected: %v", err)
	}
}

// Verifies: PLAT-PGP-11.01, PLAT-PGP-12.02
func TestNoBundleFailsClosedOnCreateAndOpenOnDelete(t *testing.T) {
	kit := newJWTKit(t)
	client := startGRPC(t, newTestServer(t), kit)
	ctx := withToken(context.Background(), kit.token(t, nil))

	_, err := client.Evaluate(ctx, validateEval(t, "CreateSandbox", createOp("x", nil, nil)))
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("CreateSandbox without bundle: got %v, want an Unavailable error (gateway fails closed)", err)
	}
	if _, err := client.Evaluate(ctx, modifyEval(t, createOp("x", nil, nil))); err == nil {
		t.Fatalf("CreateSandbox modify_operation without bundle: want error")
	}
	for _, m := range []string{"StartSandbox", "ResumeSandbox", "AttachSandboxProvider"} {
		if _, err := client.Evaluate(ctx, validateEval(t, m, map[string]any{"name": "x"})); err == nil {
			t.Errorf("%s without bundle: want error (fail_closed)", m)
		}
	}
	res, err := client.Evaluate(ctx, validateEval(t, "DeleteSandbox", map[string]any{"name": "x"}))
	if err != nil || !res.GetAllowed() {
		t.Fatalf("DeleteSandbox without bundle: got %+v, %v; want allow", res, err)
	}
}

// Verifies: PLAT-PGP-08.01, PLAT-PGP-08.06
func TestModifyOperationDefaultsOnlyUnderEnforcement(t *testing.T) {
	op := createOp("s1", map[string]any{"memory": "1Gi"}, map[string]any{"memory": "1Gi"})

	enforced := newTestServer(t, buildBundle(t, testGatewayID, false,
		testPolicy{name: "cpu-cap", mode: "enforce", cedar: cpuOver("4000")}))
	res, err := enforced.Evaluate(context.Background(), modifyEval(t, op))
	if err != nil || !res.GetAllowed() {
		t.Fatalf("modify: %+v, %v", res, err)
	}
	got := map[string]string{}
	for _, p := range res.GetPatches() {
		got[p.GetPath()] = p.GetValue().GetStringValue()
	}
	annotation := "/annotations/" + jsonPointerEscape(DefaultsAppliedAnnotation)
	if got["/spec/template/resources/requests/cpu"] != "500m" || got["/spec/template/resources/limits/cpu"] != "2" ||
		got[annotation] != "limits.cpu=2,requests.cpu=500m" || len(got) != 4 {
		t.Fatalf("got patches %v, want requests/cpu=500m, limits/cpu=2, the /annotations object, and %s=limits.cpu=2,requests.cpu=500m only", got, DefaultsAppliedAnnotation)
	}
	if enforced.Events.Len() != 0 {
		t.Fatalf("enforced gateway recorded %d events, want 0", enforced.Events.Len())
	}

	audit := newTestServer(t, buildBundle(t, testGatewayID, false,
		testPolicy{name: "cpu-cap", mode: "audit", cedar: cpuOver("4000")}))
	res, err = audit.Evaluate(context.Background(), modifyEval(t, op))
	if err != nil || !res.GetAllowed() || len(res.GetPatches()) != 0 {
		t.Fatalf("audit-only modify: got %+v, %v; want allow with no patch", res, err)
	}
	events := audit.Events.peek(10)
	if len(events) != 1 || events[0].Outcome != OutcomeWouldAddDefaults || events[0].Operation != "CreateSandbox" ||
		events[0].PrincipalSubject != "alice@example.com" || events[0].ID == "" {
		t.Fatalf("got events %+v, want one would_add_defaults", events)
	}
}

// Verifies: PLAT-PGP-07.01
func TestAuditOnlyPolicyAllowsButRecords(t *testing.T) {
	srv := newTestServer(t, buildBundle(t, testGatewayID, false,
		testPolicy{name: "cpu-audit", mode: "audit", message: "Too much CPU.", cedar: cpuOver("1000")}))
	res, err := srv.Evaluate(context.Background(), validateEval(t, "CreateSandbox",
		createOp("s1", map[string]any{"cpu": "2"}, map[string]any{"cpu": "2"})))
	if err != nil || !res.GetAllowed() || res.GetReason() != "" || len(res.GetPatches()) != 0 {
		t.Fatalf("got %+v, %v; want unchanged allow", res, err)
	}
	events := srv.Events.peek(10)
	if len(events) != 1 || events[0].Outcome != OutcomeWouldReject || len(events[0].PolicyNames) != 1 ||
		events[0].PolicyNames[0] != "cpu-audit" || events[0].RequestSummary[cedar.CtxCPURequestMillis] != 2000 {
		t.Fatalf("got events %+v, want one would_reject naming cpu-audit", events)
	}
	if events[0].BundleRevision != srv.Store.AppliedRevision() {
		t.Fatalf("event revision %q, want %q", events[0].BundleRevision, srv.Store.AppliedRevision())
	}
}

// Verifies: PLAT-PGP-13.01
func TestEnforcedForbidDeniesCreate(t *testing.T) {
	srv := newTestServer(t, buildBundle(t, testGatewayID, false,
		testPolicy{name: "cpu-cap", mode: "enforce", message: "At most one CPU.", cedar: cpuOver("1000")},
		testPolicy{name: "mem-audit", mode: "audit", cedar: memLimitOver("4294967296")}))
	res, err := srv.Evaluate(context.Background(), validateEval(t, "CreateSandbox",
		createOp("s1", map[string]any{"cpu": "2"}, map[string]any{"cpu": "2", "memory": "8Gi"})))
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.GetAllowed() || res.GetStatusCode() != "PERMISSION_DENIED" || !strings.Contains(res.GetReason(), "cpu-cap") {
		t.Fatalf("got %+v, want PERMISSION_DENIED naming cpu-cap", res)
	}
	events := srv.Events.peek(10)
	if len(events) != 1 || events[0].PolicyNames[0] != "mem-audit" || events[0].Outcome != OutcomeWouldReject {
		t.Fatalf("audit-only set not evaluated after the denial: %+v", events)
	}
	// A compliant request is allowed.
	res, err = srv.Evaluate(context.Background(), validateEval(t, "CreateSandbox",
		createOp("s2", map[string]any{"cpu": "500m"}, map[string]any{"cpu": "1", "memory": "1Gi"})))
	if err != nil || !res.GetAllowed() {
		t.Fatalf("compliant request: got %+v, %v", res, err)
	}
}

// Verifies: PLAT-PGP-14.01
func TestDenialNamesEveryMatchingPolicy(t *testing.T) {
	srv := newTestServer(t, buildBundle(t, testGatewayID, false,
		testPolicy{name: "cpu-cap", mode: "enforce", message: "At most one CPU.", doc: "https://docs.example/cpu", cedar: cpuOver("1000")},
		testPolicy{name: "mem-cap", mode: "enforce", message: "At most 4Gi memory.", doc: "https://docs.example/mem", cedar: memLimitOver("4294967296")}))
	res, err := srv.Evaluate(context.Background(), validateEval(t, "CreateSandbox",
		createOp("s1", map[string]any{"cpu": "2"}, map[string]any{"cpu": "2", "memory": "8Gi"})))
	if err != nil || res.GetAllowed() {
		t.Fatalf("got %+v, %v; want deny", res, err)
	}
	reason := res.GetReason()
	for _, want := range []string{"cpu-cap", "At most one CPU.", "mem-cap", "At most 4Gi memory.", srv.PoliciesURL} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q does not contain %q", reason, want)
		}
	}
	// The renderer recognizes the comparison: the line states value and bound.
	if !strings.Contains(reason, "allowed:") {
		t.Errorf("reason %q does not state the value and the bound", reason)
	}
}

// Verifies: PLAT-PGP-14.02
func TestDenialCarriesNoSecretRequestFields(t *testing.T) {
	srv := newTestServer(t, buildBundle(t, testGatewayID, false,
		testPolicy{name: "cpu-cap", mode: "enforce", message: "At most one CPU.", cedar: cpuOver("1000")}))
	op := createOp("s1", map[string]any{"cpu": "2"}, nil)
	spec := op["spec"].(map[string]any)
	spec["environment"] = map[string]any{"API_TOKEN": "supersecret-value"}
	op["annotations"] = map[string]any{"note": "internal-host.corp"}
	spec["template"].(map[string]any)["environment"] = map[string]any{"PASSWORD": "hunter2"}
	res, err := srv.Evaluate(context.Background(), validateEval(t, "CreateSandbox", op))
	if err != nil || res.GetAllowed() {
		t.Fatalf("got %+v, %v; want deny", res, err)
	}
	for _, leaked := range []string{"supersecret-value", "API_TOKEN", "hunter2", "PASSWORD", "internal-host.corp"} {
		if strings.Contains(res.GetReason(), leaked) {
			t.Errorf("reason leaks %q: %q", leaked, res.GetReason())
		}
		for k, v := range res.GetLogAnnotations() {
			if strings.Contains(k+v, leaked) {
				t.Errorf("log annotation leaks %q", leaked)
			}
		}
	}
}

// Verifies: PLAT-PGP-18.01
func TestUnsupportedDriverNeverEnforces(t *testing.T) {
	srv := newTestServer(t, buildBundle(t, testGatewayID, true,
		testPolicy{name: "cpu-cap", mode: "enforce", cedar: cpuOver("1000")}))
	op := createOp("s1", map[string]any{"cpu": "2"}, nil)
	res, err := srv.Evaluate(context.Background(), validateEval(t, "CreateSandbox", op))
	if err != nil || !res.GetAllowed() {
		t.Fatalf("validate on unsupported gateway: got %+v, %v; want allow", res, err)
	}
	res, err = srv.Evaluate(context.Background(), modifyEval(t, op))
	if err != nil || !res.GetAllowed() || len(res.GetPatches()) != 0 {
		t.Fatalf("modify on unsupported gateway: got %+v, %v; want allow without patches", res, err)
	}
}

type fakeLookup struct {
	ctx  cedar.Context
	err  error
	seen []string
}

func (f *fakeLookup) Lookup(_ context.Context, workspace, name string) (cedar.Context, error) {
	f.seen = append(f.seen, workspace+"/"+name)
	return f.ctx, f.err
}

// Verifies: PLAT-PGP-16.01
func TestResumeRecheckAgainstEnforcedSet(t *testing.T) {
	bundle := buildBundle(t, testGatewayID, false,
		testPolicy{name: "cpu-cap", mode: "enforce", message: "At most one CPU.", cedar: cpuOver("1000")})
	srv := newTestServer(t, bundle)
	resume := map[string]any{"name": "s1", "workspaceScope": map[string]any{"workspace": "team-a"}}

	// When the stored resources can be read, a non-compliant sandbox is denied.
	lookup := &fakeLookup{ctx: cedar.Context{CPURequestMillis: 2000, CPULimitMillis: 2000, MemoryRequestBytes: 1 << 30, MemoryLimitBytes: 1 << 30, SandboxName: "s1"}}
	srv.Lookup = lookup
	for _, m := range []string{"ResumeSandbox", "StartSandbox"} {
		res, err := srv.Evaluate(context.Background(), validateEval(t, m, resume))
		if err != nil || res.GetAllowed() || res.GetStatusCode() != "PERMISSION_DENIED" {
			t.Fatalf("%s: got %+v, %v; want PERMISSION_DENIED", m, res, err)
		}
	}
	if len(lookup.seen) != 2 || lookup.seen[0] != "team-a/s1" {
		t.Fatalf("lookup calls %v", lookup.seen)
	}

	// The production lookup cannot read stored resources (current_state is
	// always empty, AGT-EXT-06): the re-check is skipped and the call allowed.
	srv.Lookup = NoSandboxLookup{}
	res, err := srv.Evaluate(context.Background(), validateEval(t, "ResumeSandbox", resume))
	if err != nil || !res.GetAllowed() {
		t.Fatalf("resume with unsupported lookup: got %+v, %v; want allow", res, err)
	}
	srv.Lookup = &fakeLookup{err: errors.New("boom")}
	if res, err := srv.Evaluate(context.Background(), validateEval(t, "StartSandbox", resume)); err != nil || !res.GetAllowed() {
		t.Fatalf("start with failing lookup: got %+v, %v; want allow", res, err)
	}
}

// TestBuiltInCanaryInjectsDefaultsOnlyForItsPrefix checks that an enforced
// built-in policy (the e2e canary) adds fleet defaults only to sandboxes with
// the reserved prefix, records a would_add_defaults finding for others, and
// that an enforced user policy still turns injection on for every sandbox.
// Verifies: PLAT-PGP-08.07, PLAT-PGP-20.01
func TestBuiltInCanaryInjectsDefaultsOnlyForItsPrefix(t *testing.T) {
	canary := testPolicy{
		name: policybundle.CanaryPolicyName, mode: "enforce", builtIn: true,
		cedar: `forbid (principal, action == OpenShell::Action::"CreateSandbox", resource) when { context.sandboxName like "e2epol-*" && context.cpuRequestMillis > 1000 };`,
	}
	srv := newTestServer(t, buildBundle(t, testGatewayID, false, canary))

	regular, err := srv.Evaluate(context.Background(), modifyEval(t, createOp("sb-1", nil, nil)))
	if err != nil || !regular.GetAllowed() {
		t.Fatalf("regular create: %+v, %v", regular, err)
	}
	if len(regular.GetPatches()) != 0 {
		t.Fatalf("canary-only gateway patched a regular sandbox: %v", regular.GetPatches())
	}
	if srv.Events.Len() != 1 {
		t.Fatalf("regular sandbox on a canary-only gateway recorded %d events, want 1 would_add_defaults", srv.Events.Len())
	}

	prefixed, err := srv.Evaluate(context.Background(), modifyEval(t, createOp(policybundle.CanarySandboxPrefix+"123456-allow", nil, nil)))
	if err != nil || !prefixed.GetAllowed() {
		t.Fatalf("prefixed create: %+v, %v", prefixed, err)
	}
	if len(prefixed.GetPatches()) == 0 {
		t.Fatal("canary did not add defaults to a reserved-prefix sandbox")
	}

	user := newTestServer(t, buildBundle(t, testGatewayID, false, canary,
		testPolicy{name: "cpu-cap", mode: "enforce", cedar: cpuOver("4000")}))
	withUser, err := user.Evaluate(context.Background(), modifyEval(t, createOp("sb-2", nil, nil)))
	if err != nil || len(withUser.GetPatches()) == 0 {
		t.Fatalf("an enforced user policy must add defaults to every sandbox: %+v, %v", withUser, err)
	}
}
