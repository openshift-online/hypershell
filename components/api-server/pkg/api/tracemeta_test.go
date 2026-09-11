package api

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestCaptureTraceContext(t *testing.T) {
	traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	spanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")

	tests := []struct {
		name            string
		ctx             context.Context
		wantTraceparent *string
		wantTracestate  *string
	}{
		{
			name:            "no span in context leaves fields nil",
			ctx:             context.Background(),
			wantTraceparent: nil,
			wantTracestate:  nil,
		},
		{
			name:            "invalid span context leaves fields nil",
			ctx:             trace.ContextWithSpanContext(context.Background(), trace.SpanContext{}),
			wantTraceparent: nil,
			wantTracestate:  nil,
		},
		{
			name: "valid span context sets traceparent",
			ctx: trace.ContextWithSpanContext(context.Background(),
				trace.NewSpanContext(trace.SpanContextConfig{
					TraceID:    traceID,
					SpanID:     spanID,
					TraceFlags: trace.FlagsSampled,
					Remote:     true,
				}),
			),
			wantTraceparent: strPtr("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"),
			wantTracestate:  nil,
		},
		{
			name: "valid span context with tracestate sets both fields",
			ctx: trace.ContextWithSpanContext(context.Background(),
				trace.NewSpanContext(trace.SpanContextConfig{
					TraceID:    traceID,
					SpanID:     spanID,
					TraceFlags: trace.FlagsSampled,
					TraceState: mustTraceState(t, "vendor=opaque"),
					Remote:     true,
				}),
			),
			wantTraceparent: strPtr("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"),
			wantTracestate:  strPtr("vendor=opaque"),
		},
		{
			name: "unsampled span still captures traceparent with flags 00",
			ctx: trace.ContextWithSpanContext(context.Background(),
				trace.NewSpanContext(trace.SpanContextConfig{
					TraceID:    traceID,
					SpanID:     spanID,
					TraceFlags: 0,
					Remote:     true,
				}),
			),
			wantTraceparent: strPtr("00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00"),
			wantTracestate:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var tm TraceMeta
			tm.CaptureTraceContext(tc.ctx)

			if !strPtrEq(tm.Traceparent, tc.wantTraceparent) {
				t.Errorf("Traceparent = %s, want %s", strPtrFmt(tm.Traceparent), strPtrFmt(tc.wantTraceparent))
			}
			if !strPtrEq(tm.Tracestate, tc.wantTracestate) {
				t.Errorf("Tracestate = %s, want %s", strPtrFmt(tm.Tracestate), strPtrFmt(tc.wantTracestate))
			}
		})
	}
}

func TestCaptureTraceContextIsIdempotent(t *testing.T) {
	traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	spanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")

	ctx := trace.ContextWithSpanContext(context.Background(),
		trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    traceID,
			SpanID:     spanID,
			TraceFlags: trace.FlagsSampled,
			Remote:     true,
		}),
	)

	var tm TraceMeta
	tm.CaptureTraceContext(ctx)
	first := *tm.Traceparent

	tm.CaptureTraceContext(ctx)
	if *tm.Traceparent != first {
		t.Errorf("second call changed Traceparent: got %s, want %s", *tm.Traceparent, first)
	}
}

func mustTraceState(t *testing.T, s string) trace.TraceState {
	t.Helper()
	ts, err := trace.ParseTraceState(s)
	if err != nil {
		t.Fatalf("ParseTraceState(%q): %v", s, err)
	}
	return ts
}

func strPtr(s string) *string { return &s }

func strPtrEq(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func strPtrFmt(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
