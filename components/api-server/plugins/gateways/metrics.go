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
// owned by the gatewayhealth package (single source of truth).
const metricsHelp = "Number of gateways by phase (Pending, Provisioning, Running, Degraded, Failed)."

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
}
