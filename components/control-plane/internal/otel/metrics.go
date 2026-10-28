package otel

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	reconcileDuration                  metric.Int64Histogram
	reconcileQueueDepth                metric.Int64ObservableGauge
	reconcileQueueWaitDuration         metric.Float64Histogram
	gatewayProvisionDuration           metric.Float64Histogram
	gatewayProvisionOutcomes           metric.Int64Counter
	gatewaySandboxOrphaned             metric.Int64Gauge
	gatewaySandboxExpiring             metric.Int64Gauge
	gatewaySandboxIdle                 metric.Int64Gauge
	gatewayVaultUnreachable            metric.Int64Counter
	gatewaySandboxTotal                metric.Int64Gauge
	gatewaySandboxReady                metric.Int64Gauge
	gatewaySandboxReadySourceAvailable metric.Int64Gauge
	gatewaySandboxLifetime             metric.Float64Histogram
	gatewayCertificateValid            metric.Int64Gauge
	gatewayCertificateExpired          metric.Int64Gauge
	gatewayCertificateUnavailable      metric.Int64Gauge
	gatewayCertificateDaysToExpiryMin  metric.Float64Gauge
	gatewayCertificateSourceAvailable  metric.Int64Gauge
	oidcClientSynced                   metric.Int64Gauge
	oidcClientExpected                 metric.Int64Gauge
	oidcClientKeycloakConnected        metric.Int64Gauge
	reconcileErrors                    metric.Int64Counter
	reconciliationFailures             metric.Int64Counter
	reconciliationRetries              metric.Int64Counter
	reconciliationLag                  metric.Float64Histogram
	staleResourceStatusCount           metric.Int64Gauge
	reconciliationResourcesTotal       metric.Int64Gauge
	watchReconnects                    metric.Int64Counter
	reconcileOutcomes                  metric.Int64Counter
	reconcileFailed                    metric.Int64Counter
	capabilityInfo                     metric.Int64ObservableGauge
	controlPlaneInfo                   metric.Int64ObservableGauge
)

// Reconciliation outcome taxonomy (OBS-CRM-06). These are the single source of
// truth for all reconciliation outcome metrics.
const (
	OutcomeNoop      = "noop"
	OutcomeSuccess   = "success"
	OutcomeRetryable = "retryable"
	OutcomeFailed    = "failed"
)

// Retryable reason codes (OBS-CRM-07).
const (
	ReasonGRPCUnavailable    = "grpc_unavailable"
	ReasonK8sConflict        = "k8s_conflict"
	ReasonK8sUnavailable     = "k8s_unavailable"
	ReasonDependencyNotReady = "dependency_not_ready"
	ReasonKeycloakTransient  = "keycloak_transient"
)

// Failed reason codes (OBS-CRM-07).
const (
	ReasonInvalidConfig       = "invalid_config"
	ReasonMissingPrerequisite = "missing_prerequisite"
	ReasonIdentityInvalid     = "identity_invalid"
)

// ReasonUnknown is the fallback when a failure doesn't match a recognized code.
const ReasonUnknown = "unknown"

type GatewayPopulationState struct {
	ResourceID         string
	Generation         int64
	ObservedGeneration *int64
}

