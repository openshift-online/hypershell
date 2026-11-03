package e2e

import (
	"bytes"
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

func TestFailureSummaryFollowsDiagnostics(t *testing.T) {
	if os.Getenv("E2E_FAILURE_SUMMARY_HELPER") == "1" {
		reporter := harness.NewReporter(os.Stdout, harness.ReporterIdentity{RunID: "summary-test"})
		failures := &failureRecorder{}
		s := &E2ESuite{
			logger:   harness.NewLogger(t.Logf, harness.ReporterSink(reporter)).WithStep("P0.1", nil),
			reporter: reporter,
			failures: failures,
		}
		s.SetT(t)
		reportingTest{T: t, logger: s.logger, failures: failures}.Errorf(
			"Error Trace:\t/path/to/phases.go:72\nError:\tintentional failure\nTest:\tTestE2E/P0.1\nMessages:\tactionable reason",
		)
		s.logger = s.logger.WithStep("cleanup", t.Logf)
		for i := 0; i < 2500; i++ {
			s.logger.Additionalf("diagnostic-%04d", i)
		}
		s.reportFailureSummary()
		failure, ok := s.failureSummary()
		if !ok {
			t.Fatal("failure summary was not retained")
		}
		reporter.RunFinished(harness.OutcomeFailed, 0, failure)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestFailureSummaryFollowsDiagnostics$", "-test.v")
	cmd.Env = append(os.Environ(), "E2E_FAILURE_SUMMARY_HELPER=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("helper assertion failure no longer fails test")
	}
	text := string(output)
	summary := "E2E FAILURE SUMMARY [P0.1]: intentional failure | actionable reason"
	if idxSummary, idxLastDiagnostic := strings.LastIndex(text, summary), strings.LastIndex(text, "diagnostic-2499"); idxSummary <= idxLastDiagnostic {
		t.Fatalf("failure summary did not follow diagnostics (summary=%d diagnostic=%d)", idxSummary, idxLastDiagnostic)
	}
	terminal := `"event":"run_finished","runId":"summary-test"`
	if !strings.Contains(text, terminal) || !strings.Contains(text, `"stepId":"P0.1"`) || !strings.Contains(text, `"message":"intentional failure | actionable reason"`) {
		t.Fatal("terminal record lost its structured failure summary")
	}
	if bytes.Count(output, []byte(summary)) != 1 {
		t.Fatal("human failure banner should be emitted once")
	}
}
