package harness

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Logf is the logging sink the CommandRunner writes through. The suite passes
// t.Logf so output interleaves correctly with testify's per-subtest grouping and
// is attributed to the right step.
type Logf func(format string, args ...any)

// CommandRunner renders every infrastructure or API operation as a human-readable
// command line before executing it, so a captured run doubles as a demonstration
// of the commands a user would run by hand. It mirrors the Bash show_cmd helper
// (which survives in the retained smoke script).
type CommandRunner struct {
	logf  Logf
	color bool

	// reporter, when non-nil, enables the opt-in structured streaming path:
	// command output is emitted as redacted log records as it is produced, in
	// addition to the unchanged combined-output capture and return values.
	reporter *Reporter
	// stepID attributes streamed log records to the owning step; empty means the
	// output is run-scoped (not attributable to a specific step).
	stepID string
}

const (
	colorCyan  = "\033[36m"
	colorReset = "\033[0m"
)

// NewCommandRunner returns a runner that logs through logf. Color is emitted only
// when stdout is a TTY and NO_COLOR is unset; under go test output capture the
// runner emits plain text.
func NewCommandRunner(logf Logf) *CommandRunner {
	return &CommandRunner{logf: logf, color: useColor()}
}

// SetReporter attaches the structured reporter and the owning step id for the
// streaming path. A nil reporter leaves the runner on its unchanged
// combined-output-only behavior. stepID may be empty for run-scoped output.
func (r *CommandRunner) SetReporter(reporter *Reporter, stepID string) {
	r.reporter = reporter
	r.stepID = stepID
}

// SetStep updates the step attribution for subsequently streamed output. It is
// used by the sequential gate path, where one shared runner serves successive
// steps that never run concurrently.
func (r *CommandRunner) SetStep(stepID string) {
	r.stepID = stepID
}

func useColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// Show logs a `$ `-prefixed command line for an operation the runner is about to
// perform. Use it for the illustrative CLI/API command equivalent of an SDK or
// Keycloak admin call that is not a literal shell command.
func (r *CommandRunner) Show(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	if r.color {
		r.logf("%s$ %s%s", colorCyan, line, colorReset)
		return
	}
	r.logf("$ %s", line)
}

// Comment logs a `# `-prefixed note, for context around an illustrative command
// (for example explaining a Keycloak admin API call), matching the Bash suite.
func (r *CommandRunner) Comment(format string, args ...any) {
	r.logf("# %s", fmt.Sprintf(format, args...))
}

// Run echoes the command, then executes it with the suite context, capturing
// combined stdout/stderr. The output is logged and, on failure, included in the
// returned error so a reader sees why it failed. This is the path for the one
// product binary the suite invokes directly, the openshell CLI.
func (r *CommandRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	return r.RunWithInput(ctx, "", name, args...)
}

// RunWithInput is Run with stdin supplied.
func (r *CommandRunner) RunWithInput(ctx context.Context, stdin, name string, args ...string) (string, error) {
	return r.run(ctx, nil, stdin, name, args...)
}

// RunWithEnv runs a command with an explicit environment (for example the openshell
// CLI, which needs SSL_CERT_FILE and the TLS bypass), echoing it and capturing
// combined output.
func (r *CommandRunner) RunWithEnv(ctx context.Context, env []string, name string, args ...string) (string, error) {
	return r.run(ctx, env, "", name, args...)
}

func (r *CommandRunner) run(ctx context.Context, env []string, stdin, name string, args ...string) (string, error) {
	r.Show("%s %s", name, strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, name, args...)
	if env != nil {
		cmd.Env = env
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var buf bytes.Buffer
	var streamer *lineStreamer
	if r.reporter.Enabled() {
		// Tee combined output to a line scanner so records stream as the command
		// produces them, before the combined buffer finishes. os/exec guarantees a
		// single writer goroutine when Stdout and Stderr are the same comparable
		// writer, so the shared streamer needs no internal locking.
		streamer = newLineStreamer(func(line string) { r.reporter.Log(r.stepID, line) })
		w := io.MultiWriter(&buf, streamer)
		cmd.Stdout = w
		cmd.Stderr = w
	} else {
		cmd.Stdout = &buf
		cmd.Stderr = &buf
	}
	err := cmd.Run()
	if streamer != nil {
		streamer.Flush()
	}
	out := buf.String()
	if out != "" {
		r.logf("%s", strings.TrimRight(out, "\n"))
	}
	if err != nil {
		return out, fmt.Errorf("%s %s: %w\noutput:\n%s", name, strings.Join(args, " "), err, out)
	}
	return out, nil
}

// lineStreamer splits an output stream into newline-delimited lines and invokes
// emit for each complete line, so command output is reported live. It buffers a
// trailing partial line until the next write or a final Flush. It is written to
// by a single os/exec copy goroutine, so it needs no internal synchronization.
type lineStreamer struct {
	emit func(line string)
	buf  []byte
}

func newLineStreamer(emit func(line string)) *lineStreamer {
	return &lineStreamer{emit: emit}
}

func (l *lineStreamer) Write(p []byte) (int, error) {
	l.buf = append(l.buf, p...)
	for {
		i := bytes.IndexByte(l.buf, '\n')
		if i < 0 {
			break
		}
		l.emit(strings.TrimRight(string(l.buf[:i]), "\r"))
		l.buf = l.buf[i+1:]
	}
	return len(p), nil
}

// Flush emits any buffered trailing partial line that was not newline-terminated.
func (l *lineStreamer) Flush() {
	if len(l.buf) > 0 {
		l.emit(string(l.buf))
		l.buf = nil
	}
}

// Poll calls fn every interval until it reports done, the timeout elapses, or the
// context is cancelled. fn returning an error aborts immediately. It is the shared
// wait primitive for gateway provisioning, pod readiness, namespace GC, and role
// propagation, so every wait honors the suite's context-derived deadline.
func Poll(ctx context.Context, interval, timeout time.Duration, fn func(context.Context) (done bool, err error)) error {
	deadline := time.Now().Add(timeout)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		done, err := fn(ctx)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out after %s: %w", timeout, ctx.Err())
		case <-ticker.C:
		}
	}
}

// Retry calls fn up to attempts times, waiting delay between tries, returning nil
// on the first success and the last error if all attempts fail. It honors the
// context between attempts.
func Retry(ctx context.Context, attempts int, delay time.Duration, fn func() error) error {
	var err error
	for i := range attempts {
		if err = fn(); err == nil {
			return nil
		}
		if i == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled after %d attempts: %w", i+1, ctx.Err())
		case <-time.After(delay):
		}
	}
	return fmt.Errorf("all %d attempts failed: %w", attempts, err)
}
