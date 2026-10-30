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
	previousGatewaySandboxTotal := gatewaySandboxTotal
	previousGatewaySandboxReady := gatewaySandboxReady
	previousGatewaySandboxReadySourceAvailable := gatewaySandboxReadySourceAvailable
	previousGatewaySandboxLifetime := gatewaySandboxLifetime
	previousGatewayCertificateValid := gatewayCertificateValid
	previousGatewayCertificateExpired := gatewayCertificateExpired
	previousGatewayCertificateUnavailable := gatewayCertificateUnavailable
	previousGatewayCertificateDaysToExpiryMin := gatewayCertificateDaysToExpiryMin
	previousGatewayCertificateSourceAvailable := gatewayCertificateSourceAvailable
	previousOIDCObserved := oidcClientObservedTimestamp
	previousOIDCSuccess := oidcClientSuccessTimestamp
	previousOIDCClientSynced := oidcClientSynced
	previousOIDCClientExpected := oidcClientExpected
	previousOIDCClientKeycloakConnected := oidcClientKeycloakConnected
	previousReconcileErrors := reconcileErrors
	previousReconciliationFailures := reconciliationFailures
	previousReconciliationRetries := reconciliationRetries
	previousReconciliationLag := reconciliationLag
	previousStaleResourceStatusCount := staleResourceStatusCount
	previousReconciliationResourcesTotal := reconciliationResourcesTotal
	previousWatchReconnects := watchReconnects
	previousReconcileOutcomes := reconcileOutcomes
	previousReconcileFailed := reconcileFailed
	previousCapabilityInfo := capabilityInfo
	previousControlPlaneInfo := controlPlaneInfo

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
	gatewaySandboxTotal = nil
	gatewaySandboxReady = nil
	gatewaySandboxReadySourceAvailable = nil
	gatewaySandboxLifetime = nil
	gatewayCertificateValid = nil
	gatewayCertificateExpired = nil
	gatewayCertificateUnavailable = nil
	gatewayCertificateDaysToExpiryMin = nil
	gatewayCertificateSourceAvailable = nil
	oidcClientObservedTimestamp = nil
	oidcClientSuccessTimestamp = nil
	oidcClientSynced = nil
	oidcClientExpected = nil
	oidcClientKeycloakConnected = nil
	reconcileErrors = nil
	reconciliationFailures = nil
	reconciliationRetries = nil
	reconciliationLag = nil
	staleResourceStatusCount = nil
	reconciliationResourcesTotal = nil
	gatewayPopulations.Lock()
	gatewayPopulations.byCluster = make(map[string]map[string]bool)
	gatewayPopulations.seeded = make(map[string]bool)
	gatewayPopulations.seeding = make(map[string]bool)
	gatewayPopulations.pending = make(map[string]map[string]*bool)
	gatewayPopulations.Unlock()
	watchReconnects = nil
	reconcileOutcomes = nil
	reconcileFailed = nil
	capabilityInfo = nil
	controlPlaneInfo = nil
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
		gatewaySandboxTotal = previousGatewaySandboxTotal
		gatewaySandboxReady = previousGatewaySandboxReady
		gatewaySandboxReadySourceAvailable = previousGatewaySandboxReadySourceAvailable
		gatewaySandboxLifetime = previousGatewaySandboxLifetime
		gatewayCertificateValid = previousGatewayCertificateValid
		gatewayCertificateExpired = previousGatewayCertificateExpired
		gatewayCertificateUnavailable = previousGatewayCertificateUnavailable
		gatewayCertificateDaysToExpiryMin = previousGatewayCertificateDaysToExpiryMin
		gatewayCertificateSourceAvailable = previousGatewayCertificateSourceAvailable
		oidcClientObservedTimestamp = previousOIDCObserved
		oidcClientSuccessTimestamp = previousOIDCSuccess
		oidcClientSynced = previousOIDCClientSynced
		oidcClientExpected = previousOIDCClientExpected
		oidcClientKeycloakConnected = previousOIDCClientKeycloakConnected
		reconcileErrors = previousReconcileErrors
		reconciliationFailures = previousReconciliationFailures
		reconciliationRetries = previousReconciliationRetries
		reconciliationLag = previousReconciliationLag
		staleResourceStatusCount = previousStaleResourceStatusCount
		reconciliationResourcesTotal = previousReconciliationResourcesTotal
		watchReconnects = previousWatchReconnects
		reconcileOutcomes = previousReconcileOutcomes
		reconcileFailed = previousReconcileFailed
		capabilityInfo = previousCapabilityInfo
		controlPlaneInfo = previousControlPlaneInfo
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
	observed := int64(0)
	ReplaceGatewayPopulation(context.Background(), "cluster-a", []GatewayPopulationState{{ResourceID: "one", Generation: 1, ObservedGeneration: &observed}})
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

