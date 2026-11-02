package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestLoggerFanout(t *testing.T) {
	var human []string
	var structured bytes.Buffer
	reporter := NewReporter(&structured, testIdentity())
	logger := NewLogger(func(f string, args ...any) { human = append(human, fmt.Sprintf(f, args...)) }, ReporterSink(reporter)).WithStep("api", nil)
	runner := NewCommandRunner(nil)
	runner.SetLogger(logger)
	runner.Show("GET /gateways")
	runner.Comment("waiting for readiness")
	logger.Logf("phase=%s", "Running")
	logger.Additionalf("assertion failed: %s", "expected 200")
	if len(human) != 3 {
		t.Fatalf("human output changed/duplicated: %v", human)
	}
	for _, message := range []string{"$ GET /gateways", "# waiting for readiness", "phase=Running", "assertion failed"} {
		if !strings.Contains(structured.String(), message) {
			t.Errorf("missing %q", message)
		}
	}
	if strings.Count(structured.String(), `"stepId":"api"`) != 4 {
		t.Fatal("lost step attribution")
	}
}

// Verifies: E2E-CORE-09.02
func TestLoggerDisabledRetainsHumanOutput(t *testing.T) {
	var messages []string
	logger := NewLogger(func(f string, args ...any) { messages = append(messages, fmt.Sprintf(f, args...)) }, ReporterSink(nil))
	runner := NewCommandRunner(nil)
	runner.SetLogger(logger)
	out, err := runner.RunWithInput(context.Background(), "hello\n", "cat")
	if err != nil || out != "hello\n" {
		t.Fatalf("command behavior changed: %q %v", out, err)
	}
	if fmt.Sprint(messages) != "[$ cat  hello]" {
		t.Fatalf("unexpected human messages %q", messages)
	}
}

// Verifies: E2E-CORE-08.01
func TestLoggerStreamHasNoDuplicatesAndRedactsDiagnostics(t *testing.T) {
	var structured bytes.Buffer
	var human []string
	logger := NewLogger(func(f string, args ...any) { human = append(human, fmt.Sprintf(f, args...)) }, ReporterSink(NewReporter(&structured, testIdentity()))).WithStep("setup", nil)
	runner := NewCommandRunner(nil)
	runner.SetLogger(logger)
	logger.Logf("failed request\nAuthorization: Bearer secret-value\n-----BEGIN PRIVATE KEY-----\nprivate-data\n-----END PRIVATE KEY-----")
	runner.Show("client --client-secret 'cli credential' --token=abc")
	if _, err := runner.RunWithInput(context.Background(), "one-line\n", "cat"); err != nil {
		t.Fatal(err)
	}
	text := structured.String()
	for _, secret := range []string{"secret-value", "private-data", "cli credential", "--token=abc"} {
		if strings.Contains(text, secret) {
			t.Errorf("structured diagnostic leaks %q", secret)
		}
	}
	if strings.Count(text, "one-line") != 1 {
		t.Fatalf("duplicated command output: %s", text)
	}
	if !strings.Contains(strings.Join(human, "\n"), "secret-value") {
		t.Fatal("human sink no longer preserves original messages")
	}
}

func TestRedactDiagnosticProtectsHumanLogsAndArtifacts(t *testing.T) {
	raw := "Authorization: Bearer secret-value\n" +
		"database=postgres://user:password@db.example.test/app\n" +
		"-----BEGIN PRIVATE KEY-----\nprivate-data\n-----END PRIVATE KEY-----\n" +
		"access_token=eyJheader.payload.signature"
	safe := RedactDiagnostic(raw)
	for _, secret := range []string{"secret-value", ":password@", "private-data", "eyJheader"} {
		if strings.Contains(safe, secret) {
			t.Errorf("redacted diagnostic leaks %q: %s", secret, safe)
		}
	}
	if !strings.Contains(safe, "[REDACTED PRIVATE KEY]") {
		t.Fatalf("redacted diagnostic lost private-key placeholder: %s", safe)
	}
}

func TestLoggerConcurrentScopes(t *testing.T) {
	var structured bytes.Buffer
	base := NewLogger(nil, ReporterSink(NewReporter(&structured, testIdentity())))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprint(i)
			logger := base.WithStep(id, nil)
			for n := 0; n < 20; n++ {
				logger.Logf("owner=%s", id)
			}
		}(i)
	}
	wg.Wait()
	decoder := json.NewDecoder(&structured)
	var previous int64
	for decoder.More() {
		var ev event
		if err := decoder.Decode(&ev); err != nil {
			t.Fatal(err)
		}
		if ev.Message != "owner="+ev.StepID {
			t.Fatalf("cross-step leak: %+v", ev)
		}
		if ev.Sequence <= previous {
			t.Fatal("sequence not monotonic")
		}
		previous = ev.Sequence
	}
	if previous != 160 {
		t.Fatalf("lost records: %d", previous)
	}
}
