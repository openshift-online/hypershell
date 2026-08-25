package otel

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	reconcileDuration metric.Int64Histogram
	reconcileErrors   metric.Int64Counter
	watchReconnects   metric.Int64Counter
)

func registerMetrics() error {
	meter := otel.Meter(TracerName)
	var err error

	reconcileDuration, err = meter.Int64Histogram(
		"reconcile.duration",
		metric.WithUnit("ms"),
		metric.WithDescription("Latency of a single resource reconciliation"),
	)
	if err != nil {
		return err
	}

	reconcileErrors, err = meter.Int64Counter(
		"reconcile.errors",
		metric.WithUnit("{error}"),
		metric.WithDescription("Count of failed reconciliations"),
	)
	if err != nil {
		return err
	}

	watchReconnects, err = meter.Int64Counter(
		"watch.reconnects",
		metric.WithUnit("{reconnect}"),
		metric.WithDescription("Count of watch stream reconnections"),
	)
	return err
}

// RecordReconcileDuration records the duration of a reconcile operation.
func RecordReconcileDuration(ctx context.Context, kind, eventType string, start time.Time) {
	if reconcileDuration == nil {
		return
	}
	reconcileDuration.Record(ctx, time.Since(start).Milliseconds(), metric.WithAttributes(
		attribute.String("resource.kind", kind),
		attribute.String("event.type", eventType),
	))
}

// RecordReconcileError increments the reconcile error counter.
func RecordReconcileError(ctx context.Context, kind string) {
	if reconcileErrors == nil {
		return
	}
	reconcileErrors.Add(ctx, 1, metric.WithAttributes(
		attribute.String("resource.kind", kind),
	))
}

// RecordWatchReconnect increments the watch reconnect counter.
func RecordWatchReconnect(ctx context.Context, kind string) {
	if watchReconnects == nil {
		return
	}
	watchReconnects.Add(ctx, 1, metric.WithAttributes(
		attribute.String("resource.kind", kind),
	))
}