// Verifies: OBS-CPL-07.06
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

func TestRecordSandboxUsageMetrics(t *testing.T) {
	reader := testMetricsReader(t)

	RecordSandboxTotal(context.Background(), "cluster-a", "openshell-live", 4)
	RecordSandboxLifetime(context.Background(), "cluster-a", "openshell-live", 3*time.Hour)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() returned an error: %v", err)
	}

	var foundTotal, foundLifetime bool
	for _, scope := range collected.ScopeMetrics {
		for _, gotMetric := range scope.Metrics {
			switch gotMetric.Name {
			case "gateway.sandbox.total":
				foundTotal = true
				if gotMetric.Unit != "{sandbox}" {
					t.Fatalf("total unit = %q, want {sandbox}", gotMetric.Unit)
				}
				gauge, ok := gotMetric.Data.(metricdata.Gauge[int64])
				if !ok {
					t.Fatalf("total data type = %T, want int64 gauge", gotMetric.Data)
				}
				if len(gauge.DataPoints) != 1 || gauge.DataPoints[0].Value != 4 {
					t.Fatalf("total data points = %#v, want one point with value 4", gauge.DataPoints)
				}
				assertSandboxUsageAttributes(t, "gateway.sandbox.total", gauge.DataPoints[0].Attributes)
			case "gateway.sandbox.lifetime":
				foundLifetime = true
				if gotMetric.Unit != "s" {
					t.Fatalf("lifetime unit = %q, want s", gotMetric.Unit)
				}
				histogram, ok := gotMetric.Data.(metricdata.Histogram[float64])
				if !ok {
					t.Fatalf("lifetime data type = %T, want float64 histogram", gotMetric.Data)
				}
				if len(histogram.DataPoints) != 1 {
					t.Fatalf("lifetime data points = %#v, want one", histogram.DataPoints)
				}
				wantBounds := []float64{60, 300, 900, 1800, 3600, 7200, 14400, 28800, 43200, 86400, 172800, 259200, 604800, 1209600, 2592000}
				if !reflect.DeepEqual(histogram.DataPoints[0].Bounds, wantBounds) {
					t.Fatalf("lifetime bounds = %v, want %v", histogram.DataPoints[0].Bounds, wantBounds)
				}
				assertSandboxUsageAttributes(t, "gateway.sandbox.lifetime", histogram.DataPoints[0].Attributes)
			}
		}
	}
	if !foundTotal {
		t.Fatal("gateway.sandbox.total metric was not collected")
	}
	if !foundLifetime {
		t.Fatal("gateway.sandbox.lifetime metric was not collected")
	}
}

