package policyinterceptor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	gi "github.com/openshift-online/hypershell/components/control-plane/internal/openshellclient/gen/gatewayinterceptorv1"
)

// Verifies: PLAT-PGP-03.01
func TestAdapterNormalizesQuantities(t *testing.T) {
	sr, err := ParseCreateSandbox(createOp("s1", map[string]any{"cpu": "500m"}, map[string]any{"memory": "16Gi"}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c, err := sr.Context(testDefaults(t))
	if err != nil {
		t.Fatalf("context: %v", err)
	}
	if c.CPURequestMillis != 500 || c.MemoryLimitBytes != 17179869184 {
		t.Fatalf("got cpuRequestMillis=%d memoryLimitBytes=%d, want 500 and 17179869184", c.CPURequestMillis, c.MemoryLimitBytes)
	}
	// Defaults fill the rest; memory request takes the limit (effective request).
	if c.CPULimitMillis != 2000 || c.MemoryRequestBytes != 17179869184 {
		t.Fatalf("got cpuLimitMillis=%d memoryRequestBytes=%d", c.CPULimitMillis, c.MemoryRequestBytes)
	}
	if c.Image != "quay.io/example/agent:1" || c.SandboxName != "s1" {
		t.Fatalf("got image=%q name=%q", c.Image, c.SandboxName)
	}
}

// Verifies: PLAT-PGP-03.01
func TestAdapterKubernetesDriverInputs(t *testing.T) {
	op := createOp("s2", nil, map[string]any{"cpu": "3"})
	tmpl := op["spec"].(map[string]any)["template"].(map[string]any)
	tmpl["driverConfig"] = map[string]any{"kubernetes": map[string]any{"containers": map[string]any{"agent": map[string]any{
		"resources": map[string]any{
			"requests": map[string]any{"cpu": "1", "memory": "2Gi"},
			"limits":   map[string]any{"cpu": "8", "memory": "8Gi"},
		},
	}}}}
	op["spec"].(map[string]any)["resourceRequirements"] = map[string]any{"gpu": map[string]any{}}
	sr, err := ParseCreateSandbox(op)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c, err := sr.Context(testDefaults(t))
	if err != nil {
		t.Fatalf("context: %v", err)
	}
	// Template values win over driver_config (the driver merges without
	// overriding); a template limit with no template request is the effective
	// request.
	if c.CPULimitMillis != 3000 || c.CPURequestMillis != 3000 {
		t.Fatalf("cpu: got request=%d limit=%d, want 3000/3000", c.CPURequestMillis, c.CPULimitMillis)
	}
	if c.MemoryRequestBytes != 2<<30 || c.MemoryLimitBytes != 8<<30 {
		t.Fatalf("memory: got request=%d limit=%d", c.MemoryRequestBytes, c.MemoryLimitBytes)
	}
	if c.GPUCount != 1 {
		t.Fatalf("gpu: got %d, want 1 (gpu present without count)", c.GPUCount)
	}
}

// Verifies: PLAT-PGP-11.02
func TestDefaultPatchesAreDeterministic(t *testing.T) {
	d := testDefaults(t)
	op := createOp("s3", nil, nil)
	encode := func() []byte {
		sr, err := ParseCreateSandbox(op)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		patches, err := sr.DefaultPatches(d)
		if err != nil {
			t.Fatalf("patches: %v", err)
		}
		var buf bytes.Buffer
		for _, p := range patches {
			raw, err := proto.Marshal(p)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			buf.Write(raw)
		}
		return buf.Bytes()
	}
	first, second := encode(), encode()
	if len(first) == 0 || !bytes.Equal(first, second) {
		t.Fatalf("patches differ between identical evaluations")
	}

	// Through the server: the same request replayed returns the same patches.
	srv := newTestServer(t, buildBundle(t, testGatewayID, false,
		testPolicy{name: "cpu-cap", mode: "enforce", cedar: cpuOver("4000")}))
	r1, err := srv.Evaluate(context.Background(), modifyEval(t, op))
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	r2, err := srv.Evaluate(context.Background(), modifyEval(t, op))
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !proto.Equal(&gi.InterceptorResult{Patches: r1.GetPatches()}, &gi.InterceptorResult{Patches: r2.GetPatches()}) {
		t.Fatalf("replayed patches differ")
	}
	var paths []string
	for _, p := range r1.GetPatches() {
		paths = append(paths, p.GetOp()+" "+p.GetPath())
	}
	want := []string{
		"add /spec/template/resources",
		"add /spec/template/resources/limits",
		"add /spec/template/resources/limits/cpu",
		"add /spec/template/resources/limits/memory",
		"add /spec/template/resources/requests",
		"add /spec/template/resources/requests/cpu",
		"add /spec/template/resources/requests/memory",
		"add /annotations",
		"add /annotations/hypershell.redhat.io~1policy-defaults-applied",
	}
	if len(paths) != len(want) {
		t.Fatalf("got patches %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("got patches %v, want %v", paths, want)
		}
	}
}

// Verifies: PLAT-PGP-08.01
func TestDefaultPatchesAddOnlyMissingValues(t *testing.T) {
	d := testDefaults(t)
	// CPU request set, memory limit set: add the CPU limit only (memory request
	// is the memory limit).
	sr, err := ParseCreateSandbox(createOp("s4", map[string]any{"cpu": "250m"}, map[string]any{"memory": "2Gi"}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	patches, err := sr.DefaultPatches(d)
	if err != nil {
		t.Fatalf("patches: %v", err)
	}
	patches = resourcePatches(patches)
	if len(patches) != 1 || patches[0].GetPath() != "/spec/template/resources/limits/cpu" || patches[0].GetValue().GetStringValue() != "2" {
		t.Fatalf("got %v, want one add of limits/cpu=2", patches)
	}
	// A request above the default limit gets a limit equal to the request, so a
	// default never makes request > limit.
	sr, err = ParseCreateSandbox(createOp("s5", map[string]any{"cpu": "3", "memory": "1Gi"}, map[string]any{"memory": "1Gi"}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	patches, err = sr.DefaultPatches(d)
	if err != nil {
		t.Fatalf("patches: %v", err)
	}
	patches = resourcePatches(patches)
	if len(patches) != 1 || patches[0].GetValue().GetStringValue() != "3" {
		t.Fatalf("got %v, want limits/cpu=3", patches)
	}
	// Fully specified: no patches.
	sr, err = ParseCreateSandbox(createOp("s6", map[string]any{"cpu": "1", "memory": "1Gi"}, map[string]any{"cpu": "1", "memory": "1Gi"}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if patches, err = sr.DefaultPatches(d); err != nil || len(patches) != 0 {
		t.Fatalf("got %v, %v; want no patches", patches, err)
	}
}

// TestDefaultPatchesRecordAppliedDefaults checks that the defaults the
// interceptor adds are recorded on the sandbox, that an existing annotations
// object is reused, and that a client-supplied value is replaced.
// Verifies: PLAT-PGP-08.06
func TestDefaultPatchesRecordAppliedDefaults(t *testing.T) {
	d := DefaultQuantities{CPURequest: "500m", CPULimit: "2", MemoryRequest: "1Gi", MemoryLimit: "4Gi"}
	path := "/annotations/" + jsonPointerEscape(DefaultsAppliedAnnotation)
	if path != "/annotations/hypershell.redhat.io~1policy-defaults-applied" {
		t.Fatalf("annotation pointer = %q", path)
	}
	for _, tc := range []struct {
		name          string
		op            map[string]any
		wantObjectAdd bool
	}{
		{"no annotations", map[string]any{"name": "s1"}, true},
		{"client annotation replaced", map[string]any{"name": "s1", "annotations": map[string]any{DefaultsAppliedAnnotation: "forged"}}, false},
	} {
		sr, err := ParseCreateSandbox(tc.op)
		if err != nil {
			t.Fatalf("%s: parse: %v", tc.name, err)
		}
		patches, err := sr.DefaultPatches(d)
		if err != nil {
			t.Fatalf("%s: patches: %v", tc.name, err)
		}
		var sawObject bool
		var value string
		for _, p := range patches {
			switch p.GetPath() {
			case "/annotations":
				sawObject = true
			case path:
				if p.GetOp() != "add" {
					t.Fatalf("%s: annotation op = %q, want add (replaces an existing member)", tc.name, p.GetOp())
				}
				value = p.GetValue().GetStringValue()
			}
		}
		if sawObject != tc.wantObjectAdd {
			t.Fatalf("%s: /annotations object add = %v, want %v", tc.name, sawObject, tc.wantObjectAdd)
		}
		want := "requests.cpu=500m,limits.cpu=2,requests.memory=1Gi,limits.memory=4Gi"
		if value == "" || value == "forged" {
			t.Fatalf("%s: annotation value %q", tc.name, value)
		}
		for _, entry := range strings.Split(want, ",") {
			if !strings.Contains(value, entry) {
				t.Fatalf("%s: annotation %q is missing %q", tc.name, value, entry)
			}
		}
	}
}

// resourcePatches drops the applied-defaults annotation patches, leaving only
// the spec.template.resources patches.
func resourcePatches(patches []*gi.JsonPatch) []*gi.JsonPatch {
	var out []*gi.JsonPatch
	for _, p := range patches {
		if strings.HasPrefix(p.GetPath(), "/annotations") {
			continue
		}
		out = append(out, p)
	}
	return out
}
