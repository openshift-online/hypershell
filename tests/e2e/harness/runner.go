package harness

import (
	"bytes"
	"context"
	"fmt"
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
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if out != "" {
		r.logf("%s", strings.TrimRight(out, "\n"))
	}
	if err != nil {
		return out, fmt.Errorf("%s %s: %w\noutput:\n%s", name, strings.Join(args, " "), err, out)
	}
	return out, nil
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
