package harness

import (
	"fmt"
	"io"
	"strings"
)

// LogRecord carries immutable step attribution to each additional log sink.
// Sinks must be safe for concurrent calls and must not change the test verdict.
type LogRecord struct{ StepID, Message string }
type LogSink func(LogRecord)

// Logger fans out diagnostic messages to the existing Go-test output and optional
// sinks. A scoped logger is immutable, so parallel steps never share a mutable
// current-step field. The human sink retains its original formatting and timing.
type Logger struct {
	human Logf
	step  string
	sinks []LogSink
}

func NewLogger(human Logf, sinks ...LogSink) *Logger {
	return &Logger{human: human, sinks: append([]LogSink(nil), sinks...)}
}

func (l *Logger) WithStep(step string, human Logf) *Logger {
	if human == nil {
		human = l.human
	}
	return &Logger{human: human, step: step, sinks: l.sinks}
}

func (l *Logger) Logf(format string, args ...any) {
	l.Humanf(format, args...)
	l.Additionalf(format, args...)
}
func (l *Logger) Log(args ...any) { l.Logf("%s", fmt.Sprintln(args...)) }
func (l *Logger) Humanf(format string, args ...any) {
	if l != nil && l.human != nil {
		l.human(format, args...)
	}
}

// Additionalf mirrors messages already written by the Go test framework (such as
// assertion failures), without duplicating them in the human-readable output.
func (l *Logger) Additionalf(format string, args ...any) {
	if l == nil {
		return
	}
	record := LogRecord{StepID: l.step, Message: fmt.Sprintf(format, args...)}
	for _, sink := range l.sinks {
		sink(record)
	}
}

// ReporterSink is the optional NDJSON destination. Redact whole diagnostic
// messages before splitting so multiline assertions and key blocks stay safe.
func ReporterSink(reporter *Reporter) LogSink {
	return func(record LogRecord) {
		if !reporter.Enabled() {
			return
		}
		filter := &pemLineFilter{}
		for _, line := range strings.Split(strings.TrimRight(redact(record.Message), "\n"), "\n") {
			if safe, ok := filter.line(line); ok {
				reporter.Log(record.StepID, safe)
			}
		}
	}
}

// Stream creates an independently scoped line writer for a subprocess. Its
// human-readable output is still logged once, on completion; extra sinks get
// lines immediately. Each command owns its PEM state, including partial lines.
func (l *Logger) Stream() interface {
	io.Writer
	Flush()
} {
	filter := &pemLineFilter{}
	return newLineStreamer(func(line string) {
		if safe, ok := filter.line(line); ok {
			l.Additionalf("%s", safe)
		}
	})
}
