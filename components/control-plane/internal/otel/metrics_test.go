package otel

import (
	"context"
	"reflect"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func testMetricsReader(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	previousProvider := otel.GetMeterProvider()
	previousReconcileDuration := reconcileDuration
	previousReconcileQueueDepth := reconcileQueueDepth
	previousReconcileQueueWaitDuration := reconcileQueueWaitDuration
	previousGatewayProvisionDuration := gatewayProvisionDuration
	previousReconcileErrors := reconcileErrors
	previousWatchReconnects := watchReconnects

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	reconcileDuration = nil
	reconcileQueueDepth = nil
	reconcileQueueWaitDuration = nil
	gatewayProvisionDuration = nil
	reconcileErrors = nil
	watchReconnects = nil
	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)
		reconcileDuration = previousReconcileDuration
		reconcileQueueDepth = previousReconcileQueueDepth
		reconcileQueueWaitDuration = previousReconcileQueueWaitDuration
		gatewayProvisionDuration = previousGatewayProvisionDuration
		reconcileErrors = previousReconcileErrors
		watchReconnects = previousWatchReconnects
		_ = provider.Shutdown(context.Background())
	})

	if err := registerMetrics(); err != nil {
		t.Fatalf("registerMetrics() returned an error: %v", err)
	}
	return reader
}

func TestRecordGatewayProvisionDuration(t *testing.T) {
	reader := testMetricsReader(t)

	RecordGatewayProvisionDuration(context.Background(), 37_500*time.Millisecond)
	RecordGatewayProvisionDuration(context.Background(), -1*time.Second)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() returned an error: %v", err)
	}

	for _, scope := range collected.ScopeMetrics {
		for _, gotMetric := range scope.Metrics {
			if gotMetric.Name != "gateway.provision.duration" {
				continue
			}
			if gotMetric.Unit != "s" {
				t.Fatalf("metric unit = %q, want s", gotMetric.Unit)
			}
			histogram, ok := gotMetric.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("metric data type = %T, want float64 histogram", gotMetric.Data)
			}
			if len(histogram.DataPoints) != 1 {
				t.Fatalf("data point count = %d, want 1", len(histogram.DataPoints))
			}
			point := histogram.DataPoints[0]
			if point.Count != 1 {
				t.Fatalf("sample count = %d, want 1", point.Count)
			}
			if point.Sum != 37.5 {
				t.Fatalf("sample sum = %v, want 37.5", point.Sum)
			}
			if point.Attributes.Len() != 0 {
				t.Fatalf("metric attributes = %v, want none", point.Attributes)
			}
			wantBounds := []float64{1, 5, 10, 15, 30, 45, 60, 90, 120, 180, 300, 600, 900}
			if !reflect.DeepEqual(point.Bounds, wantBounds) {
				t.Fatalf("bucket bounds = %v, want %v", point.Bounds, wantBounds)
			}
			return
		}
	}

	t.Fatal("gateway.provision.duration metric was not collected")
}

func TestReconcileQueueMetrics(t *testing.T) {
	reader := testMetricsReader(t)

	depth := int64(3)
	unregister, err := RegisterReconcileQueueDepth("Gateway", func() int64 { return depth })
	if err != nil {
		t.Fatalf("RegisterReconcileQueueDepth() returned an error: %v", err)
	}
	t.Cleanup(func() { _ = unregister() })

	RecordReconcileQueueWaitDuration(context.Background(), "Gateway", 125*time.Millisecond)
	RecordReconcileQueueWaitDuration(context.Background(), "Gateway", -1*time.Second)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() returned an error: %v", err)
	}

	var foundDepth, foundWait bool
	for _, scope := range collected.ScopeMetrics {
		for _, gotMetric := range scope.Metrics {
			switch gotMetric.Name {
			case "reconcile.queue.depth":
				foundDepth = true
				if gotMetric.Unit != "{item}" {
					t.Fatalf("depth metric unit = %q, want {item}", gotMetric.Unit)
				}
				gauge, ok := gotMetric.Data.(metricdata.Gauge[int64])
				if !ok {
					t.Fatalf("depth metric data type = %T, want int64 gauge", gotMetric.Data)
				}
				if len(gauge.DataPoints) != 1 || gauge.DataPoints[0].Value != 3 {
					t.Fatalf("depth data points = %v, want one point with value 3", gauge.DataPoints)
				}
				assertResourceKindAttribute(t, gauge.DataPoints[0].Attributes, "Gateway")
			case "reconcile.queue.wait.duration":
				foundWait = true
				if gotMetric.Unit != "s" {
					t.Fatalf("wait metric unit = %q, want s", gotMetric.Unit)
				}
				histogram, ok := gotMetric.Data.(metricdata.Histogram[float64])
				if !ok {
					t.Fatalf("wait metric data type = %T, want float64 histogram", gotMetric.Data)
				}
				if len(histogram.DataPoints) != 1 {
					t.Fatalf("wait data point count = %d, want 1", len(histogram.DataPoints))
				}
				point := histogram.DataPoints[0]
				if point.Count != 1 || point.Sum != 0.125 {
					t.Fatalf("wait sample = count %d, sum %v; want count 1, sum 0.125", point.Count, point.Sum)
				}
				wantBounds := []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300}
				if !reflect.DeepEqual(point.Bounds, wantBounds) {
					t.Fatalf("wait bucket bounds = %v, want %v", point.Bounds, wantBounds)
				}
				assertResourceKindAttribute(t, point.Attributes, "Gateway")
			}
		}
	}
	if !foundDepth || !foundWait {
		t.Fatalf("queue metrics found = depth %v, wait %v; want both", foundDepth, foundWait)
	}
}

func assertResourceKindAttribute(t *testing.T, attributes attribute.Set, want string) {
	t.Helper()
	if attributes.Len() != 1 {
		t.Fatalf("metric attributes = %v, want only resource.kind", attributes)
	}
	got, ok := attributes.Value(attribute.Key("resource.kind"))
	if !ok || got.AsString() != want {
		t.Fatalf("resource.kind = %v, %v; want %q, true", got, ok, want)
	}
}