var gatewayPopulations = struct {
	sync.Mutex
	byCluster map[string]map[string]bool
	seeded    map[string]bool
	seeding   map[string]bool
	pending   map[string]map[string]*bool
}{
	byCluster: make(map[string]map[string]bool),
	seeded:    make(map[string]bool),
	seeding:   make(map[string]bool),
	pending:   make(map[string]map[string]*bool),
}

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

	gatewayVaultUnreachable, err = meter.Int64Counter(
		"gateway.vault.unreachable",
		metric.WithUnit("{observation}"),
		metric.WithDescription("Count of health observations where a vault-backed gateway's provider Vault was unreachable"),
	)
	if err != nil {
		return err
	}

	gatewaySandboxTotal, err = meter.Int64Gauge(
		"gateway.sandbox.total",
		metric.WithUnit("{sandbox}"),
		metric.WithDescription("Total provisioned Sandbox custom resources"),
	)
	if err != nil {
		return err
	}
	gatewaySandboxReady, err = meter.Int64Gauge(
		"gateway.sandbox.ready",
		metric.WithUnit("{sandbox}"),
		metric.WithDescription("Provisioned Sandbox custom resources whose Kubernetes Ready condition is true"),
	)
	if err != nil {
		return err
	}
	gatewaySandboxReadySourceAvailable, err = meter.Int64Gauge(
		"gateway.sandbox.ready.source.available",
		metric.WithDescription("Whether the sandbox readiness source completed successfully"),
	)
	if err != nil {
		return err
	}

	gatewayCertificateValid, err = meter.Int64Gauge("gateway.certificate.valid", metric.WithUnit("{certificate}"), metric.WithDescription("Evaluated gateway and console TLS certificates that are currently valid"))
	if err != nil {
		return err
	}
	gatewayCertificateExpired, err = meter.Int64Gauge("gateway.certificate.expired", metric.WithUnit("{certificate}"), metric.WithDescription("Evaluated gateway and console TLS certificates that are expired"))
	if err != nil {
		return err
	}
	gatewayCertificateUnavailable, err = meter.Int64Gauge("gateway.certificate.unavailable", metric.WithUnit("{certificate}"), metric.WithDescription("Gateway and console TLS certificates whose status is unavailable"))
	if err != nil {
		return err
	}
	gatewayCertificateDaysToExpiryMin, err = meter.Float64Gauge("gateway.certificate.days_to_expiry_min", metric.WithUnit("d"), metric.WithDescription("Minimum days to expiry among valid evaluated gateway and console certificates"))
	if err != nil {
		return err
	}
	gatewayCertificateSourceAvailable, err = meter.Int64Gauge("gateway.certificate.source.available", metric.WithDescription("Whether the gateway and console certificate source completed successfully"))
	if err != nil {
		return err
	}

	oidcClientSynced, err = meter.Int64Gauge("oidc.client.synced", metric.WithUnit("{client}"), metric.WithDescription("Desired gateway OIDC clients present in Keycloak"))
	if err != nil {
		return err
	}
	oidcClientExpected, err = meter.Int64Gauge("oidc.client.expected", metric.WithUnit("{client}"), metric.WithDescription("Desired gateway OIDC client count"))
	if err != nil {
		return err
	}
	oidcClientKeycloakConnected, err = meter.Int64Gauge("oidc.client.keycloak_connected", metric.WithDescription("Whether the Keycloak client comparison completed"))
	if err != nil {
		return err
	}

	gatewaySandboxLifetime, err = meter.Float64Histogram(
		"gateway.sandbox.lifetime",
		metric.WithUnit("s"),
		metric.WithDescription("Lifetime of a sandbox when it leaves the active set"),
		metric.WithExplicitBucketBoundaries(
			60, 300, 900, 1800, 3600, 7200, 14400, 28800, 43200,
			86400, 172800, 259200, 604800, 1209600, 2592000,
		),
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
	reconciliationResourcesTotal, err = meter.Int64Gauge(
		"hypershell.reconciliation.resources.total",
		metric.WithUnit("{resource}"),
		metric.WithDescription("Non-deleted Gateways in the control-plane watch population"),
	)
	if err != nil {
		return err
	}

	watchReconnects, err = meter.Int64Counter(
		"watch.reconnects",
		metric.WithUnit("{reconnect}"),
		metric.WithDescription("Count of watch stream reconnections"),
	)
	if err != nil {
		return err
	}

	reconcileOutcomes, err = meter.Int64Counter(
		"reconcile.outcomes",
		metric.WithUnit("{outcome}"),
		metric.WithDescription("Count of reconciliation attempts by outcome"),
	)
	if err != nil {
		return err
	}

	reconcileFailed, err = meter.Int64Counter(
		"reconcile.failed",
		metric.WithUnit("{failure}"),
		metric.WithDescription("Count of reconciliation attempts that reached a non-recoverable state"),
	)
	if err != nil {
		return err
	}

	// The info gauges carry no unit: an annotation unit adds nothing, and the
	// Prometheus exporter turns the unit "1" into a _ratio suffix.
	capabilityInfo, err = meter.Int64ObservableGauge(
		"hypershell.managed_cluster.capability.info",
		metric.WithDescription("Self-declared capability of this managed cluster; one series per capability, always 1"),
	)
	if err != nil {
		return err
	}

	controlPlaneInfo, err = meter.Int64ObservableGauge(
		"hypershell.control_plane.info",
		metric.WithDescription("Default gateway image this control plane provisions; always 1"),
	)
	return err
}