func TestRecordDashboardHealthGauges(t *testing.T) {
	reader := testMetricsReader(t)
	minimum := 4.5
	RecordSandboxReady(t.Context(), "cluster-a", 3)
	RecordCertificateHealth(t.Context(), "cluster-a", 7, 1, 2, &minimum)
	RecordOIDCClientHealth(t.Context(), "cluster-a", 4, 5, true)

	wantInt := map[string]int64{
		"gateway.sandbox.ready":                  3,
		"gateway.sandbox.ready.source.available": 1,
		"gateway.certificate.valid":              7,
		"gateway.certificate.expired":            1,
		"gateway.certificate.unavailable":        2,
		"gateway.certificate.source.available":   1,
		"oidc.client.synced":                     4,
		"oidc.client.expected":                   5,
		"oidc.client.keycloak_connected":         1,
	}
	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatal(err)
	}
	for _, scope := range collected.ScopeMetrics {
		for _, item := range scope.Metrics {
			if expected, ok := wantInt[item.Name]; ok {
				gauge, ok := item.Data.(metricdata.Gauge[int64])
				if !ok {
					t.Fatalf("%s type = %T", item.Name, item.Data)
				}
				if len(gauge.DataPoints) != 1 || gauge.DataPoints[0].Value != expected {
					t.Fatalf("%s = %#v, want %d", item.Name, gauge.DataPoints, expected)
				}
				delete(wantInt, item.Name)
			}
			if item.Name == "gateway.certificate.days_to_expiry_min" {
				gauge := item.Data.(metricdata.Gauge[float64])
				if len(gauge.DataPoints) != 1 || gauge.DataPoints[0].Value != minimum {
					t.Fatalf("expiry minimum = %#v", gauge.DataPoints)
				}
			}
		}
	}
	if len(wantInt) != 0 {
		t.Fatalf("missing metrics: %v", wantInt)
	}
}

func TestDashboardHealthSourceAvailabilityClearsSuccessfulObservation(t *testing.T) {
	reader := testMetricsReader(t)
	minimum := 4.5
	RecordSandboxReady(t.Context(), "cluster-a", 3)
	RecordCertificateHealth(t.Context(), "cluster-a", 7, 0, 0, &minimum)
	RecordSandboxReadyUnavailable(t.Context(), "cluster-a")
	RecordCertificateHealthUnavailable(t.Context(), "cluster-a")

	want := map[string]int64{
		"gateway.sandbox.ready.source.available": 0,
		"gateway.certificate.source.available":   0,
	}
	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatal(err)
	}
	for _, scope := range collected.ScopeMetrics {
		for _, item := range scope.Metrics {
			expected, ok := want[item.Name]
			if !ok {
				continue
			}
			gauge, ok := item.Data.(metricdata.Gauge[int64])
			if !ok || len(gauge.DataPoints) != 1 || gauge.DataPoints[0].Value != expected {
				t.Fatalf("%s = %#v, want %d", item.Name, item.Data, expected)
			}
			delete(want, item.Name)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing availability metrics: %v", want)
	}
}

func TestGatewayPopulationSnapshotUpdateAndDelete(t *testing.T) {
	reader := testMetricsReader(t)
	observedOne := int64(1)
	observedTwo := int64(2)
	ReplaceGatewayPopulation(t.Context(), "cluster-a", []GatewayPopulationState{
		{ResourceID: "stale", Generation: 2, ObservedGeneration: &observedOne},
		{ResourceID: "current", Generation: 2, ObservedGeneration: &observedTwo},
	})
	UpdateGatewayPopulation(t.Context(), "cluster-a", GatewayPopulationState{ResourceID: "current", Generation: 2, ObservedGeneration: &observedTwo}, true)

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"hypershell.stale.resource.status.count": 1, "hypershell.reconciliation.resources.total": 1}
	for _, scope := range collected.ScopeMetrics {
		for _, item := range scope.Metrics {
			expected, ok := want[item.Name]
			if !ok {
				continue
			}
			gauge := item.Data.(metricdata.Gauge[int64])
			var matched bool
			for _, point := range gauge.DataPoints {
				cluster, _ := point.Attributes.Value(attribute.Key("hypershell.cluster_id"))
				if cluster.AsString() != "cluster-a" {
					continue
				}
				matched = true
				if point.Value != expected {
					t.Fatalf("%s = %d, want %d", item.Name, point.Value, expected)
				}
			}
			if !matched {
				t.Fatalf("%s has no cluster-a point: %#v", item.Name, gauge.DataPoints)
			}
			delete(want, item.Name)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing population metrics: %v", want)
	}
}

