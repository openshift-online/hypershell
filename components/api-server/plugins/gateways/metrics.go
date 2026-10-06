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

var metricsOnce sync.Once

// metricsHelp describes the per-phase, per-managed-cluster gateway gauge. The
// canonical phase set is owned by the gatewayhealth package (single source of
// truth); any non-canonical or blank phase is aggregated into the "other" bucket.
// The managed_cluster label is the ManagedCluster.Name (== GitOps spoke name) the
// gateway runs on, so counts attribute to the spoke, not the emitting hub.
const metricsHelp = "Number of gateways by phase (Pending, Provisioning, Running, Degraded, Failed, other) and managed cluster."

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
			[]string{"phase", "managed_cluster"},
			nil,
		),
	}
}

func (c *gatewayCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *gatewayCollector) Collect(ch chan<- prometheus.Metric) {
	rows, err := c.dao.CountByClusterAndPhase(context.Background())
	if err != nil {
		// Emit an error metric so the scrape doesn't silently drop the series.
		ch <- prometheus.NewInvalidMetric(c.desc, err)
		return
	}

	// Fold into managed cluster -> phase -> count so each spoke's series can be
	// emitted gapless (all canonical phases) plus its own "other" bucket.
	byCluster := map[string]map[string]int64{}
	for _, r := range rows {
		phases, ok := byCluster[r.ClusterName]
		if !ok {
			phases = map[string]int64{}
			byCluster[r.ClusterName] = phases
		}
		phases[r.Phase] += r.Count
	}

	for cluster, counts := range byCluster {
		// Always emit the canonical phases per spoke so graphs never have gaps.
		for _, phase := range gatewayhealth.PhaseStrings() {
			ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(counts[phase]), phase, cluster)
		}

		// Aggregate any gateway whose stored phase is outside the canonical set
		// (legacy or blank rows) into a single "other" bucket per spoke, emitted
		// every scrape even at zero, so total drift stays visible rather than
		// silently dropped.
		var other float64
		for phase, count := range counts {
			if !gatewayhealth.IsValidPhase(phase) {
				other += float64(count)
			}
		}
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, other, gatewayPhaseOther, cluster)
	}
}

const activeSandboxesHelp = "Active agent sandboxes across gateways, by managed cluster (ManagedCluster.Name / GitOps spoke name)."

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
			[]string{"managed_cluster"},
			nil,
		),
	}
}

func (c *gatewayActiveSandboxesCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *gatewayActiveSandboxesCollector) Collect(ch chan<- prometheus.Metric) {
	rows, err := c.dao.SumActiveSandboxCountByCluster(context.Background())
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.desc, err)
		return
	}
	for _, r := range rows {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(r.Count), r.ClusterName)
	}
}
