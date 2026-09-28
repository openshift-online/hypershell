package otel

import (
	"context"
	"fmt"
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
	previousGatewayProvisionOutcomes := gatewayProvisionOutcomes
	previousGatewaySandboxOrphaned := gatewaySandboxOrphaned
	previousGatewaySandboxExpiring := gatewaySandboxExpiring
	previousGatewaySandboxIdle := gatewaySandboxIdle
	previousReconcileErrors := reconcileErrors
	previousReconciliationFailures := reconciliationFailures
	previousReconciliationRetries := reconciliationRetries
	previousReconciliationLag := reconciliationLag
	previousStaleResourceStatusCount := staleResourceStatusCount
	previousWatchReconnects := watchReconnects
	previousReconcileOutcomes := reconcileOutcomes
	previousReconcileFailed := reconcileFailed

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	reconcileDuration = nil
	reconcileQueueDepth = nil
	reconcileQueueWaitDuration = nil
	gatewayProvisionDuration = nil
	gatewayProvisionOutcomes = nil
	gatewaySandboxOrphaned = nil
	gatewaySandboxExpiring = nil
	gatewaySandboxIdle = nil
	reconcileErrors = nil
	reconciliationFailures = nil
	reconciliationRetries = nil
	reconciliationLag = nil
	staleResourceStatusCount = nil
	watchReconnects = nil
	reconcileOutcomes = nil
	reconcileFailed = nil
	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)
		reconcileDuration = previousReconcileDuration
		reconcileQueueDepth = previousReconcileQueueDepth
		reconcileQueueWaitDuration = previousReconcileQueueWaitDuration
		gatewayProvisionDuration = previousGatewayProvisionDuration
		gatewayProvisionOutcomes = previousGatewayProvisionOutcomes
		gatewaySandboxOrphaned = previousGatewaySandboxOrphaned
		gatewaySandboxExpiring = previousGatewaySandboxExpiring
		gatewaySandboxIdle = previousGatewaySandboxIdle
		reconcileErrors = previousReconcileErrors
		reconciliationFailures = previousReconciliationFailures
		reconciliationRetries = previousReconciliationRetries
		reconciliationLag = previousReconciliationLag
		staleResourceStatusCount = previousStaleResourceStatusCount
		watchReconnects = previousWatchReconnects
		reconcileOutcomes = previousReconcileOutcomes
		reconcileFailed = previousReconcileFailed
		_ = provider.Shutdown(context.Background())
	})

	if err := registerMetrics(); err != nil {
		t.Fatalf("registerMetrics() returned an error: %v", err)
	}
	return reader
}

func TestReconciliationMetrics(t *testing.T) {
	reader := testMetricsReader(t)
	RecordReconcileError(context.Background(), "Gateway")
	RecordReconciliationRetry(context.Background(), "Gateway", ReasonK8sConflict)
	RecordReconciliationLag(context.Background(), "Gateway", 2*time.Second)
	SetResourceStatusStale(context.Background(), "cluster-a", "Gateway", "one", true)
	SetResourceStatusStale(context.Background(), "cluster-a", "Gateway", "one", true)
	SetResourceStatusStale(context.Background(), "cluster-a", "Gateway", "two", true)
	SetResourceStatusStale(context.Background(), "cluster-a", "Gateway", "one", false)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() returned an error: %v", err)
	}
	for _, scope := range collected.ScopeMetrics {
		for _, got := range scope.Metrics {
			switch got.Name {
			case "hypershell.reconciliation.failures":
				if _, ok := got.Data.(metricdata.Sum[int64]); !ok {
					t.Fatalf("failures type = %T", got.Data)
				}
			case "hypershell.reconciliation.retries":
				if _, ok := got.Data.(metricdata.Sum[int64]); !ok {
					t.Fatalf("retries type = %T", got.Data)
				}
			case "hypershell.reconciliation.lag":
				h := got.Data.(metricdata.Histogram[float64])
				want := []float64{0.001, 0.01, 0.1, 1, 5, 10, 30, 60, 300}
				if !reflect.DeepEqual(h.DataPoints[0].Bounds, want) {
					t.Fatalf("lag bounds = %v", h.DataPoints[0].Bounds)
				}
			case "hypershell.stale.resource.status.count":
				g := got.Data.(metricdata.Gauge[int64])
				found := false
				for _, point := range g.DataPoints {
					v, ok := point.Attributes.Value(attribute.Key("hypershell.cluster_id"))
					if ok && v.AsString() == "cluster-a" {
						found = true
						if point.Value != 1 {
							t.Fatalf("stale gauge = %d, want 1", point.Value)
						}
					}
				}
				if !found {
					t.Fatal("stale cluster label was not exported")
				}
			}
		}
	}
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

