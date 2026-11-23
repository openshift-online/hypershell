package gateways

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// Prometheus export of the durable convergence and provision state on the API
// server metrics endpoint (:4433/metrics, OBS-SLI-03 / OBS-SLI-04). Every
// collector reads the durable PostgreSQL totals per scrape (the DB-backed
// collector pattern the gateway gauges already use), so the exported values are
// monotonic across restarts and never derived from process uptime.

var convergenceMetricsOnce sync.Once

// RegisterConvergenceMetrics registers the change-convergence counter and the
// provision metrics. It is safe to call multiple times; subsequent calls are
// no-ops.
func RegisterConvergenceMetrics(store *convergenceStore) {
	convergenceMetricsOnce.Do(func() {
		prometheus.MustRegister(
			newConvergenceCounterCollector(store),
			newProvisionDurationCollector(store),
			newProvisionOutcomesCollector(store),
		)
	})
}

// convergenceCounterCollector exports hypershell_change_convergence_total
// {change_type, outcome} from the durable per-series totals. All nine series are
// emitted every scrape (missing ones at zero) so graphs are gapless, and no
// sample ever carries a Gateway identifier or namespace name (OBS-SLI-03).
type convergenceCounterCollector struct {
	store *convergenceStore
	desc  *prometheus.Desc
}

func newConvergenceCounterCollector(store *convergenceStore) *convergenceCounterCollector {
	return &convergenceCounterCollector{
		store: store,
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(metricsNamespace, "", "change_convergence_total"),
			"Count of resolved Gateway changes by change type and ten-minute outcome.",
			[]string{"change_type", "outcome"},
			nil,
		),
	}
}

func (c *convergenceCounterCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *convergenceCounterCollector) Collect(ch chan<- prometheus.Metric) {
	counts, err := c.store.convergenceCounts(context.Background())
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.desc, err)
		return
	}
	for _, changeType := range changeTypes {
		for _, outcome := range outcomes {
			total := counts[convergenceCountKey{ChangeType: changeType, Outcome: outcome}]
			ch <- prometheus.MustNewConstMetric(c.desc, prometheus.CounterValue, float64(total), changeType, outcome)
		}
	}
}

// provisionDurationCollector exports gateway_provision_duration_seconds as a
// histogram reconstructed from the durable one-per-gateway create-to-Running
// observations (OBS-SLI-04). The series keeps its exact name so the operational
// dashboard consumes it unchanged when the producer moves from the control plane
// to the API server.
type provisionDurationCollector struct {
	store *convergenceStore
	desc  *prometheus.Desc
}

func newProvisionDurationCollector(store *convergenceStore) *provisionDurationCollector {
	return &provisionDurationCollector{
		store: store,
		desc: prometheus.NewDesc(
			"gateway_provision_duration_seconds",
			"Gateway provision duration in seconds, one observation per gateway on its first transition to Running.",
			nil,
			nil,
		),
	}
}

func (c *provisionDurationCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *provisionDurationCollector) Collect(ch chan<- prometheus.Metric) {
	summary, err := c.store.provisionDurations(context.Background())
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.desc, err)
		return
	}
	ch <- prometheus.MustNewConstHistogram(c.desc, summary.Count, summary.Sum, summary.Buckets)
}

// provisionOutcomesCollector exports gateway_provision_outcomes_total{outcome}
// from the durable one-per-gateway terminal outcomes (OBS-SLI-04). Both outcome
// series are emitted every scrape (missing ones at zero).
type provisionOutcomesCollector struct {
	store *convergenceStore
	desc  *prometheus.Desc
}

func newProvisionOutcomesCollector(store *convergenceStore) *provisionOutcomesCollector {
	return &provisionOutcomesCollector{
		store: store,
		desc: prometheus.NewDesc(
			"gateway_provision_outcomes_total",
			"Count of terminal Gateway provision outcomes (success or failure), one per gateway.",
			[]string{"outcome"},
			nil,
		),
	}
}

func (c *provisionOutcomesCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *provisionOutcomesCollector) Collect(ch chan<- prometheus.Metric) {
	counts, err := c.store.provisionOutcomeCounts(context.Background())
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.desc, err)
		return
	}
	for _, outcome := range []string{provisionOutcomeSuccess, provisionOutcomeFailure} {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.CounterValue, float64(counts[outcome]), outcome)
	}
}
