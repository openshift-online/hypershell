package policyinterceptor

import (
	"bytes"
	"context"
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
