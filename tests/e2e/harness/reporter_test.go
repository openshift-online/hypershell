package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// decodeEvents parses the NDJSON lines a Reporter wrote into events.
func decodeEvents(t *testing.T, b []byte) []event {
	t.Helper()
	var events []event
	for _, line := range bytes.Split(bytes.TrimRight(b, "\n"), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var ev event
		if err := json.Unmarshal(line, &ev); err != nil {
			t.Fatalf("decode NDJSON line %q: %v", line, err)
		}
		events = append(events, ev)
	}
	return events
}

func testIdentity() ReporterIdentity {
	return ReporterIdentity{
		RunID:          "pod-uid-1",
		Revision:       "hydrated-sha",
		SourceRevision: "source-sha",
		Cluster:        "hub-00",
		Namespace:      "hypershell",
	}
}

func TestReporterNilIsDisabledNoOp(t *testing.T) {
	var r *Reporter
	if r.Enabled() {
		t.Fatal("nil reporter should report disabled")
	}
	// None of these should panic or write anything.
	r.RunStarted([]PlannedStep{{StepID: "P1.1", Name: "provisioning"}})
	r.StepStarted("P1.1", "provisioning")
	r.Log("P1.1", "line")
	r.StepFinished("P1.1", "provisioning", OutcomePassed, time.Second)
	r.RunFinished(OutcomePassed, time.Second)
}

// Verifies: E2E-CORE-06.02, E2E-CORE-07.01
func TestReporterEmitsVersionedEnvelope(t *testing.T) {
	var buf bytes.Buffer
	r := NewReporter(&buf, testIdentity())

	r.RunStarted([]PlannedStep{{StepID: "P1.1", Name: "provisioning"}})
	r.StepStarted("P1.1", "provisioning")
	r.StepFinished("P1.1", "provisioning", OutcomePassed, 1500*time.Millisecond)
	r.RunFinished(OutcomePassed, 3*time.Second)

	events := decodeEvents(t, buf.Bytes())
	if len(events) != 4 {
		t.Fatalf("want 4 events, got %d", len(events))
	}

	wantEventOrder := []string{eventRunStarted, eventStepStarted, eventStepFinished, eventRunFinished}
	var lastSeq int64
	for i, ev := range events {
		if ev.Schema != SchemaE2EV1 {
			t.Errorf("event %d: schema = %q, want %q", i, ev.Schema, SchemaE2EV1)
		}
		if ev.Event != wantEventOrder[i] {
			t.Errorf("event %d: event = %q, want %q", i, ev.Event, wantEventOrder[i])
		}
		if ev.RunID != "pod-uid-1" || ev.Revision != "hydrated-sha" || ev.SourceRevision != "source-sha" || ev.Cluster != "hub-00" || ev.Namespace != "hypershell" {
			t.Errorf("event %d: identity not carried: %+v", i, ev)
		}
		if _, err := time.Parse(time.RFC3339Nano, ev.Timestamp); err != nil {
			t.Errorf("event %d: timestamp %q not RFC3339Nano: %v", i, ev.Timestamp, err)
		}
		if ev.Sequence <= lastSeq {
			t.Errorf("event %d: sequence %d not strictly increasing (last %d)", i, ev.Sequence, lastSeq)
		}
		lastSeq = ev.Sequence
	}

	if got := events[0].PlannedSteps; len(got) != 1 || got[0].StepID != "P1.1" {
		t.Errorf("run_started planned steps = %+v, want one P1.1", got)
	}
	if events[2].Outcome != OutcomePassed {
		t.Errorf("step_finished outcome = %q, want passed", events[2].Outcome)
	}
	if events[2].DurationMs == nil || *events[2].DurationMs != 1500 {
		t.Errorf("step_finished durationMs = %v, want 1500", events[2].DurationMs)
	}
}

func TestReporterRunIDGeneratedWhenAbsent(t *testing.T) {
	var buf bytes.Buffer
	r := NewReporter(&buf, ReporterIdentity{}) // no supplied run id
	r.RunStarted(nil)

	events := decodeEvents(t, buf.Bytes())
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	// NewReporter does not generate; NewReporterFromEnv does. Here we assert the
	// empty-identity path still emits a well-formed record (run id empty).
	if events[0].Schema != SchemaE2EV1 {
		t.Fatalf("schema = %q", events[0].Schema)
	}

	// newUUID must return a distinct, non-empty, RFC4122-shaped value each call.
	a, b := newUUID(), newUUID()
	if a == "" || b == "" || a == b {
		t.Fatalf("newUUID not unique/non-empty: %q %q", a, b)
	}
	if len(strings.Split(a, "-")) != 5 {
		t.Fatalf("newUUID not UUID-shaped: %q", a)
	}
}

