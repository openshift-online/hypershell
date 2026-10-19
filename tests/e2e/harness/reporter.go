package harness

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// SchemaE2EV1 is the versioned envelope identifier for the structured run
// reporting stream. A reader separates these records from the human-readable
// suite output by matching this schema field. Keep it in sync with the
// e2e-run-log-visibility.spec.md consumer contract.
const SchemaE2EV1 = "hypershell.e2e.v1"

// Structured event names.
const (
	eventRunStarted   = "run_started"
	eventStepStarted  = "step_started"
	eventStepFinished = "step_finished"
	eventLog          = "log"
	eventRunFinished  = "run_finished"
)

// Step and run outcomes carried by step_finished and run_finished.
const (
	OutcomePassed  = "passed"
	OutcomeFailed  = "failed"
	OutcomeSkipped = "skipped"
)

// maxLogBody bounds a single structured log record's message so a runaway
// command line cannot emit an unbounded record. Longer messages are truncated
// with an explicit marker and flagged.
const maxLogBody = 3072

// maxRecord bounds the marshaled NDJSON line (including its trailing newline) so
// a single Write stays at or below the POSIX PIPE_BUF (4096 bytes) guarantee:
// writes at or below that size are atomic on the pod stdout pipe, so a record
// never tears or interleaves with the suite's own stdout even though both share
// the stream. maxLogBody bounds the message pre-marshal, but JSON string escaping
// (a control byte expands to a six-byte \uXXXX) can inflate the marshaled record
// past the raw budget, so emit re-checks the marshaled size and shrinks the
// message until the whole line fits rather than trusting the pre-marshal bound.
const maxRecord = 4096

// PlannedStep names one step in the planned set carried by the run_started
// event, so a consumer can distinguish a step that never ran (fail-fast) from
// one that ran and was skipped.
type PlannedStep struct {
	StepID string `json:"stepId"`
	Name   string `json:"name"`
}

// ReporterIdentity is the run identity supplied by the environment (by
// hypershell-gitops in production). Every record carries it so a consumer can
// correlate a run to a promotion by revision and attempt id.
type ReporterIdentity struct {
	RunID          string
	Revision       string
	SourceRevision string
	Cluster        string
	Namespace      string
}

// event is the on-the-wire NDJSON envelope. Optional fields are omitted when
// empty so each record stays compact; schema, event, runId, sequence, and
// timestamp are always present.
type event struct {
	Schema         string        `json:"schema"`
	Event          string        `json:"event"`
	RunID          string        `json:"runId"`
	Revision       string        `json:"revision,omitempty"`
	SourceRevision string        `json:"sourceRevision,omitempty"`
	Cluster        string        `json:"cluster,omitempty"`
	Namespace      string        `json:"namespace,omitempty"`
	Sequence       int64         `json:"sequence"`
	Timestamp      string        `json:"timestamp"`
	StepID         string        `json:"stepId,omitempty"`
	Name           string        `json:"name,omitempty"`
	Outcome        string        `json:"outcome,omitempty"`
	DurationMs     *int64        `json:"durationMs,omitempty"`
	Message        string        `json:"message,omitempty"`
	Truncated      bool          `json:"truncated,omitempty"`
	PlannedSteps   []PlannedStep `json:"plannedSteps,omitempty"`
}

// Reporter emits one NDJSON object per line to an underlying writer, immediately
// as each event occurs, through a single serialized writer shared across all
// parallel subtests and goroutines so lines are never interleaved or torn. A nil
// *Reporter is a valid disabled reporter: every method is a no-op, so call sites
// invoke it unconditionally and the feature stays off by default.
type Reporter struct {
	identity ReporterIdentity

	mu  sync.Mutex
	w   io.Writer
	seq int64
}

// NewReporterFromEnv builds a Reporter from the E2E_REPORT_* environment when
// E2E_REPORT_JSON=1, writing to w; otherwise it returns nil (the disabled
// reporter). When E2E_REPORT_RUN_ID is absent it generates a UUID rather than
// reuse the suite's short, non-unique internal run id.
func NewReporterFromEnv(w io.Writer) *Reporter {
	if os.Getenv("E2E_REPORT_JSON") != "1" {
		return nil
	}
	id := ReporterIdentity{
		RunID:          os.Getenv("E2E_REPORT_RUN_ID"),
		Revision:       os.Getenv("E2E_REPORT_REVISION"),
		SourceRevision: os.Getenv("E2E_REPORT_SOURCE_REVISION"),
		Cluster:        os.Getenv("E2E_REPORT_CLUSTER"),
		Namespace:      os.Getenv("E2E_REPORT_NAMESPACE"),
	}
	if id.RunID == "" {
		id.RunID = newUUID()
	}
	return NewReporter(w, id)
}

