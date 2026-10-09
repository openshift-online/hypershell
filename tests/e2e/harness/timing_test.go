package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTimingReporterSummaryIsCanonicalAndIncludesTotal(t *testing.T) {
	started := time.Unix(100, 0)
	r := NewTimingReporter(started)
	r.Record("P2.3", "deletion-namespace-gc", 3*time.Second, "PASS")
	r.Record("P0.1", "authentication", time.Second, "PASS")

	summary := r.Summary(started.Add(9 * time.Second))
	if strings.Index(summary, "P0.1") > strings.Index(summary, "P2.3") {
		t.Fatalf("summary is not in canonical order:\n%s", summary)
	}
	for _, want := range []string{"P0.1", "authentication", "1.0s", "P2.3", "deletion-namespace-gc", "3.0s", "total", "9.0s"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestAppendGitHubStepSummary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", path)
	if err := AppendGitHubStepSummary("Go E2E", "timing table\n"); err != nil {
		t.Fatalf("AppendGitHubStepSummary() error = %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	for _, want := range []string{"### Go E2E", "```text", "timing table"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("step summary missing %q:\n%s", want, b)
		}
	}
}
