package e2e

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift-online/hypershell/tests/e2e/driver"
	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

// TestFailureDiagnostics is the workflow fallback for failures that happen
// after TestE2E succeeds (for example the live tracing assertion). The normal
// suite path collects automatically from TearDownSuite; this opt-in entry point
// keeps the fallback implementation in the tests instead of in Actions YAML.
func TestFailureDiagnostics(t *testing.T) {
	if os.Getenv("E2E_COLLECT_DIAGNOSTICS") != "1" {
		t.Skip("set E2E_COLLECT_DIAGNOSTICS=1 to collect diagnostics without running the suite")
	}
	clients, err := harness.NewClients()
	if err != nil {
		t.Logf("DIAG build Kubernetes clients: %v", err)
		return
	}
	d, err := driver.Resolve(t.Context(), clients, os.Getenv("E2E_INFRA_DRIVER"))
	if err != nil {
		t.Logf("DIAG resolve infrastructure driver: %v", err)
	}
	s := newE2ESuite(d, clients)
	s.runID = "workflow-failure"
	s.logger = harness.NewLogger(t.Logf)
	s.SetT(t)
	s.collectFailureDiagnostics()
}

func TestDiagnosticSectionRedactsArtifacts(t *testing.T) {
	s := &E2ESuite{logger: harness.NewLogger(t.Logf)}
	s.SetT(t)
	dir := filepath.Join(t.TempDir(), "run")
	d := &failureDiagnostics{suite: s, dir: dir}

	d.section(context.Background(), "secret-bearing section", "section.txt", func(_ context.Context, w io.Writer) {
		_, _ = w.Write([]byte("Authorization: Bearer secret-token\n-----BEGIN PRIVATE KEY-----\nprivate-data\n-----END PRIVATE KEY-----\n"))
	})

	data, err := os.ReadFile(filepath.Join(dir, "section.txt"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, secret := range []string{"secret-token", "private-data"} {
		if strings.Contains(text, secret) {
			t.Errorf("diagnostic artifact leaks %q: %s", secret, text)
		}
	}
	if !strings.Contains(text, "[REDACTED PRIVATE KEY]") {
		t.Fatalf("diagnostic artifact lost redaction marker: %s", text)
	}
	info, err := os.Stat(filepath.Join(dir, "section.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("diagnostic artifact mode = %o, want 600", got)
	}
}

func TestWriteAllPodsIncludesReadinessAndFailureState(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "hypershell-system"},
		Spec:       corev1.PodSpec{NodeName: "worker-a", Containers: []corev1.Container{{Name: "api"}}},
		Status: corev1.PodStatus{
			Phase:             corev1.PodFailed,
			Reason:            "CrashLoop",
			Message:           "container exited",
			ContainerStatuses: []corev1.ContainerStatus{{Name: "api", Ready: false, RestartCount: 3}},
		},
	}
	s := &E2ESuite{clients: &harness.Clients{Kube: fake.NewSimpleClientset(pod)}}
	d := &failureDiagnostics{suite: s}
	var out strings.Builder
	d.writeAllPods(context.Background(), &out)
	for _, want := range []string{"hypershell-system/api-1", "phase=Failed", "ready=0/1", "restarts=3", `reason="CrashLoop"`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("pod diagnostics %q do not contain %q", out.String(), want)
		}
	}
}

func TestDiagnosticPathComponentIsFilesystemSafe(t *testing.T) {
	if got := diagnosticPathComponent("123/ibm:us-south"); got != "123_ibm_us-south" {
		t.Fatalf("diagnosticPathComponent = %q", got)
	}
	if got := diagnosticPathComponent(""); got != "setup-failure" {
		t.Fatalf("empty diagnosticPathComponent = %q", got)
	}
}
