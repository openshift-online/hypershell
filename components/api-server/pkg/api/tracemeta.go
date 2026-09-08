package api

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"
)

// TraceMeta embeds alongside api.Meta to persist the originating W3C Trace
// Context on every resource. The json:"-" tag keeps these fields out of REST
// API responses (RTC-05). The columns are nullable so pre-existing rows and
// resources created with telemetry disabled have NULL trace context.
type TraceMeta struct {
	Traceparent *string `json:"-" gorm:"column:traceparent"`
	Tracestate  *string `json:"-" gorm:"column:tracestate"`
}

// CaptureTraceContext extracts the active span's W3C traceparent and
// tracestate from ctx and stores them. When no valid span is active (OTel
// disabled or no sampled span), the fields are left nil.
//
// On an update path this is deliberate: the DAO persists via gorm Save, which
// writes nil pointers as NULL, so an update performed with telemetry disabled
// overwrites any previously-stored trace context to NULL. That is the intended
// overwrite-on-update semantics (RTC-01) - the columns always describe the most
// recent mutation, and a mutation with no active span has no traceable origin.
func (t *TraceMeta) CaptureTraceContext(ctx context.Context) {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return
	}
	tp := fmt.Sprintf("00-%s-%s-%s", sc.TraceID(), sc.SpanID(), sc.TraceFlags())
	t.Traceparent = &tp
	if ts := sc.TraceState().String(); ts != "" {
		t.Tracestate = &ts
	}
}