// NewReporter returns an enabled Reporter writing to w with the given identity.
// It is used directly by tests; production builds through NewReporterFromEnv.
func NewReporter(w io.Writer, identity ReporterIdentity) *Reporter {
	return &Reporter{w: w, identity: identity}
}

// Enabled reports whether structured reporting is active. It is nil-safe.
func (r *Reporter) Enabled() bool { return r != nil }

// RunStarted emits the run_started event naming the planned set of steps.
func (r *Reporter) RunStarted(planned []PlannedStep) {
	r.emit(event{Event: eventRunStarted, PlannedSteps: planned})
}

// StepStarted emits a step_started event for the step id and slug.
func (r *Reporter) StepStarted(stepID, name string) {
	r.emit(event{Event: eventStepStarted, StepID: stepID, Name: name})
}

// StepFinished emits a step_finished event carrying the outcome and duration.
func (r *Reporter) StepFinished(stepID, name, outcome string, d time.Duration) {
	ms := d.Milliseconds()
	r.emit(event{Event: eventStepFinished, StepID: stepID, Name: name, Outcome: outcome, DurationMs: &ms})
}

// Log emits a redacted, size-bounded log record. stepID is empty for a
// run-scoped line that is not attributable to a specific step; a caller must not
// guess an owning step for ambiguous concurrent output.
func (r *Reporter) Log(stepID, message string) {
	if r == nil {
		return
	}
	body, truncated := boundAndRedact(message)
	r.emit(event{Event: eventLog, StepID: stepID, Message: body, Truncated: truncated})
}

// RunFinished emits the terminal run_finished event. It must be called only
// after cleanup has completed so the terminal record observes cleanup errors; a
// run killed before this record is emitted reads to a consumer as
// incomplete/unknown, never as passed.
func (r *Reporter) RunFinished(outcome string, d time.Duration) {
	ms := d.Milliseconds()
	r.emit(event{Event: eventRunFinished, Outcome: outcome, DurationMs: &ms})
}

// emit stamps the shared envelope fields and writes one line under the mutex. A
// write or marshal error is reported on stderr (visible) but never changes the
// suite's verdict or exit code.
func (r *Reporter) emit(ev event) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	ev.Schema = SchemaE2EV1
	ev.Sequence = r.seq
	ev.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	ev.RunID = r.identity.RunID
	ev.Revision = r.identity.Revision
	ev.SourceRevision = r.identity.SourceRevision
	ev.Cluster = r.identity.Cluster
	ev.Namespace = r.identity.Namespace

	b, err := marshalBounded(ev)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e reporter: marshal %s event: %v\n", ev.Event, err)
		return
	}
	b = append(b, '\n')
	if _, err := r.w.Write(b); err != nil {
		fmt.Fprintf(os.Stderr, "e2e reporter: write %s event: %v\n", ev.Event, err)
	}
}

// marshalBounded marshals ev to a single NDJSON line guaranteed to fit, with its
// trailing newline, within maxRecord so the direct stdout write stays atomic. The
// Message field is the only variable-length, unbounded-by-escaping field; when the
// marshaled line overruns the budget it is shortened and the record re-marshaled
// until it fits, flagging the truncation. The envelope fields are bounded by
// construction, so a record with no message that still overruns (it cannot in
// practice) is returned as-is rather than looping forever.
func marshalBounded(ev event) ([]byte, error) {
	b, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	for len(b)+1 > maxRecord && len(ev.Message) > 0 {
		// Drop at least the overflow, and never fewer than a chunk, so escape-heavy
		// expansion converges in a bounded number of iterations.
		over := len(b) + 1 - maxRecord
		if over < 128 {
			over = 128
		}
		if over >= len(ev.Message) {
			ev.Message = ""
		} else {
			ev.Message = ev.Message[:len(ev.Message)-over]
		}
		ev.Truncated = true
		b, err = json.Marshal(ev)
		if err != nil {
			return nil, err
		}
	}
	return b, nil
}

// boundAndRedact scrubs secrets from s and then bounds its length, returning the
// safe body and whether it was truncated.
func boundAndRedact(s string) (string, bool) {
	s = redact(s)
	if len(s) > maxLogBody {
		return fmt.Sprintf("%s ...[truncated %d bytes]", s[:maxLogBody], len(s)-maxLogBody), true
	}
	return s, false
}

