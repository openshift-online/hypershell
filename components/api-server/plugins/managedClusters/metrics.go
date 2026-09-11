package managedClusters

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "hypershell"
	metricsSubsystem = "managed_clusters"
)

var (
	managedClusterMetricsOnce sync.Once
)

// RegisterManagedClusterMetrics registers Prometheus gauges for fleet-wide
// managed cluster inventory. Safe to call multiple times.
func RegisterManagedClusterMetrics(dao ManagedClusterDao) {
	managedClusterMetricsOnce.Do(func() {
		prometheus.MustRegister(newManagedClusterInventoryCollector(dao))
	})
}

type managedClusterInventoryCollector struct {
	dao             ManagedClusterDao
	totalDesc       *prometheus.Desc
	createdDesc     *prometheus.Desc
	dimensionedDesc *prometheus.Desc
}

func newManagedClusterInventoryCollector(dao ManagedClusterDao) *managedClusterInventoryCollector {
	return &managedClusterInventoryCollector{
		dao: dao,
		totalDesc: prometheus.NewDesc(
			prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "total"),
			"Total registered managed clusters.",
			nil,
			nil,
		),
		createdDesc: prometheus.NewDesc(
			prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "created_last_30_days_total"),
			"Managed clusters created in the last 30 days.",
			nil,
			nil,
		),
		dimensionedDesc: prometheus.NewDesc(
			prometheus.BuildFQName(metricsNamespace, metricsSubsystem, "inventory_total"),
			"Managed clusters by inventory status, provider, and region.",
			[]string{"status", "provider", "region"},
			nil,
		),
	}
}

func (c *managedClusterInventoryCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalDesc
	ch <- c.createdDesc
	ch <- c.dimensionedDesc
}

func (c *managedClusterInventoryCollector) Collect(ch chan<- prometheus.Metric) {
	snapshot, err := c.dao.InventorySnapshot(context.Background(), time.Now().UTC())
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
			row.Provider,
			row.Region,
		)
	}

	ch <- prometheus.MustNewConstMetric(c.totalDesc, prometheus.GaugeValue, float64(total))
	ch <- prometheus.MustNewConstMetric(
		c.createdDesc,
		prometheus.GaugeValue,
		float64(snapshot.CreatedLast30Days),
	)
}