func TestGatewayPopulationSeedPreservesConcurrentWatchEvents(t *testing.T) {
	_ = testMetricsReader(t)
	observedCurrent := int64(2)
	observedStale := int64(1)

	BeginGatewayPopulationSeed("cluster-a")
	UpdateGatewayPopulation(t.Context(), "cluster-a", GatewayPopulationState{
		ResourceID: "deleted-during-list", Generation: 2, ObservedGeneration: &observedCurrent,
	}, true)
	UpdateGatewayPopulation(t.Context(), "cluster-a", GatewayPopulationState{
		ResourceID: "created-during-list", Generation: 2, ObservedGeneration: &observedCurrent,
	}, false)
	ReplaceGatewayPopulation(t.Context(), "cluster-a", []GatewayPopulationState{
		{ResourceID: "deleted-during-list", Generation: 2, ObservedGeneration: &observedCurrent},
		{ResourceID: "stale", Generation: 2, ObservedGeneration: &observedStale},
	})

	gatewayPopulations.Lock()
	defer gatewayPopulations.Unlock()
	population := gatewayPopulations.byCluster["cluster-a"]
	if _, exists := population["deleted-during-list"]; exists {
		t.Fatal("seed snapshot resurrected a gateway deleted by the concurrent watch")
	}
	if stale, exists := population["created-during-list"]; !exists || stale {
		t.Fatalf("concurrent create = (%v, %v), want present and current", exists, stale)
	}
	if stale, exists := population["stale"]; !exists || !stale {
		t.Fatalf("snapshot stale gateway = (%v, %v), want present and stale", exists, stale)
	}
}

