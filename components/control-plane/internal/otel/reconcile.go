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
// (e.g. "reconcile Gateway", "delete Gateway"). The caller must call the
// returned end function when the reconcile completes, passing any error.
func StartReconcileSpan(ctx context.Context, kind, eventType string) (context.Context, func(error)) {
	tracer := otel.Tracer(TracerName)
	spanName := eventType + " " + kind

	ctx, span := tracer.Start(ctx, spanName, trace.WithAttributes(
		attribute.String("resource.kind", kind),
		attribute.String("event.type", eventType),
	))

	start := time.Now()
	return ctx, func(err error) {
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			span.RecordError(err)
			RecordReconcileError(ctx, kind)
		} else {
			span.SetStatus(codes.Ok, "")
		}
		RecordReconcileDuration(ctx, kind, eventType, start)
		span.End()
	}
}
