package otel

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	reconcileDuration          metric.Int64Histogram
	reconcileQueueDepth        metric.Int64ObservableGauge
	reconcileQueueWaitDuration metric.Float64Histogram
	gatewayProvisionDuration   metric.Float64Histogram
	reconcileErrors            metric.Int64Counter
	watchReconnects            metric.Int64Counter
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

	reconcileQueueDepth, err = meter.Int64ObservableGauge(
		"reconcile.queue.depth",
		metric.WithUnit("{item}"),
		metric.WithDescription("Ready resource keys that are waiting for a reconcile worker"),
	)
	if err != nil {
		return err
	}

	reconcileQueueWaitDuration, err = meter.Float64Histogram(
		"reconcile.queue.wait.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Time from when a resource key becomes ready until a worker starts reconciliation"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300),
	)
	if err != nil {
		return err
	}

	gatewayProvisionDuration, err = meter.Float64Histogram(
		"gateway.provision.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Time from Gateway creation until its first successful transition to Running"),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 15, 30, 45, 60, 90, 120, 180, 300, 600, 900),
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

// RegisterReconcileQueueDepth registers one callback for a reconcile queue.
// The returned function removes the callback when the queue stops.
func RegisterReconcileQueueDepth(kind string, depth func() int64) (func() error, error) {
	if reconcileQueueDepth == nil {
		return func() error { return nil }, nil
	}
	registration, err := otel.Meter(TracerName).RegisterCallback(
		func(_ context.Context, observer metric.Observer) error {
			observer.ObserveInt64(reconcileQueueDepth, depth(), metric.WithAttributes(
				attribute.String("resource.kind", kind),
			))
			return nil
		},
		reconcileQueueDepth,
	)
	if err != nil {
		return nil, err
	}
	return registration.Unregister, nil
}

// RecordReconcileQueueWaitDuration records the time that a ready resource key
// waited before a worker started its reconciliation.
func RecordReconcileQueueWaitDuration(ctx context.Context, kind string, duration time.Duration) {
	if reconcileQueueWaitDuration == nil || duration < 0 {
		return
	}
	reconcileQueueWaitDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("resource.kind", kind),
	))
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

// RecordGatewayProvisionDuration records one successful create-to-Running
// duration. The caller owns the one-observation rule for each Gateway.
func RecordGatewayProvisionDuration(ctx context.Context, duration time.Duration) {
	if gatewayProvisionDuration == nil || duration < 0 {
		return
	}
	gatewayProvisionDuration.Record(ctx, duration.Seconds())
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
