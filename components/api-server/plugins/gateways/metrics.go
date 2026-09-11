package gateways

import (
	"context"
	"sync"

	"github.com/openshift-online/hypershell/components/api-server/pkg/gatewayhealth"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "hypershell"
	metricsSubsystem = "gateways"
)

var (
	gatewayTotalVec *prometheus.GaugeVec
	metricsOnce     sync.Once
)

// metricsHelp describes the per-phase gateway gauge. The canonical phase set is
// owned by the gatewayhealth package (single source of truth); any non-canonical
// or blank phase is aggregated into the "other" bucket.
const metricsHelp = "Number of gateways by phase (Pending, Provisioning, Running, Degraded, Failed, other)."

// gatewayPhaseOther is the catch-all label for gateways whose stored phase is
// outside the canonical vocabulary (legacy or blank rows the service layer still
// tolerates), so the total never silently under-reports.
const gatewayPhaseOther = "other"

// RegisterGatewayMetrics registers a Prometheus GaugeVec that reports the
// number of gateways broken down by phase. The gauge is refreshed on every
// scrape by querying the database. It is safe to call multiple times;
// subsequent calls are no-ops.
func RegisterGatewayMetrics(dao GatewayDao) {
	metricsOnce.Do(func() {
		gatewayTotalVec = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "total",
				Help:      metricsHelp,
			},
			[]string{"phase"},
		)

		// Pre-seed the canonical phases so they always appear in the output even
		// when the count is zero, avoiding gaps in graphs.
		for _, phase := range gatewayhealth.PhaseStrings() {
			gatewayTotalVec.WithLabelValues(phase).Set(0)
		}

		prometheus.MustRegister(newGatewayCollector(dao))
		prometheus.MustRegister(newGatewayActiveSandboxesCollector(dao))
	})
}

// gatewayCollector is a custom Collector so that the DB query runs exactly
// once per scrape rather than once per gauge.
type gatewayCollector struct {
	dao  GatewayDao
	desc *prometheus.Desc
}

func newGatewayCollector(dao GatewayDao) *gatewayCollector {
	return &gatewayCollector{
		dao: dao,
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "total"),
			metricsHelp,
			[]string{"phase"},
			nil,
		),
	}
}

func (c *gatewayCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *gatewayCollector) Collect(ch chan<- prometheus.Metric) {
	counts, err := c.dao.CountByPhase(context.Background())
	if err != nil {
		// Emit an error metric so the scrape doesn't silently drop the series.
		ch <- prometheus.NewInvalidMetric(c.desc, err)
		return
	}

	// Always emit the canonical phases so graphs never have gaps.
	for _, phase := range gatewayhealth.PhaseStrings() {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(counts[phase]), phase)
	}

	// Aggregate any gateway whose stored phase is outside the canonical set
	// (legacy or blank rows) into a single "other" bucket, emitted every scrape
	// even at zero, so total drift stays visible rather than silently dropped.
	var other float64
	for phase, count := range counts {
		if !gatewayhealth.IsValidPhase(phase) {
			other += float64(count)
		}
	}
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, other, gatewayPhaseOther)
}

const activeSandboxesHelp = "Total active agent sandboxes across all gateways."

type gatewayActiveSandboxesCollector struct {
	dao  GatewayDao
	desc *prometheus.Desc
}

func newGatewayActiveSandboxesCollector(dao GatewayDao) *gatewayActiveSandboxesCollector {
	return &gatewayActiveSandboxesCollector{
		dao: dao,
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "active_sandboxes_total"),
			activeSandboxesHelp,
			nil,
			nil,
		),
	}
}

func (c *gatewayActiveSandboxesCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *gatewayActiveSandboxesCollector) Collect(ch chan<- prometheus.Metric) {
	total, err := c.dao.SumActiveSandboxCount(context.Background())
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.desc, err)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(total))
}
