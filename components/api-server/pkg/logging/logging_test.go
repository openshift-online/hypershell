package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/logger"
	serverlogging "github.com/openshift-online/rh-trex-ai/components/api-server/pkg/server/logging"
	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/trex"
	"go.opentelemetry.io/otel/trace"
)

// recordSink keeps the records it receives, standing in for the OTel bridge.
type recordSink struct {
	records *[]slog.Record
}

func (s recordSink) Enabled(context.Context, slog.Level) bool { return true }
func (s recordSink) Handle(_ context.Context, r slog.Record) error {
	*s.records = append(*s.records, r)
	return nil
}
func (s recordSink) WithAttrs([]slog.Attr) slog.Handler { return s }
func (s recordSink) WithGroup(string) slog.Handler      { return s }

// newTestLogger returns a logger built like Install, writing JSON lines to the
// returned buffer and records at or above level to the returned slice.
func newTestLogger(t *testing.T, level slog.Level) (*slog.Logger, *bytes.Buffer, *[]slog.Record) {
	t.Helper()
	var buf bytes.Buffer
	primary, err := logger.NewHandler(&buf, logger.FormatJSON, level)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	var records []slog.Record
	return slog.New(newHandler(withTraceIDs(primary), []slog.Handler{recordSink{&records}}, level)), &buf, &records
}

func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func spanContext(t *testing.T) context.Context {
	t.Helper()
	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	return trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))
}

// Verifies: API-OBS-09.01
func TestStderrRecordsCarryTraceAndRequestAttributes(t *testing.T) {
	l, buf, _ := newTestLogger(t, slog.LevelInfo)
	ctx := logger.WithOpID(spanContext(t))

	l.InfoContext(ctx, "Gateway created", "gateway_id", "g1")
	l.Info("No context")

	lines := decodeLines(t, buf)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %s", len(lines), buf)
	}
	first := lines[0]
	if first[TraceIDAttr] != "4bf92f3577b34da6a3ce929d0e0e4736" || first[SpanIDAttr] != "00f067aa0ba902b7" {
		t.Errorf("trace attributes = %v / %v", first[TraceIDAttr], first[SpanIDAttr])
	}
	if first[logger.OpIDAttr] == nil || first[logger.OpIDAttr] == "" {
		t.Errorf("opid missing: %v", first)
	}
	if _, ok := lines[1][TraceIDAttr]; ok {
		t.Errorf("record without a span has trace_id: %v", lines[1])
	}
}

func TestSinkHonorsLevelAndGetsContextAttributes(t *testing.T) {
	l, _, records := newTestLogger(t, slog.LevelInfo)
	ctx := logger.WithOpID(context.Background())

	l.DebugContext(ctx, "Hidden")
	l.WarnContext(ctx, "Shown")

	if len(*records) != 1 || (*records)[0].Message != "Shown" {
		t.Fatalf("sink records = %v, want only Shown", *records)
	}
	var hasOpID bool
	(*records)[0].Attrs(func(a slog.Attr) bool {
		hasOpID = hasOpID || a.Key == logger.OpIDAttr
		return true
	})
	if !hasOpID {
		t.Error("sink record has no opid")
	}
}

// The framework logs request and response headers and bodies at debug level.
// They must never reach any output, because responses can carry one-time
// credentials. This runs the framework's real middleware, so a rename of its
// messages or fields fails here.
//
// Verifies: API-OBS-09.02
func TestFrameworkHTTPLogsDropHeadersAndBodies(t *testing.T) {
	trex.Init(trex.Config{})
	l, buf, records := newTestLogger(t, slog.LevelDebug)
	previous := slog.Default()
	slog.SetDefault(l)
	t.Cleanup(func() { slog.SetDefault(previous) })

	const secret = "one-time-secret-value"
	handler := serverlogging.RequestLoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"client_secret":"` + secret + `"}`))
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/hypershell/v1/service_accounts", strings.NewReader(`{"password":"`+secret+`"}`))
	req.Header.Set("X-Api-Key", secret)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	if strings.Contains(out, secret) {
		t.Fatalf("stderr output contains the secret: %s", out)
	}
	var seen []string
	for _, line := range decodeLines(t, buf) {
		msg, _ := line["msg"].(string)
		if msg != httpRequestMsg && msg != httpResponseMsg {
			continue
		}
		seen = append(seen, msg)
		details, _ := line[httpDetailsAttr].(string)
		if !strings.Contains(details, "request_method") && !strings.Contains(details, "response_status") {
			t.Errorf("%s details lost their metadata: %q", msg, details)
		}
	}
	if len(seen) != 2 {
		t.Fatalf("saw HTTP records %v, want request and response; output: %s", seen, out)
	}
	for _, r := range *records {
		r.Attrs(func(a slog.Attr) bool {
			if strings.Contains(a.Value.String(), secret) {
				t.Errorf("sink record %q attribute %q contains the secret", r.Message, a.Key)
			}
			return true
		})
	}
}

func TestRedactHTTPDetails(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"drops header and body", `{"request_method":"POST","request_header":{"A":["b"]},"request_body":{}}`, `{"request_method":"POST"}`},
		{"drops response body", `{"response_status":200,"response_body":"x","elapsed":"1ms"}`, `{"elapsed":"1ms","response_status":200}`},
		{"not json", `plain text`, `REDACTED`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactHTTPDetails(tt.in); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}
