package e2e

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/openshift-online/hypershell/tests/e2e/harness"
)

func TestAssertionFanout(t *testing.T) {
	if os.Getenv("E2E_LOGGING_ASSERTION_HELPER") == "1" {
		reporter := harness.NewReporter(os.Stdout, harness.ReporterIdentity{RunID: "assertion-test"})
		s := &E2ESuite{logger: harness.NewLogger(t.Logf, harness.ReporterSink(reporter)).WithStep("api-check", nil)}
		s.SetT(t)
		s.Require().Equal("ready", "pending", "readiness assertion")
		t.Fatal("Require did not preserve FailNow")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestAssertionFanout$", "-test.v")
	cmd.Env = append(os.Environ(), "E2E_LOGGING_ASSERTION_HELPER=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("assertion failure no longer fails test")
	}
	text := string(output)
	if !strings.Contains(text, `"stepId":"api-check"`) || !strings.Contains(text, "readiness assertion") || !strings.Contains(text, "Not equal") {
		t.Fatalf("missing structured or human diagnostic: %s", text)
	}
	if strings.Contains(text, "Require did not preserve FailNow") {
		t.Fatal("fatal behavior changed")
	}
}