func TestRecordGatewayProvisionOutcome(t *testing.T) {
	reader := testMetricsReader(t)

	RecordGatewayProvisionOutcome(context.Background(), "success")
	RecordGatewayProvisionOutcome(context.Background(), "failure")
	RecordGatewayProvisionOutcome(context.Background(), "")

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() returned an error: %v", err)
	}

	var found bool
	var successCount, failureCount int64
	for _, scope := range collected.ScopeMetrics {
		for _, gotMetric := range scope.Metrics {
			if gotMetric.Name != "gateway.provision.outcomes" {
				continue
			}
			found = true
			if gotMetric.Unit != "{outcome}" {
				t.Fatalf("metric unit = %q, want {outcome}", gotMetric.Unit)
			}
			counter, ok := gotMetric.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("metric data type = %T, want int64 sum", gotMetric.Data)
			}
			for _, point := range counter.DataPoints {
				outcome, ok := point.Attributes.Value(attribute.Key("outcome"))
				if !ok {
					t.Fatalf("missing outcome attribute on %v", point.Attributes)
				}
				switch outcome.AsString() {
				case "success":
					successCount = point.Value
				case "failure":
					failureCount = point.Value
				default:
					t.Fatalf("unexpected outcome = %q", outcome.AsString())
				}
			}
		}
	}

	if !found {
		t.Fatal("gateway.provision.outcomes metric was not collected")
	}
	if successCount != 1 || failureCount != 1 {
		t.Fatalf("outcome counts = success %d, failure %d; want 1 each", successCount, failureCount)
	}
}

func TestRecordSandboxAttentionCounts(t *testing.T) {
	reader := testMetricsReader(t)

	RecordSandboxAttentionCounts(context.Background(), "cluster-a", 3, 12, 7)

	assertSandboxAttentionGauges(t, reader, "cluster-a", 3, 12, 7)
}

func TestRecordSandboxAttentionCountsFloorsNegatives(t *testing.T) {
	reader := testMetricsReader(t)

	RecordSandboxAttentionCounts(context.Background(), "", -1, -2, -3)

	assertSandboxAttentionGauges(t, reader, DefaultAttentionClusterID, 0, 0, 0)
}

func TestAttentionClusterID(t *testing.T) {
	if got := AttentionClusterID(""); got != DefaultAttentionClusterID {
		t.Fatalf("AttentionClusterID(\"\") = %q, want %q", got, DefaultAttentionClusterID)
	}
	if got := AttentionClusterID("spoke-1"); got != "spoke-1" {
		t.Fatalf("AttentionClusterID(spoke-1) = %q, want spoke-1", got)
	}
}

func assertSandboxAttentionGauges(t *testing.T, reader *sdkmetric.ManualReader, clusterID string, orphaned, expiring, idle int64) {
	t.Helper()
	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() returned an error: %v", err)
	}

	want := map[string]int64{
		"gateway.sandbox.orphaned": orphaned,
		"gateway.sandbox.expiring": expiring,
		"gateway.sandbox.idle":     idle,
	}
	found := map[string]bool{}
	for _, scope := range collected.ScopeMetrics {
		for _, gotMetric := range scope.Metrics {
			wantValue, ok := want[gotMetric.Name]
			if !ok {
				continue
			}
			found[gotMetric.Name] = true
			if gotMetric.Unit != "{sandbox}" {
				t.Fatalf("%s unit = %q, want {sandbox}", gotMetric.Name, gotMetric.Unit)
			}
			gauge, ok := gotMetric.Data.(metricdata.Gauge[int64])
			if !ok {
				t.Fatalf("%s data type = %T, want int64 gauge", gotMetric.Name, gotMetric.Data)
			}
			if len(gauge.DataPoints) != 1 {
				t.Fatalf("%s data points = %v, want one", gotMetric.Name, gauge.DataPoints)
			}
			if gauge.DataPoints[0].Value != wantValue {
				t.Fatalf("%s value = %d, want %d", gotMetric.Name, gauge.DataPoints[0].Value, wantValue)
			}
			attrs := gauge.DataPoints[0].Attributes
			if attrs.Len() != 1 {
				t.Fatalf("%s attributes = %v, want hypershell.cluster_id only", gotMetric.Name, attrs)
			}
			v, ok := attrs.Value(attribute.Key("hypershell.cluster_id"))
			if !ok || v.AsString() != clusterID {
				t.Fatalf("%s hypershell.cluster_id = %v, want %q", gotMetric.Name, v, clusterID)
			}
		}
	}
	for name := range want {
		if !found[name] {
			t.Fatalf("%s metric was not collected", name)
		}
	}
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

