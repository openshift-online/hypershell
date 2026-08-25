package otel

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// StartReconcileSpan begins a reconcile span named by kind and operation
// (e.g. "reconcile Gateway", "delete Gateway"). Each reconcile starts as a
// new trace root so it is independently sampled and does not grow the
// long-lived watch stream span into a single unbounded trace. The caller
// must call the returned end function when the reconcile completes, passing
// any error. When telemetry is disabled, it returns the original context
// and a no-op end function so there is zero overhead (CP-OBS-01).
func StartReconcileSpan(ctx context.Context, kind, eventType string) (context.Context, func(error)) {
	if !enabled {
		return ctx, func(error) {}
	}

	tracer := otel.Tracer(TracerName)
	spanName := eventType + " " + kind

	ctx, span := tracer.Start(ctx, spanName,
		trace.WithNewRoot(),
		trace.WithAttributes(
			attribute.String("resource.kind", kind),
			attribute.String("event.type", eventType),
		))

	start := time.Now()
	return ctx, func(err error) {
		if err != nil {
			span.SetStatus(codes.Error, sanitizeError(err))
			RecordReconcileError(ctx, kind)
		} else {
			span.SetStatus(codes.Ok, "")
		}
		RecordReconcileDuration(ctx, kind, eventType, start)
		span.End()
	}
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
