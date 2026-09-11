package managedDatabases

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "hypershell"
	metricsSubsystem = "managed_databases"
)

var managedDatabaseMetricsOnce sync.Once

// RegisterManagedDatabaseMetrics registers Prometheus gauges for fleet-wide
// managed database inventory. Safe to call multiple times.
func RegisterManagedDatabaseMetrics(dao ManagedDatabaseDao) {
	managedDatabaseMetricsOnce.Do(func() {
		prometheus.MustRegister(newManagedDatabaseInventoryCollector(dao))
	})
}

type managedDatabaseInventoryCollector struct {
	dao             ManagedDatabaseDao
	totalDesc       *prometheus.Desc
	dimensionedDesc *prometheus.Desc
}

func newManagedDatabaseInventoryCollector(dao ManagedDatabaseDao) *managedDatabaseInventoryCollector {
	return &managedDatabaseInventoryCollector{
		dao: dao,
		totalDesc: prometheus.NewDesc(
			prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "total"),
			"Total registered managed databases.",
			nil,
			nil,
		),
		dimensionedDesc: prometheus.NewDesc(
			prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "inventory_total"),
			"Managed databases by inventory status.",
			[]string{"status"},
			nil,
		),
	}
}

func (c *managedDatabaseInventoryCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalDesc
	ch <- c.dimensionedDesc
}

func (c *managedDatabaseInventoryCollector) Collect(ch chan<- prometheus.Metric) {
	snapshot, err := c.dao.InventorySnapshot(context.Background())
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.totalDesc, err)
		return
	}

	var total int64
	for _, row := range snapshot.Rows {
		total += row.Count
		ch <- prometheus.MustNewConstMetric(
			c.dimensionedDesc,
			prometheus.GaugeValue,
			float64(row.Count),
			row.Status,
		)
	}

	ch <- prometheus.MustNewConstMetric(c.totalDesc, prometheus.GaugeValue, float64(total))
}
