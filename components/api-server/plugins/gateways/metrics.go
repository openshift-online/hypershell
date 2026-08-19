package gateways

import (
	"context"
	"sync"

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

// RegisterGatewayMetrics registers a Prometheus GaugeVec that reports the
// number of gateways broken down by phase (Running, Provisioning, Degraded,
// Failed). The gauge is refreshed on every scrape by querying the database.
// It is safe to call multiple times; subsequent calls are no-ops.
func RegisterGatewayMetrics(dao GatewayDao) {
	metricsOnce.Do(func() {
		gatewayTotalVec = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "total",
				Help:      "Number of gateways by phase (Running, Provisioning, Degraded, Failed).",
			},
			[]string{"phase"},
		)

		// Pre-seed the known phases so they always appear in the output even
		// when the count is zero, avoiding gaps in graphs.
		for _, phase := range []string{"Running", "Provisioning", "Degraded", "Failed"} {
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
			"Number of gateways by phase (Running, Provisioning, Degraded, Failed).",
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

	// Always emit the known phases so graphs never have gaps.
	known := map[string]struct{}{
		"Running":      {},
		"Provisioning": {},
		"Degraded":     {},
		"Failed":       {},
	}
	for phase := range known {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(counts[phase]), phase)
	}
}
