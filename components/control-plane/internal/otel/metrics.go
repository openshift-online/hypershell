package otel

import (
	"context"
	"fmt"
	"sync"
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
	gatewayProvisionOutcomes   metric.Int64Counter
	gatewaySandboxOrphaned     metric.Int64Gauge
	gatewaySandboxExpiring     metric.Int64Gauge
	gatewaySandboxIdle         metric.Int64Gauge
	reconcileErrors            metric.Int64Counter
	reconciliationFailures     metric.Int64Counter
	reconciliationRetries      metric.Int64Counter
	reconciliationLag          metric.Float64Histogram
	staleResourceStatusCount   metric.Int64Gauge
	watchReconnects            metric.Int64Counter
)

var staleResources sync.Map
var staleResourcesMu sync.Mutex

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

	gatewayProvisionOutcomes, err = meter.Int64Counter(
		"gateway.provision.outcomes",
		metric.WithUnit("{outcome}"),
		metric.WithDescription("Count of terminal gateway provision outcomes (success or failure)"),
	)
	if err != nil {
		return err
	}

	gatewaySandboxOrphaned, err = meter.Int64Gauge(
		"gateway.sandbox.orphaned",
		metric.WithUnit("{sandbox}"),
		metric.WithDescription("Cluster-local orphaned active sandbox count"),
	)
	if err != nil {
		return err
	}

	gatewaySandboxExpiring, err = meter.Int64Gauge(
		"gateway.sandbox.expiring",
		metric.WithUnit("{sandbox}"),
		metric.WithDescription("Cluster-local expiring-soon active sandbox count"),
	)
	if err != nil {
		return err
	}

	gatewaySandboxIdle, err = meter.Int64Gauge(
		"gateway.sandbox.idle",
		metric.WithUnit("{sandbox}"),
		metric.WithDescription("Cluster-local idle active sandbox count"),
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

	reconciliationFailures, err = meter.Int64Counter(
		"hypershell.reconciliation.failures",
		metric.WithUnit("{failure}"),
		metric.WithDescription("Count of failed control-plane reconciliation attempts"),
	)
	if err != nil {
		return err
	}
	reconciliationFailures.Add(context.Background(), 0)

	reconciliationRetries, err = meter.Int64Counter(
		"hypershell.reconciliation.retries",
		metric.WithUnit("{retry}"),
		metric.WithDescription("Count of control-plane reconciliation retries"),
	)
	if err != nil {
		return err
	}
	reconciliationRetries.Add(context.Background(), 0)

	reconciliationLag, err = meter.Float64Histogram(
		"hypershell.reconciliation.lag",
		metric.WithUnit("s"),
		metric.WithDescription("Time spent processing a control-plane reconciliation"),
		metric.WithExplicitBucketBoundaries(0.001, 0.01, 0.1, 1, 5, 10, 30, 60, 300),
	)
	if err != nil {
		return err
	}

	staleResourceStatusCount, err = meter.Int64Gauge(
		"hypershell.stale.resource.status.count",
		metric.WithUnit("{resource}"),
		metric.WithDescription("Resources whose observed status has not caught up with desired state"),
	)
	if err != nil {
		return err
	}
	staleResourceStatusCount.Record(context.Background(), 0, metric.WithAttributes(
		attribute.String("hypershell.cluster_id", DefaultAttentionClusterID),
	))

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
		return nil, fmt.Errorf("register reconcile queue depth callback: %w", err)
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

// RecordGatewayProvisionOutcome records one terminal provision outcome. The
// caller owns the one-observation rule for each Gateway.
func RecordGatewayProvisionOutcome(ctx context.Context, outcome string) {
	if gatewayProvisionOutcomes == nil || outcome == "" {
		return
	}
	gatewayProvisionOutcomes.Add(ctx, 1, metric.WithAttributes(
		attribute.String("outcome", outcome),
	))
}

// DefaultAttentionClusterID is the hypershell.cluster_id attribute value when
// the control plane runs in single-cluster mode (empty ClusterID).
const DefaultAttentionClusterID = "default"

// AttentionClusterID returns a low-cardinality cluster key for attention gauges.
// Empty clusterID maps to DefaultAttentionClusterID so hub and spoke series share
// a stable label for PromQL max-by-cluster dedupe of control-plane replicas.
func AttentionClusterID(clusterID string) string {
	if clusterID == "" {
		return DefaultAttentionClusterID
	}
	return clusterID
}

// RecordSandboxAttentionCounts updates the cluster-local attention gauges from
// the most recent sandbox attention reconcile tick (SSA-06). clusterID labels
// the series so fleet PromQL can max-by-cluster before summing (replica dedupe).
func RecordSandboxAttentionCounts(ctx context.Context, clusterID string, orphaned, expiring, idle int) {
	attrs := metric.WithAttributes(
		attribute.String("hypershell.cluster_id", AttentionClusterID(clusterID)),
	)
	if gatewaySandboxOrphaned != nil {
		gatewaySandboxOrphaned.Record(ctx, int64(max(0, orphaned)), attrs)
	}
	if gatewaySandboxExpiring != nil {
		gatewaySandboxExpiring.Record(ctx, int64(max(0, expiring)), attrs)
	}
	if gatewaySandboxIdle != nil {
		gatewaySandboxIdle.Record(ctx, int64(max(0, idle)), attrs)
	}
}

// RecordReconcileError increments the reconcile error counter.
func RecordReconcileError(ctx context.Context, kind string) {
	if reconcileErrors == nil {
		return
	}
	reconcileErrors.Add(ctx, 1, metric.WithAttributes(
		attribute.String("resource.kind", kind),
	))
	if reconciliationFailures != nil {
		reconciliationFailures.Add(ctx, 1, metric.WithAttributes(
			attribute.String("resource.kind", kind),
		))
	}
}

// RecordReconciliationRetry records a retry scheduled for a reconciliation.
func RecordReconciliationRetry(ctx context.Context, kind string) {
	if reconciliationRetries == nil {
		return
	}
	reconciliationRetries.Add(ctx, 1, metric.WithAttributes(
		attribute.String("resource.kind", kind),
	))
}

// RecordReconciliationLag records the elapsed reconciliation processing time.
func RecordReconciliationLag(ctx context.Context, kind string, duration time.Duration) {
	if reconciliationLag == nil || duration < 0 {
		return
	}
	reconciliationLag.Record(ctx, duration.Seconds(), metric.WithAttributes(
		attribute.String("resource.kind", kind),
	))
}

// SetResourceStatusStale records whether a resource's observed status is
// behind its desired state. Resource IDs remain in process memory only and are
// never exported as metric labels.
func SetResourceStatusStale(ctx context.Context, clusterID, kind, resourceID string, stale bool) {
	if staleResourceStatusCount == nil || kind == "" || resourceID == "" {
		return
	}
	staleResourcesMu.Lock()
	defer staleResourcesMu.Unlock()
	key := kind + "\x00" + resourceID
	if stale {
		staleResources.Store(key, struct{}{})
	} else {
		staleResources.Delete(key)
	}
	count := int64(0)
	staleResources.Range(func(_, _ any) bool { count++; return true })
	staleResourceStatusCount.Record(ctx, count, metric.WithAttributes(
		attribute.String("hypershell.cluster_id", AttentionClusterID(clusterID)),
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