// redactPatterns scrubs credential material from log bodies. Each pattern
// replaces the secret value while keeping enough surrounding text that the line
// stays recognizable. Order does not matter; all are applied.
var redactPatterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	// Sensitive CLI options now also appear in command-description events.
	{regexp.MustCompile(`(?i)(--(?:password|token|access-token|client-secret|client_secret|secret)(?:=|\s+))(?:(?:"[^"]*")|(?:'[^']*')|[^\s]+)`), "${1}[REDACTED]"},
	// PEM private key blocks (any key type), including the body.
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`), "[REDACTED PRIVATE KEY]"},
	// Authorization: Bearer / Basic <token> headers and bare bearer tokens.
	{regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)\S+`), "${1}[REDACTED]"},
	{regexp.MustCompile(`(?i)(authorization\s*:\s*basic\s+)\S+`), "${1}[REDACTED]"},
	{regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/-]+=*`), "Bearer [REDACTED]"},
	// Credentials embedded in a connection-string URL (e.g. postgres://user:pass@host):
	// keep the scheme and user, scrub the password.
	{regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^:@/\s]+:)[^@/\s]+@`), "${1}[REDACTED]@"},
	// JWT-shaped tokens (header.payload.signature) that leak without a prefix.
	{regexp.MustCompile(`\beyJ[A-Za-z0-9._-]{10,}`), "[REDACTED]"},
	// key=value / key: value / key "value" secrets for common sensitive keys.
	{regexp.MustCompile(`(?i)(client[_-]?secret["']?\s*[=:]\s*["']?)[^"'\s,}]+`), "${1}[REDACTED]"},
	{regexp.MustCompile(`(?i)(password["']?\s*[=:]\s*["']?)[^"'\s,}]+`), "${1}[REDACTED]"},
	{regexp.MustCompile(`(?i)((?:access|refresh|id|api|secret)[_-]?token["']?\s*[=:]\s*["']?)[^"'\s,}]+`), "${1}[REDACTED]"},
}

var (
	pemPrivateBegin = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)
	pemPrivateEnd   = regexp.MustCompile(`-----END [A-Z0-9 ]*PRIVATE KEY-----`)
)

// pemLineFilter collapses a multi-line PEM private key block, which the per-line
// redact() cannot match (its regex needs the BEGIN and END markers in one call),
// into a single placeholder across a line-at-a-time stream. The streaming command
// path reports output one line as it is produced, so a key body arrives as
// separate lines after the BEGIN marker; without this filter those base64 body
// lines would pass through unredacted. It is stateful and not safe for concurrent
// use; each streaming command run owns one, fed by the single os/exec copy
// goroutine.
type pemLineFilter struct {
	inKey bool
}

// line feeds one raw output line through the filter and returns the line to emit
// (if any) and whether to emit it. A complete BEGIN...END block on one line is
// left for the per-line redactor; a BEGIN with no END opens suppression: the
// BEGIN line becomes a single placeholder and every following line is swallowed
// until the END marker (or end of stream).
func (f *pemLineFilter) line(s string) (string, bool) {
	if f.inKey {
		if pemPrivateEnd.MatchString(s) {
			f.inKey = false
		}
		return "", false
	}
	if pemPrivateBegin.MatchString(s) && !pemPrivateEnd.MatchString(s) {
		f.inKey = true
		return "[REDACTED PRIVATE KEY]", true
	}
	return s, true
}

// redact removes bearer tokens, Authorization headers (Bearer and Basic), client
// secrets, private keys, connection-string passwords, and other credential
// material from a structured log body before it is emitted.
//
// Scope note: this scrubs the STRUCTURED stream only. The suite's human-readable
// output (CommandRunner.Show command lines and t.Logf) also reaches pod stdout
// and is not redacted, so the suite MUST NOT pass secrets as command arguments or
// log them in the clear; the streaming path redacts command OUTPUT, which is
// where credentials could legitimately surface.
func redact(s string) string {
	for _, p := range redactPatterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}
	return s
}

// newUUID returns a random RFC 4122 version 4 UUID. The suite has no UUID
// dependency, so this uses crypto/rand directly; on the vanishingly unlikely
// read failure it falls back to a timestamp-based id so reporting still has a
// non-empty run id.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	var sb strings.Builder
	fmt.Fprintf(&sb, "%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	return sb.String()
}