// Verifies: E2E-CORE-08.01
func TestReporterRedactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	r := NewReporter(&buf, testIdentity())

	cases := []string{
		"Authorization: Bearer abc.def.ghi-secret-value",
		"curl -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiumBearerTokenValue'",
		"Authorization: Basic dXNlcjpiYXNpY1Bhc3N3b3Jk",
		`{"client_secret":"super-secret-value","ok":true}`,
		"password=hunter2 extra",
		"--access-token=eyJ0b2tlbnZhbHVl1234567890abcdef",
		"connecting to postgres://dbuser:conn-str-secret@db.internal:5432/gw",
		"-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA\n-----END RSA PRIVATE KEY-----",
	}
	secrets := []string{"abc.def.ghi-secret-value", "super-secret-value", "hunter2", "MIIEpAIBAAKCAQEA", "BearerTokenValue", "dXNlcjpiYXNpY1Bhc3N3b3Jk", "conn-str-secret"}

	for _, c := range cases {
		r.Log("P1.1", c)
	}

	out := buf.String()
	for _, secret := range secrets {
		if strings.Contains(out, secret) {
			t.Errorf("secret %q leaked into structured records:\n%s", secret, out)
		}
	}
	if !strings.Contains(out, "[REDACTED") {
		t.Errorf("expected a redaction marker in output:\n%s", out)
	}
}

// Verifies: E2E-CORE-08.03
func TestReporterBoundsOversizedLogLine(t *testing.T) {
	var buf bytes.Buffer
	r := NewReporter(&buf, testIdentity())

	long := strings.Repeat("x", maxLogBody+5000)
	r.Log("P1.1", long)

	events := decodeEvents(t, buf.Bytes())
	if len(events) != 1 {
		t.Fatalf("want 1 event, got %d", len(events))
	}
	if !events[0].Truncated {
		t.Error("oversized line should be flagged truncated")
	}
	if len(events[0].Message) >= len(long) {
		t.Errorf("message not bounded: len=%d, input len=%d", len(events[0].Message), len(long))
	}
	if !strings.Contains(events[0].Message, "truncated") {
		t.Error("truncated message should carry an explicit marker")
	}
}

func TestReporterConcurrentWritesAreNotTorn(t *testing.T) {
	var buf bytes.Buffer
	r := NewReporter(&buf, testIdentity())

	const writers = 8
	const perWriter = 50
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				r.Log("P2.1", "concurrent line")
			}
		}()
	}
	wg.Wait()

	events := decodeEvents(t, buf.Bytes())
	if len(events) != writers*perWriter {
		t.Fatalf("want %d events, got %d (lines torn or dropped)", writers*perWriter, len(events))
	}
	// Sequences must be unique across all records.
	seen := make(map[int64]bool, len(events))
	for _, ev := range events {
		if seen[ev.Sequence] {
			t.Fatalf("duplicate sequence %d", ev.Sequence)
		}
		seen[ev.Sequence] = true
	}
}

// Verifies: E2E-CORE-08.03
func TestMarshalBoundedKeepsRecordWithinPipeBuf(t *testing.T) {
	// A control byte expands to a six-byte \uXXXX under JSON escaping, so a message
	// at the raw maxLogBody bound can marshal far past it. The record (plus its
	// newline) must still fit maxRecord so the direct stdout write stays atomic and
	// cannot tear or interleave with the suite's own stdout.
	ev := event{
		Schema:  SchemaE2EV1,
		Event:   eventLog,
		RunID:   "pod-uid-1",
		Message: strings.Repeat("\x00", maxLogBody),
	}
	b, err := marshalBounded(ev)
	if err != nil {
		t.Fatalf("marshalBounded: %v", err)
	}
	if len(b)+1 > maxRecord {
		t.Fatalf("marshaled record %d bytes exceeds maxRecord %d", len(b)+1, maxRecord)
	}
	var got event
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("bounded record is not valid JSON: %v", err)
	}
	if !got.Truncated {
		t.Error("a shortened record should be flagged truncated")
	}
}

func TestReporterWrittenLineNeverExceedsPipeBuf(t *testing.T) {
	var buf bytes.Buffer
	r := NewReporter(&buf, testIdentity())
	// Escape-heavy content: quotes, backslashes, and control bytes all expand.
	r.Log("P1.1", strings.Repeat("\"\\\n\t\x00", 2000))

	for _, line := range bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n")) {
		if len(line)+1 > maxRecord {
			t.Fatalf("written line %d bytes exceeds maxRecord %d", len(line)+1, maxRecord)
		}
	}
}

// failingWriter errors on every Write to prove a reporting failure is isolated.
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return 0, errWriteFailed }

var errWriteFailed = errors.New("write failed")

func TestReporterWriteErrorDoesNotPanic(t *testing.T) {
	r := NewReporter(failingWriter{}, testIdentity())
	// Must not panic; the error is reported on stderr and the call returns.
	r.RunStarted(nil)
	r.Log("P1.1", "line")
	r.RunFinished(OutcomeFailed, time.Second)
}