// RegisterClusterIdentityInfo exports this control plane's managed-cluster name,
// its configured capability set, and its default gateway image as always-1 info
// gauges (OBS-CPL-09). The values come from deployment configuration, so a
// GitOps override of the capability list cannot hide the image the control plane
// actually deploys. The returned function removes the callback.
func RegisterClusterIdentityInfo(clusterName string, capabilities []string, gatewayImage string) (func() error, error) {
	if capabilityInfo == nil || controlPlaneInfo == nil || clusterName == "" {
		return func() error { return nil }, nil
	}
	caps := append([]string(nil), capabilities...)
	registration, err := otel.Meter(TracerName).RegisterCallback(
		func(_ context.Context, observer metric.Observer) error {
			for _, capability := range caps {
				namespace, value, _ := strings.Cut(capability, "/")
				observer.ObserveInt64(capabilityInfo, 1, metric.WithAttributes(
					attribute.String("managed_cluster", clusterName),
					attribute.String("capability", capability),
					attribute.String("capability_namespace", namespace),
					attribute.String("capability_value", value),
				))
			}
			observer.ObserveInt64(controlPlaneInfo, 1, metric.WithAttributes(
				attribute.String("managed_cluster", clusterName),
				attribute.String("gateway_image", gatewayImage),
			))
			return nil
		},
		capabilityInfo,
		controlPlaneInfo,
	)
	if err != nil {
		return nil, fmt.Errorf("register cluster identity info callback: %w", err)
	}
	return registration.Unregister, nil
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

// RecordReconcileDuration records the duration of a reconcile operation with
// an outcome label (OBS-CRM-08). The outcome must be one of the constants from
// OBS-CRM-06; unrecognized values are rejected and logged.
func RecordReconcileDuration(ctx context.Context, kind, eventType, outcome string, start time.Time) {
	if reconcileDuration == nil {
		return
	}
	if !isValidOutcome(outcome) {
		log.Printf("WARN invalid reconcile outcome %q for duration metric (kind=%s); metric not recorded", outcome, kind)
		return
	}
	reconcileDuration.Record(ctx, time.Since(start).Milliseconds(), metric.WithAttributes(
		attribute.String("resource.kind", kind),
		attribute.String("event.type", eventType),
		attribute.String("outcome", outcome),
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
// the most recent sandbox attention reconcile tick (OBS-SBXA-06). clusterID labels
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

// RecordSandboxTotal exports the total provisioned Sandbox count for one
// namespace. The gauge is cluster-local and includes inactive sandboxes.
func RecordSandboxTotal(ctx context.Context, clusterID, namespace string, count int) {
	if gatewaySandboxTotal == nil {
		return
	}
	gatewaySandboxTotal.Record(ctx, int64(max(0, count)), metric.WithAttributes(
		attribute.String("namespace", namespace),
		attribute.String("hypershell.cluster_id", AttentionClusterID(clusterID)),
	))
}

// RecordSandboxReady exports the cluster-local count of provisioned Sandboxes
// whose observed Kubernetes workload is Ready.
func RecordSandboxReady(ctx context.Context, clusterID string, count int) {
	if gatewaySandboxReady == nil {
		return
	}
	gatewaySandboxReady.Record(ctx, int64(max(0, count)), clusterMetricAttributes(clusterID))
	if gatewaySandboxReadySourceAvailable != nil {
		gatewaySandboxReadySourceAvailable.Record(ctx, 1, clusterMetricAttributes(clusterID))
	}
}

// RecordSandboxReadyUnavailable marks the cluster-local readiness observation
// unavailable so an earlier successful gauge cannot be presented as current.
func RecordSandboxReadyUnavailable(ctx context.Context, clusterID string) {
	attrs := clusterMetricAttributes(clusterID)
	if gatewaySandboxReadySourceAvailable != nil {
		gatewaySandboxReadySourceAvailable.Record(ctx, 0, attrs)
	}
}

// RecordCertificateHealth exports one complete cluster-local certificate
// observation. A missing minimum is recorded as NaN, clearing the prior
// last-value sample without inventing a finite expiry when no certificate is
// currently valid.
func RecordCertificateHealth(ctx context.Context, clusterID string, valid, expired, unavailable int, daysToExpiryMin *float64) {
	attrs := clusterMetricAttributes(clusterID)
	if gatewayCertificateSourceAvailable != nil {
		gatewayCertificateSourceAvailable.Record(ctx, 1, attrs)
	}
	if gatewayCertificateValid != nil {
		gatewayCertificateValid.Record(ctx, int64(max(0, valid)), attrs)
	}
	if gatewayCertificateExpired != nil {
		gatewayCertificateExpired.Record(ctx, int64(max(0, expired)), attrs)
	}
	if gatewayCertificateUnavailable != nil {
		gatewayCertificateUnavailable.Record(ctx, int64(max(0, unavailable)), attrs)
	}
	if gatewayCertificateDaysToExpiryMin != nil {
		value := math.NaN()
		if daysToExpiryMin != nil {
			value = *daysToExpiryMin
		}
		gatewayCertificateDaysToExpiryMin.Record(ctx, value, attrs)
	}
}

// RecordCertificateHealthUnavailable marks the cluster-local certificate
// observation unavailable so earlier valid counts cannot remain authoritative.
func RecordCertificateHealthUnavailable(ctx context.Context, clusterID string) {
	attrs := clusterMetricAttributes(clusterID)
	if gatewayCertificateSourceAvailable != nil {
		gatewayCertificateSourceAvailable.Record(ctx, 0, attrs)
	}
}

// RecordOIDCClientHealth exports a cluster-local comparison of the desired
// gateway client set with Keycloak. connected is false when the comparison did
// not complete, keeping an empty or failed denominator distinct from 100%.
func RecordOIDCClientHealth(ctx context.Context, clusterID string, synced, expected int, connected bool) {
	attrs := clusterMetricAttributes(clusterID)
	if oidcClientSynced != nil {
		oidcClientSynced.Record(ctx, int64(max(0, synced)), attrs)
	}
	if oidcClientExpected != nil {
		oidcClientExpected.Record(ctx, int64(max(0, expected)), attrs)
	}
	if oidcClientKeycloakConnected != nil {
		value := int64(0)
		if connected {
			value = 1
		}
		oidcClientKeycloakConnected.Record(ctx, value, attrs)
	}
}

func clusterMetricAttributes(clusterID string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String("hypershell.cluster_id", AttentionClusterID(clusterID)))
}

// RecordSandboxLifetime exports one observed sandbox lifetime in seconds.
func RecordSandboxLifetime(ctx context.Context, clusterID, namespace string, lifetime time.Duration) {
	if gatewaySandboxLifetime == nil || lifetime < 0 {
		return
	}
	gatewaySandboxLifetime.Record(ctx, lifetime.Seconds(), metric.WithAttributes(
		attribute.String("namespace", namespace),
		attribute.String("hypershell.cluster_id", AttentionClusterID(clusterID)),
	))
}

// RecordGatewayVaultUnreachable counts a health observation where a vault-backed
// gateway's provider Vault was unreachable (the dashboard-visible signal that lets an
// operator distinguish Vault-caused Degraded gateways from other causes). Cluster-level
// only; the gateway identifier is intentionally not an attribute.
func RecordGatewayVaultUnreachable(ctx context.Context, clusterID string) {
	if gatewayVaultUnreachable == nil {
		return
	}
	gatewayVaultUnreachable.Add(ctx, 1, metric.WithAttributes(
		attribute.String("hypershell.cluster_id", clusterID),
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
	if reconciliationFailures != nil {
		reconciliationFailures.Add(ctx, 1, metric.WithAttributes(
			attribute.String("resource.kind", kind),
		))
	}
}

// RecordReconciliationRetry records a retry scheduled for a reconciliation with
// a reason code (OBS-CRM-10). The reason must be a retryable code from OBS-CRM-07 or
// ReasonUnknown; unrecognized values fall back to ReasonUnknown and are logged.
func RecordReconciliationRetry(ctx context.Context, kind, reason string) {
	if reconciliationRetries == nil {
		return
	}
	validatedReason := validateRetryableReason(reason)
	if validatedReason != reason {
		log.Printf("WARN unrecognized retryable reason %q for kind %s; using %q", reason, kind, validatedReason)
	}
	reconciliationRetries.Add(ctx, 1, metric.WithAttributes(
		attribute.String("resource.kind", kind),
		attribute.String("reason", validatedReason),
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
	if staleResourceStatusCount == nil || kind != "Gateway" || resourceID == "" {
		return
	}
	cluster := AttentionClusterID(clusterID)
	gatewayPopulations.Lock()
	population := gatewayPopulations.byCluster[cluster]
	if population == nil {
		population = make(map[string]bool)
		gatewayPopulations.byCluster[cluster] = population
	}
	population[resourceID] = stale
	if gatewayPopulations.seeded[cluster] {
		recordGatewayPopulationLocked(ctx, cluster, population)
	}
	gatewayPopulations.Unlock()
}

// BeginGatewayPopulationSeed starts an authoritative seed transaction. Watch
// events received while the LIST is in flight are retained and overlaid on the
// eventual snapshot so the snapshot cannot roll the population backward.
func BeginGatewayPopulationSeed(clusterID string) {
	cluster := AttentionClusterID(clusterID)
	gatewayPopulations.Lock()
	gatewayPopulations.seeding[cluster] = true
	gatewayPopulations.pending[cluster] = make(map[string]*bool)
	gatewayPopulations.Unlock()
}

// CancelGatewayPopulationSeed ends a failed seed transaction. Live events were
// already applied to the current population by UpdateGatewayPopulation, so only
// the pending overlay needs to be discarded.
func CancelGatewayPopulationSeed(clusterID string) {
	cluster := AttentionClusterID(clusterID)
	gatewayPopulations.Lock()
	gatewayPopulations.seeding[cluster] = false
	delete(gatewayPopulations.pending, cluster)
	gatewayPopulations.Unlock()
}

// ReplaceGatewayPopulation atomically publishes the authoritative Gateway seed
// after a successful watch (re)connect. Events received after the seed began
// are overlaid before publication, so a LIST snapshot cannot overwrite a newer
// update or resurrect a resource deleted by the live watch.
func ReplaceGatewayPopulation(ctx context.Context, clusterID string, states []GatewayPopulationState) {
	cluster := AttentionClusterID(clusterID)
	population := make(map[string]bool, len(states))
	for _, state := range states {
		if state.ResourceID == "" {
			continue
		}
		observed := int64(0)
		if state.ObservedGeneration != nil {
			observed = *state.ObservedGeneration
		}
		population[state.ResourceID] = observed < state.Generation
	}
	gatewayPopulations.Lock()
	for resourceID, stale := range gatewayPopulations.pending[cluster] {
		if stale == nil {
			delete(population, resourceID)
			continue
		}
		population[resourceID] = *stale
	}
	gatewayPopulations.byCluster[cluster] = population
	gatewayPopulations.seeded[cluster] = true
	gatewayPopulations.seeding[cluster] = false
	delete(gatewayPopulations.pending, cluster)
	recordGatewayPopulationLocked(ctx, cluster, population)
	gatewayPopulations.Unlock()
}

// UpdateGatewayPopulation applies one post-seed watch event to both the total
// and stale subsets. Deletes remove the resource from the shared population.
func UpdateGatewayPopulation(ctx context.Context, clusterID string, state GatewayPopulationState, deleted bool) {
	if state.ResourceID == "" {
		return
	}
	cluster := AttentionClusterID(clusterID)
	gatewayPopulations.Lock()
	population := gatewayPopulations.byCluster[cluster]
	if population == nil {
		population = make(map[string]bool)
		gatewayPopulations.byCluster[cluster] = population
	}
	if deleted {
		delete(population, state.ResourceID)
	} else {
		observed := int64(0)
		if state.ObservedGeneration != nil {
			observed = *state.ObservedGeneration
		}
		population[state.ResourceID] = observed < state.Generation
	}
	if gatewayPopulations.seeding[cluster] {
		pending := gatewayPopulations.pending[cluster]
		if pending == nil {
			pending = make(map[string]*bool)
			gatewayPopulations.pending[cluster] = pending
		}
		if deleted {
			pending[state.ResourceID] = nil
		} else {
			value := population[state.ResourceID]
			pending[state.ResourceID] = &value
		}
	}
	if gatewayPopulations.seeded[cluster] {
		recordGatewayPopulationLocked(ctx, cluster, population)
	}
	gatewayPopulations.Unlock()
}

func recordGatewayPopulationLocked(ctx context.Context, cluster string, population map[string]bool) {
	attrs := metric.WithAttributes(attribute.String("hypershell.cluster_id", cluster))
	stale := int64(0)
	for _, isStale := range population {
		if isStale {
			stale++
		}
	}
	if staleResourceStatusCount != nil {
		staleResourceStatusCount.Record(ctx, stale, attrs)
	}
	if reconciliationResourcesTotal != nil {
		reconciliationResourcesTotal.Record(ctx, int64(len(population)), attrs)
	}
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

// RecordReconcileOutcome increments the reconcile outcomes counter with the
// given outcome (OBS-CRM-09). The outcome must be one of the constants from
// OBS-CRM-06; unrecognized values are rejected and logged.
func RecordReconcileOutcome(ctx context.Context, kind, outcome string) {
	if reconcileOutcomes == nil {
		return
	}
	if !isValidOutcome(outcome) {
		log.Printf("WARN invalid reconcile outcome %q for kind %s; metric not recorded", outcome, kind)
		return
	}
	reconcileOutcomes.Add(ctx, 1, metric.WithAttributes(
		attribute.String("resource.kind", kind),
		attribute.String("outcome", outcome),
	))
}

// RecordReconcileFailedWithReason increments the reconcile.failed counter with
// the given reason code (OBS-CRM-11). The reason must be a failed code from
// OBS-CRM-07 or ReasonUnknown; unrecognized values fall back to ReasonUnknown and
// are logged at WARN.
func RecordReconcileFailedWithReason(ctx context.Context, kind, reason string) {
	if reconcileFailed == nil {
		return
	}
	validatedReason := validateFailedReason(reason)
	if validatedReason != reason {
		log.Printf("WARN unrecognized failed reason %q for kind %s; using %q", reason, kind, validatedReason)
	}
	reconcileFailed.Add(ctx, 1, metric.WithAttributes(
		attribute.String("resource.kind", kind),
		attribute.String("reason", validatedReason),
	))
}

// isValidOutcome returns true if outcome is one of the OBS-CRM-06 constants.
func isValidOutcome(outcome string) bool {
	switch outcome {
	case OutcomeNoop, OutcomeSuccess, OutcomeRetryable, OutcomeFailed:
		return true
	default:
		return false
	}
}

// validateRetryableReason returns the reason if it is a valid retryable code
// from OBS-CRM-07, otherwise returns ReasonUnknown.
func validateRetryableReason(reason string) string {
	switch reason {
	case ReasonGRPCUnavailable, ReasonK8sConflict, ReasonK8sUnavailable,
		ReasonDependencyNotReady, ReasonKeycloakTransient, ReasonUnknown:
		return reason
	default:
		return ReasonUnknown
	}
}

// validateFailedReason returns the reason if it is a valid failed code from
// OBS-CRM-07, otherwise returns ReasonUnknown.
func validateFailedReason(reason string) string {
	switch reason {
	case ReasonInvalidConfig, ReasonMissingPrerequisite, ReasonIdentityInvalid, ReasonUnknown:
		return reason
	default:
		return ReasonUnknown
	}
}
