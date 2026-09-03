package otel

import (
	"context"
	"reflect"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestRecordGatewayProvisionDuration(t *testing.T) {
	previousProvider := otel.GetMeterProvider()
	previousReconcileDuration := reconcileDuration
	previousGatewayProvisionDuration := gatewayProvisionDuration
	previousReconcileErrors := reconcileErrors
	previousWatchReconnects := watchReconnects

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	reconcileDuration = nil
	gatewayProvisionDuration = nil
	reconcileErrors = nil
	watchReconnects = nil
	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)
		reconcileDuration = previousReconcileDuration
		gatewayProvisionDuration = previousGatewayProvisionDuration
		reconcileErrors = previousReconcileErrors
		watchReconnects = previousWatchReconnects
		_ = provider.Shutdown(context.Background())
	})

	if err := registerMetrics(); err != nil {
		t.Fatalf("registerMetrics() returned an error: %v", err)
	}

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
