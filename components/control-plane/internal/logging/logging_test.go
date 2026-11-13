package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

// restoreDefault puts back the slog default and the package state after a test.
func restoreDefault(t *testing.T) {
	t.Helper()
	prior := slog.Default()
	priorOut := log.Writer()
	mu.Lock()
	priorSettings := current
	mu.Unlock()
	t.Cleanup(func() {
		slog.SetDefault(prior)
		log.SetOutput(priorOut)
		mu.Lock()
		current = priorSettings
		mu.Unlock()
	})
}

func spanContext(t *testing.T) context.Context {
	t.Helper()
	tid, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatal(err)
	}
	sid, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatal(err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid, TraceFlags: trace.FlagsSampled})
	return trace.ContextWithSpanContext(context.Background(), sc)
}

func decode(t *testing.T, line string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("decoding %q: %v", line, err)
	}
	return m
}

// Verifies: OBS-CPL-11.01
func TestStderrRecordCarriesTraceIDs(t *testing.T) {
	restoreDefault(t)
	var out bytes.Buffer
	Install(&out, slog.LevelInfo, FormatJSON)

	slog.InfoContext(spanContext(t), "gateway reconciled", "gateway_id", "gw-1")

	m := decode(t, strings.TrimSpace(out.String()))
	if m[TraceIDAttr] != "4bf92f3577b34da6a3ce929d0e0e4736" || m[SpanIDAttr] != "00f067aa0ba902b7" {
		t.Fatalf("trace attributes missing: %v", m)
	}
	if m["msg"] != "gateway reconciled" || m["gateway_id"] != "gw-1" {
		t.Fatalf("record content: %v", m)
	}
}

func TestRecordWithoutSpanHasNoTraceIDs(t *testing.T) {
	restoreDefault(t)
	var out bytes.Buffer
	Install(&out, slog.LevelInfo, FormatJSON)

	slog.Info("starting")

	m := decode(t, strings.TrimSpace(out.String()))
	if _, ok := m[TraceIDAttr]; ok {
		t.Fatalf("unexpected trace_id: %v", m)
	}
}

// Verifies: OBS-CPL-11.02
func TestLevelFilterAppliesToStderrAndSinks(t *testing.T) {
	restoreDefault(t)
	var out bytes.Buffer
	sink := &recordingHandler{}
	Install(&out, slog.LevelWarn, FormatText)
	AddSink(sink)

	slog.Info("dropped")
	slog.Debug("dropped too")
	slog.WarnContext(spanContext(t), "kept")

	if strings.Contains(out.String(), "dropped") || !strings.Contains(out.String(), "kept") {
		t.Fatalf("stderr level filter: %q", out.String())
	}
	got := sink.messages()
	if len(got) != 1 || got[0] != "kept" {
		t.Fatalf("sink level filter: %v", got)
	}
}

func TestStdlibLogStaysOnStderr(t *testing.T) {
	restoreDefault(t)
	var out bytes.Buffer
	sink := &recordingHandler{}
	Install(&out, slog.LevelDebug, FormatText)
	AddSink(sink)

	log.Print("exporter error")

	if len(sink.messages()) != 0 {
		t.Fatalf("stdlib log reached the sink: %v", sink.messages())
	}
	if strings.Contains(out.String(), "exporter error") {
		t.Fatalf("stdlib log went through the slog handler: %q", out.String())
	}
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"": slog.LevelInfo, "info": slog.LevelInfo, "DEBUG": slog.LevelDebug,
		"warn": slog.LevelWarn, "warning": slog.LevelWarn, "error": slog.LevelError,
	} {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseLevel("verbose"); err == nil {
		t.Error("ParseLevel(verbose) succeeded")
	}
}

func TestParseFormat(t *testing.T) {
	for in, want := range map[string]string{"": FormatText, "text": FormatText, "JSON": FormatJSON} {
		got, err := ParseFormat(in)
		if err != nil || got != want {
			t.Errorf("ParseFormat(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseFormat("logfmt"); err == nil {
		t.Error("ParseFormat(logfmt) succeeded")
	}
}

// recordingHandler keeps the messages it receives. Like the OTel bridge, it
// enables every level.
type recordingHandler struct {
	mu   sync.Mutex
	msgs []string
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.msgs = append(h.msgs, r.Message)
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.msgs...)
}