func assertSandboxUsageAttributes(t *testing.T, metricName string, attrs attribute.Set) {
	t.Helper()
	if attrs.Len() != 2 {
		t.Fatalf("%s attributes = %v, want namespace and hypershell.cluster_id", metricName, attrs)
	}
	namespace, ok := attrs.Value(attribute.Key("namespace"))
	if !ok || namespace.AsString() != "openshell-live" {
		t.Fatalf("%s namespace = %v, want openshell-live", metricName, namespace)
	}
	clusterID, ok := attrs.Value(attribute.Key("hypershell.cluster_id"))
	if !ok || clusterID.AsString() != "cluster-a" {
		t.Fatalf("%s hypershell.cluster_id = %v, want cluster-a", metricName, clusterID)
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

	// OBS-CRM-13 scenarios
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
		name        string
		err         error
		wantOutcome string
		wantReason  string
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

// Verifies: OBS-CPL-09.01
func TestRegisterClusterIdentityInfo(t *testing.T) {
	reader := testMetricsReader(t)

	unregister, err := RegisterClusterIdentityInfo(
		"hyp2-spoke1",
		[]string{"provider/ibm", "network/public"},
		"quay.io/opendatahub/odh-openshell-gateway:v0.1.5-rhaiv.2",
	)
	if err != nil {
		t.Fatalf("RegisterClusterIdentityInfo() returned an error: %v", err)
	}
	t.Cleanup(func() { _ = unregister() })

	capabilities := map[string][2]string{}
	var image string
	for _, point := range collectInt64Gauge(t, reader, "hypershell.managed_cluster.capability.info") {
		if point.Value != 1 {
			t.Fatalf("capability info value = %d, want 1", point.Value)
		}
		assertAttribute(t, point.Attributes, "managed_cluster", "hyp2-spoke1")
		capability := attributeString(t, point.Attributes, "capability")
		capabilities[capability] = [2]string{
			attributeString(t, point.Attributes, "capability_namespace"),
			attributeString(t, point.Attributes, "capability_value"),
		}
	}
	want := map[string][2]string{
		"provider/ibm":   {"provider", "ibm"},
		"network/public": {"network", "public"},
	}
	if !reflect.DeepEqual(capabilities, want) {
		t.Fatalf("capability series = %v, want %v", capabilities, want)
	}

	for _, point := range collectInt64Gauge(t, reader, "hypershell.control_plane.info") {
		if point.Value != 1 {
			t.Fatalf("control plane info value = %d, want 1", point.Value)
		}
		assertAttribute(t, point.Attributes, "managed_cluster", "hyp2-spoke1")
		image = attributeString(t, point.Attributes, "gateway_image")
	}
	if image != "quay.io/opendatahub/odh-openshell-gateway:v0.1.5-rhaiv.2" {
		t.Fatalf("gateway_image = %q", image)
	}
}

// Verifies: OBS-CPL-09.02
func TestRegisterClusterIdentityInfoWithoutClusterName(t *testing.T) {
	reader := testMetricsReader(t)

	unregister, err := RegisterClusterIdentityInfo("", []string{"provider/ibm"}, "image:tag")
	if err != nil {
		t.Fatalf("RegisterClusterIdentityInfo() returned an error: %v", err)
	}
	t.Cleanup(func() { _ = unregister() })

	if points := collectInt64Gauge(t, reader, "hypershell.managed_cluster.capability.info"); len(points) != 0 {
		t.Fatalf("capability info points = %d, want 0 without a cluster name", len(points))
	}
}

func collectInt64Gauge(t *testing.T, reader *sdkmetric.ManualReader, name string) []metricdata.DataPoint[int64] {
	t.Helper()
	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() returned an error: %v", err)
	}
	for _, scope := range collected.ScopeMetrics {
		for _, got := range scope.Metrics {
			if got.Name != name {
				continue
			}
			gauge, ok := got.Data.(metricdata.Gauge[int64])
			if !ok {
				t.Fatalf("%s data type = %T, want int64 gauge", name, got.Data)
			}
			return gauge.DataPoints
		}
	}
	return nil
}

func attributeString(t *testing.T, set attribute.Set, key string) string {
	t.Helper()
	v, ok := set.Value(attribute.Key(key))
	if !ok {
		t.Fatalf("missing attribute %q on %v", key, set)
	}
	return v.AsString()
}

func assertAttribute(t *testing.T, set attribute.Set, key, want string) {
	t.Helper()
	if got := attributeString(t, set, key); got != want {
		t.Fatalf("attribute %q = %q, want %q", key, got, want)
	}
}

func TestOIDCHealthFreshnessUsesProducerTimes(t *testing.T) {
	reader := testMetricsReader(t)
	observed := time.Unix(1000, 0)
	RecordOIDCClientFreshness(t.Context(), "cluster-a", observed, time.Time{})
	assertTimes := func(wantObserved, wantSuccess int64) {
		t.Helper()
		var collected metricdata.ResourceMetrics
		if err := reader.Collect(t.Context(), &collected); err != nil {
			t.Fatal(err)
		}
		want := map[string]int64{"oidc.client.health.observed_timestamp_seconds": wantObserved, "oidc.client.health.success_timestamp_seconds": wantSuccess}
		for _, scope := range collected.ScopeMetrics {
			for _, item := range scope.Metrics {
				if value, ok := want[item.Name]; ok {
					points := item.Data.(metricdata.Gauge[int64]).DataPoints
					if len(points) != 1 || points[0].Value != value {
						t.Fatalf("%s = %v, want %d", item.Name, points, value)
					}
					delete(want, item.Name)
				}
			}
		}
		if len(want) != 0 {
			t.Fatalf("missing metrics: %v", want)
		}
	}
	assertTimes(1000, 0)
	RecordOIDCClientFreshness(t.Context(), "cluster-a", observed.Add(time.Minute), observed)
	assertTimes(1060, 1000)
	// Re-exporting without another observation cannot renew freshness.
	assertTimes(1060, 1000)
}
