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
	runningGatewaysGauge prometheus.GaugeFunc
	metricsOnce          sync.Once
)

// RegisterGatewayMetrics registers a Prometheus GaugeFunc that reports the
// number of running gateways by querying the database on each scrape.
// It is safe to call multiple times; subsequent calls are no-ops.
func RegisterGatewayMetrics(dao GatewayDao) {
	metricsOnce.Do(func() {
		runningGatewaysGauge = prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Subsystem: metricsSubsystem,
				Name:      "running",
				Help:      "Current number of running gateways.",
			},
			func() float64 {
				gateways, err := dao.All(context.Background())
				if err != nil {
					return 0
				}
				return float64(len(gateways))
			},
		)
		prometheus.MustRegister(runningGatewaysGauge)
	})
}
