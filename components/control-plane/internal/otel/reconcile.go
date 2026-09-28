package otel

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ReconcileEndFunc is the callback returned by StartReconcileSpan. Call it when
// the reconcile completes, passing the outcome (from CRM-006), an optional
// reason code (from CRM-007, required for retryable/failed outcomes), and any
// error. The error is used for span status and logging; outcome and reason
// drive the metrics.
type ReconcileEndFunc func(outcome, reason string, err error)

// StartReconcileSpan begins a reconcile span named by kind and operation
// (e.g. "reconcile Gateway", "delete Gateway"). Each reconcile starts as a
// new trace root so it is independently sampled and does not grow the
// long-lived watch stream span into a single unbounded trace. The caller
// must call the returned end function when the reconcile completes, passing
// the outcome, reason, and error. When telemetry is disabled, it returns the
// original context and a no-op end function so there is zero overhead (CP-OBS-01).
//
// When traceparent is non-empty, the span carries a link to the originating
// request trace (RTC-03). A missing or malformed traceparent produces a
// normal root with no link and no error.
func StartReconcileSpan(ctx context.Context, kind, eventType, traceparent string) (context.Context, ReconcileEndFunc) {
	if !enabled {
		return ctx, func(string, string, error) {}
	}

	tracer := otel.Tracer(TracerName)
	spanName := eventType + " " + kind

	opts := []trace.SpanStartOption{
		trace.WithNewRoot(),
		trace.WithAttributes(
			attribute.String("resource.kind", kind),
			attribute.String("event.type", eventType),
		),
	}

	if link, ok := parseTraceparentLink(traceparent); ok {
		opts = append(opts, trace.WithLinks(link))
	}

	ctx, span := tracer.Start(ctx, spanName, opts...)

	start := time.Now()
	return ctx, func(outcome, reason string, err error) {
		// Record outcome and duration (CRM-008, CRM-009)
		RecordReconcileOutcome(ctx, kind, outcome)
		RecordReconcileDuration(ctx, kind, eventType, outcome, start)
		RecordReconciliationLag(ctx, kind, time.Since(start))

		// Record failure-specific metrics (CRM-010, CRM-011)
		if outcome == OutcomeRetryable {
			RecordReconciliationRetry(ctx, kind, reason)
			RecordReconcileError(ctx, kind)
		} else if outcome == OutcomeFailed {
			RecordReconcileFailedWithReason(ctx, kind, reason)
			RecordReconcileError(ctx, kind)
		}

		// Set span status
		if err != nil {
			span.SetStatus(codes.Error, sanitizeError(err))
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}
}

// parseTraceparentLink parses a W3C traceparent header value and returns a
// span link to the referenced trace/span. Returns false if the value is
// empty, malformed, or contains an invalid trace/span ID.
func parseTraceparentLink(traceparent string) (trace.Link, bool) {
	if traceparent == "" {
		return trace.Link{}, false
	}

	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 {
		return trace.Link{}, false
	}

	traceIDHex := parts[1]
	spanIDHex := parts[2]
	flagsHex := parts[3]

	traceIDBytes, err := hex.DecodeString(traceIDHex)
	if err != nil || len(traceIDBytes) != 16 {
		return trace.Link{}, false
	}
	var traceID trace.TraceID
	copy(traceID[:], traceIDBytes)

	spanIDBytes, err := hex.DecodeString(spanIDHex)
	if err != nil || len(spanIDBytes) != 8 {
		return trace.Link{}, false
	}
	var spanID trace.SpanID
	copy(spanID[:], spanIDBytes)

	if !traceID.IsValid() || !spanID.IsValid() {
		return trace.Link{}, false
	}

	flagsByte, err := hex.DecodeString(flagsHex)
	if err != nil || len(flagsByte) != 1 {
		return trace.Link{}, false
	}

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.TraceFlags(flagsByte[0]),
		Remote:     true,
	})

	return trace.Link{SpanContext: sc}, true
}

// StartWatchSpan begins a lifecycle span for a single watch stream
// connection attempt. When telemetry is disabled it returns the original
// context and a no-op end function (CP-OBS-01).
func StartWatchSpan(ctx context.Context, kind string) (context.Context, func(error)) {
	if !enabled {
		return ctx, func(error) {}
	}

	tracer := otel.Tracer(TracerName)
	ctx, span := tracer.Start(ctx, "watch "+kind, trace.WithAttributes(
		attribute.String("resource.kind", kind),
	))
	// Watch errors are gRPC transport-level (connection reset, EOF) and never
	// carry user data, so RecordError is safe here without sanitization --
	// unlike reconcile errors which may contain namespace/secret references.
	return ctx, func(err error) {
		if err != nil {
			span.SetStatus(codes.Error, "watch disconnected")
			span.RecordError(err)
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}
}

// sanitizeError returns a bounded error class string safe for telemetry
// export, stripping concrete identifiers that may contain namespace names,
// secret references, or usernames.
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	return "reconcile failed"
}

// ClassifyReconcileOutcome returns an outcome and reason from an error using
// heuristics. This is a transitional helper while reconcilers are updated to
// pass explicit outcomes. It defaults to OutcomeRetryable with ReasonUnknown
// for non-nil errors, which is conservative (allows retry). Reconcilers should
// prefer passing explicit outcomes to StartReconcileSpan's end callback.
func ClassifyReconcileOutcome(err error) (outcome, reason string) {
	if err == nil {
		return OutcomeSuccess, ""
	}
	// Default: assume errors are retryable until proven otherwise
	return OutcomeRetryable, ReasonUnknown
}