func TestReconcileOutcomeMetrics(t *testing.T) {
	reader := testMetricsReader(t)

	// CRM-013 scenarios
	RecordReconcileOutcome(context.Background(), "Gateway", OutcomeSuccess)
	RecordReconcileOutcome(context.Background(), "Gateway", OutcomeNoop)
	RecordReconcileOutcome(context.Background(), "Gateway", OutcomeRetryable)
	RecordReconcileOutcome(context.Background(), "Gateway", OutcomeFailed)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() returned an error: %v", err)
	}

	var found bool
	for _, scope := range collected.ScopeMetrics {
		for _, gotMetric := range scope.Metrics {
			if gotMetric.Name != "reconcile.outcomes" {
				continue
			}
			found = true
			counter, ok := gotMetric.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("metric data type = %T, want int64 sum", gotMetric.Data)
			}

			outcomes := make(map[string]int64)
			for _, point := range counter.DataPoints {
				outcome, ok := point.Attributes.Value(attribute.Key("outcome"))
				if !ok {
					t.Fatalf("missing outcome attribute on %v", point.Attributes)
				}
				outcomes[outcome.AsString()] = point.Value
			}

			want := map[string]int64{
				OutcomeSuccess:   1,
				OutcomeNoop:      1,
				OutcomeRetryable: 1,
				OutcomeFailed:    1,
			}
			for outcome, wantCount := range want {
				if got := outcomes[outcome]; got != wantCount {
					t.Errorf("outcome %q count = %d, want %d", outcome, got, wantCount)
				}
			}
		}
	}
	if !found {
		t.Fatal("reconcile.outcomes metric was not collected")
	}
}

func TestReconcileFailedWithReason(t *testing.T) {
	reader := testMetricsReader(t)

	RecordReconcileFailedWithReason(context.Background(), "Gateway", ReasonInvalidConfig)
	RecordReconcileFailedWithReason(context.Background(), "Gateway", ReasonMissingPrerequisite)
	RecordReconcileFailedWithReason(context.Background(), "Gateway", ReasonIdentityInvalid)
	RecordReconcileFailedWithReason(context.Background(), "Gateway", ReasonUnknown)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() returned an error: %v", err)
	}

	var found bool
	for _, scope := range collected.ScopeMetrics {
		for _, gotMetric := range scope.Metrics {
			if gotMetric.Name != "reconcile.failed" {
				continue
			}
			found = true
			counter, ok := gotMetric.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("metric data type = %T, want int64 sum", gotMetric.Data)
			}

			reasons := make(map[string]int64)
			for _, point := range counter.DataPoints {
				reason, ok := point.Attributes.Value(attribute.Key("reason"))
				if !ok {
					t.Fatalf("missing reason attribute on %v", point.Attributes)
				}
				reasons[reason.AsString()] = point.Value
			}

			want := map[string]int64{
				ReasonInvalidConfig:       1,
				ReasonMissingPrerequisite: 1,
				ReasonIdentityInvalid:     1,
				ReasonUnknown:             1,
			}
			for reason, wantCount := range want {
				if got := reasons[reason]; got != wantCount {
					t.Errorf("reason %q count = %d, want %d", reason, got, wantCount)
				}
			}
		}
	}
	if !found {
		t.Fatal("reconcile.failed metric was not collected")
	}
}

func TestInvalidOutcomeRejected(t *testing.T) {
	reader := testMetricsReader(t)

	RecordReconcileOutcome(context.Background(), "Gateway", "invalid-outcome")

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() returned an error: %v", err)
	}

	for _, scope := range collected.ScopeMetrics {
		for _, gotMetric := range scope.Metrics {
			if gotMetric.Name == "reconcile.outcomes" {
				t.Fatal("invalid outcome should not have recorded a metric")
			}
		}
	}
}

func TestValidateReasonFallback(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(string) string
		want     string
	}{
		{"valid retryable", ReasonK8sConflict, validateRetryableReason, ReasonK8sConflict},
		{"invalid retryable falls back", "bad-reason", validateRetryableReason, ReasonUnknown},
		{"valid failed", ReasonInvalidConfig, validateFailedReason, ReasonInvalidConfig},
		{"invalid failed falls back", "bad-reason", validateFailedReason, ReasonUnknown},
		{"unknown is valid", ReasonUnknown, validateRetryableReason, ReasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.validate(tt.input)
			if got != tt.want {
				t.Errorf("validate(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestClassifyReconcileOutcome(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		wantOutcome  string
		wantReason   string
	}{
		{"nil error is success", nil, OutcomeSuccess, ""},
		{"non-nil error is retryable", fmt.Errorf("some error"), OutcomeRetryable, ReasonUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome, reason := ClassifyReconcileOutcome(tt.err)
			if outcome != tt.wantOutcome {
				t.Errorf("outcome = %q, want %q", outcome, tt.wantOutcome)
			}
			if reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}
