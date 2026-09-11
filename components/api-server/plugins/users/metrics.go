package users

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "hypershell"
	metricsSubsystem = "users"
)

var userMetricsOnce sync.Once

// RegisterUserMetrics registers Prometheus gauges for registered user counts.
// Safe to call multiple times.
func RegisterUserMetrics(dao UserDao) {
	userMetricsOnce.Do(func() {
		prometheus.MustRegister(newRegisteredUsersCollector(dao))
	})
}

type registeredUsersCollector struct {
	dao  UserDao
	desc *prometheus.Desc
}

func newRegisteredUsersCollector(dao UserDao) *registeredUsersCollector {
	return &registeredUsersCollector{
		dao: dao,
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "registered_total"),
			"Total registered users.",
			nil,
			nil,
		),
	}
}

func (c *registeredUsersCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *registeredUsersCollector) Collect(ch chan<- prometheus.Metric) {
	count, err := c.dao.CountRegistered(context.Background())
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.desc, err)
		return
	}

	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(count))
}
