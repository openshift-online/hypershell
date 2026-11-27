package helm

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// imageRefPattern matches every image reference the chart renders: workload
// "image:" fields and the image settings in the generated gateway config.
var imageRefPattern = regexp.MustCompile(`(?m)^\s*(?:-\s*)?(?:image|[a-z_]*_image)\s*[:=]\s*"?([^"\s]+)"?`)

func renderedImageRefs(t *testing.T, helmBin, chart, valuesFile string) []string {
	t.Helper()
	out, err := exec.Command(helmBin, "template", "t", chart, "-f", valuesFile,
		"--set", "agentSandbox.preflight.enabled=false").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template %s: %v\n%s", chart, err, out)
	}
	var refs []string
	for _, m := range imageRefPattern.FindAllStringSubmatch(string(out), -1) {
		refs = append(refs, m[1])
	}
	sort.Strings(refs)
	return refs
}

func packageChart(t *testing.T, helmBin, src, dest string, extra ...string) string {
	t.Helper()
	args := append([]string{"package", src, "--destination", dest}, extra...)
	if out, err := exec.Command(helmBin, args...).CombinedOutput(); err != nil {
		t.Fatalf("helm package %v: %v\n%s", extra, err, out)
	}
	matches, err := filepath.Glob(filepath.Join(dest, "*.tgz"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("packaged chart in %s: %v %v", dest, matches, err)
	}
	return matches[0]
}

// TestChartVersionStampDoesNotChangeRenderedImages packages the vendored chart
// with its 0.0.0 placeholders and with the version the control-plane image
// build stamps from OPENSHELL_TAG, and proves every rendered image reference is
// identical. The chart falls back to appVersion only for an unset image tag;
// the control plane always sets the gateway and supervisor images, and an
// unset sandbox runtime image is not rendered at all.
// Verifies: OSG-HELM-07.02, OSG-HELM-07.01
func TestChartVersionStampDoesNotChangeRenderedImages(t *testing.T) {
	helmBin, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm binary not found on PATH")
	}
	src, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "charts", "openshell"))
	if err != nil {
		t.Fatal(err)
	}
	const tag = "v0.1.10-rhaiv.0"
	placeholder := packageChart(t, helmBin, src, t.TempDir())
	stamped := packageChart(t, helmBin, src, t.TempDir(),
		"--version", strings.TrimPrefix(tag, "v")+"+hs", "--app-version", tag)

	out, err := exec.Command(helmBin, "show", "chart", stamped).CombinedOutput()
	if err != nil {
		t.Fatalf("helm show chart: %v\n%s", err, out)
	}
	for _, want := range []string{"version: 0.1.10-rhaiv.0+hs", "appVersion: " + tag} {
		if !strings.Contains(string(out), want) {
			t.Errorf("stamped Chart.yaml lacks %q:\n%s", want, out)
		}
	}

	for name, sandboxRuntime := range map[string]string{
		"sandbox runtime pinned": "quay.io/test/sandbox:v1",
		"sandbox runtime unset":  "",
	} {
		t.Run(name, func(t *testing.T) {
			values, err := (&ValuesBuilder{
				Gateway: GatewayConfig{
					WorkspaceMode:       "shared",
					Image:               "quay.io/test/gateway:v1",
					SupervisorImage:     "quay.io/test/supervisor:v1",
					SandboxRuntimeImage: sandboxRuntime,
				},
				Namespace: "test-ns",
			}).Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			raw, err := json.Marshal(values)
			if err != nil {
				t.Fatal(err)
			}
			valuesFile := filepath.Join(t.TempDir(), "values.json")
			if err := os.WriteFile(valuesFile, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			before := renderedImageRefs(t, helmBin, placeholder, valuesFile)
			t.Logf("rendered images: %v", before)
			after := renderedImageRefs(t, helmBin, stamped, valuesFile)
			if len(before) == 0 {
				t.Fatal("no image references rendered; the pattern is stale")
			}
			if strings.Join(before, "\n") != strings.Join(after, "\n") {
				t.Errorf("stamping the chart version changed rendered images:\nplaceholder: %v\nstamped:     %v", before, after)
			}
			for _, ref := range after {
				if strings.Contains(ref, tag) || strings.HasSuffix(ref, ":0.0.0") {
					t.Errorf("image %q fell back to the chart appVersion", ref)
				}
			}
		})
	}
}
