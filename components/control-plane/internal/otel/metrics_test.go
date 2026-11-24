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
	previousOldestUnconvergedGenerationAge := oldestUnconvergedGenerationAge
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
	oldestUnconvergedGenerationAge = nil
	staleResourceStatusCount = nil
	reconciliationResourcesTotal = nil
	gatewayPopulations.Lock()
	gatewayPopulations.byCluster = make(map[string]map[string]bool)
	gatewayPopulations.unconvergedSince = make(map[string]map[string]time.Time)
	gatewayPopulations.seeded = make(map[string]bool)
	gatewayPopulations.seeding = make(map[string]bool)
	gatewayPopulations.pending = make(map[string]map[string]*bool)
	gatewayPopulations.Unlock()
	reconciliationLagStamps.Lock()
	reconciliationLagStamps.byKey = make(map[string]*generationStamp)
	reconciliationLagStamps.Unlock()
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
		oldestUnconvergedGenerationAge = previousOldestUnconvergedGenerationAge
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
	NoteGenerationObserved("Gateway", "lag-one", 1)
	RecordReconciliationLagForStart(context.Background(), "Gateway", "lag-one", 1)
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

func TestRecordManagedClusterHealth(t *testing.T) {
	// OBS-CPL-12.01: the overall rollup and per-check conditions export as bounded
	// gauges labeled only by managed_cluster (and, per check, the bounded check
	// name); no raw identifier or message is a label.
	reader := testMetricsReader(t)
	RecordManagedClusterHealth(t.Context(), "mc1", 0, map[string]int64{
		"oidc-issuer-reachable": 0,
		"images-pullable":       1,
	})

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatal(err)
	}

	var sawOverall bool
	checkStates := map[string]int64{}
	for _, scope := range collected.ScopeMetrics {
		for _, item := range scope.Metrics {
			switch item.Name {
			case "hypershell.managed_cluster.health":
				gauge := item.Data.(metricdata.Gauge[int64])
				if len(gauge.DataPoints) != 1 {
					t.Fatalf("overall gauge points = %d, want 1", len(gauge.DataPoints))
				}
				dp := gauge.DataPoints[0]
				if dp.Value != 0 {
					t.Fatalf("overall = %d, want 0 (failing)", dp.Value)
				}
				if dp.Attributes.Len() != 1 {
					t.Fatalf("overall attributes = %v, want only managed_cluster", dp.Attributes)
				}
				assertAttribute(t, dp.Attributes, "managed_cluster", "mc1")
				sawOverall = true
			case "hypershell.managed_cluster.health.check":
				gauge := item.Data.(metricdata.Gauge[int64])
				for _, dp := range gauge.DataPoints {
					if dp.Attributes.Len() != 2 {
						t.Fatalf("per-check attributes = %v, want managed_cluster and check", dp.Attributes)
					}
					assertAttribute(t, dp.Attributes, "managed_cluster", "mc1")
					checkStates[attributeString(t, dp.Attributes, "check")] = dp.Value
				}
			}
		}
	}
	if !sawOverall {
		t.Fatalf("overall health gauge not exported")
	}
	if checkStates["oidc-issuer-reachable"] != 0 || checkStates["images-pullable"] != 1 {
		t.Fatalf("per-check states = %v", checkStates)
	}
}

func TestRecordManagedClusterHealthSkipsEmptyClusterName(t *testing.T) {
	// OBS-CPL-12.02: no gauges when the managed-cluster name is empty.
	reader := testMetricsReader(t)
	RecordManagedClusterHealth(t.Context(), "", 2, map[string]int64{"images-pullable": 1})

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &collected); err != nil {
		t.Fatal(err)
	}
	for _, scope := range collected.ScopeMetrics {
		for _, item := range scope.Metrics {
			if item.Name == "hypershell.managed_cluster.health" || item.Name == "hypershell.managed_cluster.health.check" {
				t.Fatalf("health gauge %q exported for an empty cluster name", item.Name)
			}
		}
	}
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
