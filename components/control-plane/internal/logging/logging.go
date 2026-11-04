// Package logging installs the control plane's log/slog default logger
// (specs/platform/control-plane-observability.spec.md OBS-CPL-11).
//
// The logger:
//
//  1. writes every record at or above HYPERSHELL_LOG_LEVEL to stderr, in
//     HYPERSHELL_LOG_FORMAT, with trace_id and span_id added when the
//     record's context carries a valid span;
//  2. sends every record at the same level to each added sink, such as the
//     OpenTelemetry log bridge that the otel package adds once the SDK is up.
//
// The OTel SDK reports exporter errors through the standard "log" package.
// Install keeps that package on stderr, so an exporter error never feeds back
// into the log exporter.
package logging

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/trace"
)

// Attribute keys added to stderr records for a context with a valid span.
const (
	TraceIDAttr = "trace_id"
	SpanIDAttr  = "span_id"
)

// Formats accepted by HYPERSHELL_LOG_FORMAT.
const (
	FormatText = "text"
	FormatJSON = "json"
)

var (
	mu      sync.Mutex
	current *settings
)

// settings is what Install was called with, kept so AddSink can rebuild the
// default logger with one more sink.
type settings struct {
	out    io.Writer
	level  slog.Level
	format string
	sinks  []slog.Handler
}

// ParseLevel maps a HYPERSHELL_LOG_LEVEL value to a slog level.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q (want debug, info, warn or error)", s)
	}
}

// ParseFormat checks a HYPERSHELL_LOG_FORMAT value.
func ParseFormat(s string) (string, error) {
	switch f := strings.ToLower(strings.TrimSpace(s)); f {
	case "", FormatText:
		return FormatText, nil
	case FormatJSON:
		return FormatJSON, nil
	default:
		return FormatText, fmt.Errorf("unknown log format %q (want text or json)", s)
	}
}

// Install makes the control-plane logger the slog default. It writes to out
// (os.Stderr in production) at level, in format (FormatText or FormatJSON).
func Install(out io.Writer, level slog.Level, format string) {
	mu.Lock()
	defer mu.Unlock()
	current = &settings{out: out, level: level, format: format}
	apply(current)
}

// AddSink adds a handler that receives every record at or above the installed
// level, besides out. Call it after Install.
func AddSink(h slog.Handler) {
	mu.Lock()
	defer mu.Unlock()
	if current == nil {
		current = &settings{out: os.Stderr, level: slog.LevelInfo, format: FormatText}
	}
	current.sinks = append(current.sinks, h)
	apply(current)
}

func apply(s *settings) {
	slog.SetDefault(slog.New(newHandler(s)))
	// slog.SetDefault routes the standard "log" package through the new
	// handler. Point it back at stderr so exporter errors that the OTel SDK
	// prints with "log" never reach the log exporter.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags)
}

func newHandler(s *settings) slog.Handler {
	opts := &slog.HandlerOptions{Level: s.level}
	var primary slog.Handler
	if s.format == FormatJSON {
		primary = slog.NewJSONHandler(s.out, opts)
	} else {
		primary = slog.NewTextHandler(s.out, opts)
	}
	handlers := []slog.Handler{&traceHandler{next: primary}}
	for _, sink := range s.sinks {
		handlers = append(handlers, &levelHandler{level: s.level, next: sink})
	}
	if len(handlers) == 1 {
		return handlers[0]
	}
	return slog.NewMultiHandler(handlers...)
}

// levelHandler drops records below level before they reach next. The OTel
// bridge enables every level, so it needs this to honor HYPERSHELL_LOG_LEVEL.
type levelHandler struct {
	level slog.Leveler
	next  slog.Handler
}

func (h *levelHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= h.level.Level() && h.next.Enabled(ctx, l)
}

func (h *levelHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.next.Handle(ctx, r)
}

func (h *levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelHandler{level: h.level, next: h.next.WithAttrs(attrs)}
}

func (h *levelHandler) WithGroup(name string) slog.Handler {
	return &levelHandler{level: h.level, next: h.next.WithGroup(name)}
}

// traceHandler adds trace_id and span_id to records whose context carries a
// valid span, so stderr lines can be joined to traces. The OTel bridge reads
// the span from the context itself, so sinks do not need it.
type traceHandler struct {
	next slog.Handler
}

func (h *traceHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r = r.Clone()
		r.AddAttrs(
			slog.String(TraceIDAttr, sc.TraceID().String()),
			slog.String(SpanIDAttr, sc.SpanID().String()),
		)
	}
	return h.next.Handle(ctx, r)
}

func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{next: h.next.WithAttrs(attrs)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{next: h.next.WithGroup(name)}
}
