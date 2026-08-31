package otel

import (
	"context"
	"testing"
)

func TestSamplerRatio(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want float64
	}{
		{"empty uses default", "", defaultSampleArg},
		{"valid 0.5", "0.5", 0.5},
		{"valid 0", "0", 0},
		{"valid 1", "1", 1},
		{"negative falls back", "-0.1", defaultSampleArg},
		{"above one falls back", "1.5", defaultSampleArg},
		{"NaN falls back", "NaN", defaultSampleArg},
		{"Inf falls back", "Inf", defaultSampleArg},
		{"-Inf falls back", "-Inf", defaultSampleArg},
		{"non-numeric falls back", "abc", defaultSampleArg},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_TRACES_SAMPLER_ARG", tt.env)
			got := samplerRatio()
			if got != tt.want {
				t.Errorf("samplerRatio() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSanitizeError(t *testing.T) {
	if sanitizeError(nil) != "" {
		t.Error("expected empty string for nil error")
	}
	msg := sanitizeError(errWith("secret openshell-abc/db-password leaked"))
	if msg != "reconcile failed" {
		t.Errorf("expected bounded message, got %q", msg)
	}
}

type errWith string

func (e errWith) Error() string { return string(e) }

func TestStartReconcileSpanDisabled(t *testing.T) {
	prev := enabled
	enabled = false
	defer func() { enabled = prev }()

	ctx := context.Background()
	ctx2, end := StartReconcileSpan(ctx, "Fleet", "reconcile", "")
	end(nil)

	if ctx2 != ctx {
		t.Error("disabled StartReconcileSpan should return the same context")
	}
}

func TestParseTraceparentLink(t *testing.T) {
	tests := []struct {
		name        string
		traceparent string
		wantOK      bool
		wantTraceID string
		wantSpanID  string
	}{
		{
			"valid traceparent",
			"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			true,
			"4bf92f3577b34da6a3ce929d0e0e4736",
			"00f067aa0ba902b7",
		},
		{
			"valid unsampled",
			"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00",
			true,
			"4bf92f3577b34da6a3ce929d0e0e4736",
			"00f067aa0ba902b7",
		},
		{"empty string", "", false, "", ""},
		{"too few parts", "00-abc-def", false, "", ""},
		{"too many parts", "00-a-b-c-d", false, "", ""},
		{"all-zero trace ID", "00-00000000000000000000000000000000-00f067aa0ba902b7-01", false, "", ""},
		{"all-zero span ID", "00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01", false, "", ""},
		{"invalid hex in trace ID", "00-ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ-00f067aa0ba902b7-01", false, "", ""},
		{"short trace ID", "00-4bf92f35-00f067aa0ba902b7-01", false, "", ""},
		{"invalid flags hex", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-ZZ", false, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link, ok := parseTraceparentLink(tt.traceparent)
			if ok != tt.wantOK {
				t.Fatalf("parseTraceparentLink(%q) ok = %v, want %v", tt.traceparent, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if got := link.SpanContext.TraceID().String(); got != tt.wantTraceID {
				t.Errorf("traceID = %q, want %q", got, tt.wantTraceID)
			}
			if got := link.SpanContext.SpanID().String(); got != tt.wantSpanID {
				t.Errorf("spanID = %q, want %q", got, tt.wantSpanID)
			}
			if !link.SpanContext.IsRemote() {
				t.Error("link should be marked as remote")
			}
		})
	}
}

func TestStartWatchSpanDisabled(t *testing.T) {
	prev := enabled
	enabled = false
	defer func() { enabled = prev }()

	ctx := context.Background()
	ctx2, end := StartWatchSpan(ctx, "Fleet")
	end(nil)

	if ctx2 != ctx {
		t.Error("disabled StartWatchSpan should return the same context")
	}
}

func TestGRPCDialOptionsDisabled(t *testing.T) {
	prev := enabled
	enabled = false
	defer func() { enabled = prev }()

	opts := GRPCDialOptions()
	if opts != nil {
		t.Error("disabled GRPCDialOptions should return nil")
	}
}

func TestInstrumentK8sConfigDisabled(t *testing.T) {
	prev := enabled
	enabled = false
	defer func() { enabled = prev }()

	if InstrumentK8sConfig(nil) != nil {
		t.Error("disabled InstrumentK8sConfig(nil) should return nil")
	}
}
