// Package logging installs the API server's log/slog default logger.
//
// The rh-trex-ai framework logs only through log/slog and passes the request
// context with every record. HyperShell installs its own default logger before
// the framework configures logging, so the framework keeps it
// (logger.SetDefaultIfUnset). That logger:
//
//  1. writes every record to stderr, in the --log-format and at the
//     --log-level the framework parsed, with trace_id and span_id added when
//     the context carries a span;
//  2. sends every record at the same level to each registered sink, such as
//     the OpenTelemetry log bridge the otel plugin registers;
//  3. adds the framework's request attributes (opid, accountID) to both;
//  4. removes HTTP headers and bodies from the framework's request and
//     response records, because some HyperShell responses carry one-time
//     credentials.
package logging

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"sync"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/config"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/logger"
	"go.opentelemetry.io/otel/trace"
)

// Attribute keys added to stderr records for a context with a valid span.
const (
	TraceIDAttr = "trace_id"
	SpanIDAttr  = "span_id"
)

var (
	mu    sync.Mutex
	sinks []slog.Handler
)

// RegisterSink adds a handler that receives every record at or above the
// configured level, besides stderr. Call it from a plugin's init(), before
// Install runs.
func RegisterSink(h slog.Handler) {
	mu.Lock()
	defer mu.Unlock()
	sinks = append(sinks, h)
}

// Install builds the HyperShell logger from the framework's logging
// configuration and makes it the slog default. Call it after flags are parsed
// and before the framework initializes the environment.
func Install(cfg *config.LoggingConfig) error {
	level, err := cfg.SlogLevel()
	if err != nil {
		return err
	}
	stderr, err := logger.NewHandler(os.Stderr, cfg.Format, level)
	if err != nil {
		return err
	}

	mu.Lock()
	registered := append([]slog.Handler(nil), sinks...)
	mu.Unlock()

	slog.SetDefault(slog.New(newHandler(withTraceIDs(stderr), registered, level)))
	// slog.SetDefault also routes the standard "log" package through the new
	// handler. Keep it on stderr, as the framework does, so an exporter error
	// that the OTel SDK prints with "log" never feeds back into the exporter.
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags)
	return nil
}

// newHandler assembles the handler chain described in the package comment.
func newHandler(primary slog.Handler, sinks []slog.Handler, level slog.Leveler) slog.Handler {
	handlers := []slog.Handler{primary}
	for _, s := range sinks {
		handlers = append(handlers, &levelHandler{level: level, next: s})
	}
	return logger.WithContextAttrs(&httpRedactHandler{next: slog.NewMultiHandler(handlers...)})
}

// levelHandler drops records below level before they reach next. The OTel
// bridge enables every level, so it needs this to honor --log-level.
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

// withTraceIDs adds trace_id and span_id to records whose context carries a
// valid span, so stderr lines can be joined to traces.
func withTraceIDs(h slog.Handler) slog.Handler {
	return &traceHandler{next: h}
}

type traceHandler struct {
	next slog.Handler
}

func (h *traceHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			r = r.Clone()
			r.AddAttrs(
				slog.String(TraceIDAttr, sc.TraceID().String()),
				slog.String(SpanIDAttr, sc.SpanID().String()),
			)
		}
	}
	return h.next.Handle(ctx, r)
}

func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{next: h.next.WithAttrs(attrs)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{next: h.next.WithGroup(name)}
}

// The framework's HTTP logging middleware (pkg/server/logging) logs these
// messages with the request or response as JSON in a "details" attribute. At
// debug level it adds the headers and bodies.
const (
	httpRequestMsg  = "HTTP request"
	httpResponseMsg = "HTTP response"
	httpDetailsAttr = "details"
)

// httpDetailFields are the framework's JSON fields that can hold credentials.
var httpDetailFields = []string{"request_header", "request_body", "response_header", "response_body"}

// httpRedactHandler removes headers and bodies from the framework's HTTP
// request and response records, at every level. HyperShell responses can carry
// one-time credentials (for example a new service-account secret), and the
// framework only redacts credential headers.
type httpRedactHandler struct {
	next slog.Handler
}

func (h *httpRedactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.next.Enabled(ctx, l)
}

func (h *httpRedactHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Message != httpRequestMsg && r.Message != httpResponseMsg {
		return h.next.Handle(ctx, r)
	}
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == httpDetailsAttr && a.Value.Kind() == slog.KindString {
			a = slog.String(a.Key, redactHTTPDetails(a.Value.String()))
		}
		out.AddAttrs(a)
		return true
	})
	return h.next.Handle(ctx, out)
}

func (h *httpRedactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &httpRedactHandler{next: h.next.WithAttrs(attrs)}
}

func (h *httpRedactHandler) WithGroup(name string) slog.Handler {
	return &httpRedactHandler{next: h.next.WithGroup(name)}
}

// redactHTTPDetails drops the header and body fields from the framework's JSON
// details. Details that are not a JSON object are replaced, because they
// cannot be checked.
func redactHTTPDetails(details string) string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(details), &fields); err != nil {
		return "REDACTED"
	}
	for _, f := range httpDetailFields {
		delete(fields, f)
	}
	out, err := json.Marshal(fields)
	if err != nil {
		return "REDACTED"
	}
	return string(out)
}
