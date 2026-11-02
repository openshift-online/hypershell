package harness

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestCommandRunnerStreamRedactsMultilinePEM drives a PEM private key block
// through the live streaming path (one line reported as it is produced). The
// per-line redactor cannot match a multi-line block, so the streaming filter must
// collapse the whole block: the key body lines must never reach the structured
// stream. Regression for the streamed-PEM leak.
// Verifies: E2E-CORE-08.01
func TestCommandRunnerStreamRedactsMultilinePEM(t *testing.T) {
	var buf bytes.Buffer
	reporter := NewReporter(&buf, testIdentity())
	runner := NewCommandRunner(func(string, ...any) {})
	runner.SetReporter(reporter, "P1.3")

	pem := "preamble line\n" +
		"-----BEGIN RSA PRIVATE KEY-----\n" +
		"MIIEsecretKeyBodyAAAA\n" +
		"SecondBodyLineBBBB\n" +
		"-----END RSA PRIVATE KEY-----\n" +
		"trailing line\n"
	// cat echoes stdin to stdout, so the suite's streaming reporter path processes
	// each line as it is produced, exactly as it would for openshell CLI output.
	if _, err := runner.RunWithInput(context.Background(), pem, "cat"); err != nil {
		t.Fatalf("cat: %v", err)
	}

	out := buf.String()
	for _, secret := range []string{"MIIEsecretKeyBodyAAAA", "SecondBodyLineBBBB"} {
		if strings.Contains(out, secret) {
			t.Errorf("PEM key body %q leaked through the streamed path:\n%s", secret, out)
		}
	}
	if !strings.Contains(out, "[REDACTED PRIVATE KEY]") {
		t.Errorf("expected a private-key redaction placeholder in the stream:\n%s", out)
	}
	if !strings.Contains(out, "preamble line") || !strings.Contains(out, "trailing line") {
		t.Errorf("non-secret lines around the key should survive:\n%s", out)
	}
}

func TestPemLineFilter(t *testing.T) {
	f := &pemLineFilter{}
	type step struct {
		in   string
		want string
		emit bool
	}
	steps := []step{
		{"ordinary", "ordinary", true},
		{"-----BEGIN EC PRIVATE KEY-----", "[REDACTED PRIVATE KEY]", true},
		{"bodyline1", "", false},
		{"bodyline2", "", false},
		{"-----END EC PRIVATE KEY-----", "", false},
		{"after", "after", true},
		// A complete single-line block is left for the per-line redactor.
		{"-----BEGIN PRIVATE KEY-----aaa-----END PRIVATE KEY-----", "-----BEGIN PRIVATE KEY-----aaa-----END PRIVATE KEY-----", true},
	}
	for i, s := range steps {
		got, emit := f.line(s.in)
		if got != s.want || emit != s.emit {
			t.Errorf("step %d: line(%q) = (%q,%v), want (%q,%v)", i, s.in, got, emit, s.want, s.emit)
		}
	}
}
