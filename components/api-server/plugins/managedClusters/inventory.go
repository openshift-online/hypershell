package managedClusters

import (
	"strings"
	"time"
)

const inventoryLookback7Days = 7 * 24 * time.Hour
const inventoryLookback30Days = 30 * 24 * time.Hour

// ClusterInventoryRow is one labeled bucket in the managed cluster inventory
// aggregate exposed to Prometheus.
type ClusterInventoryRow struct {
	Status   string
	Provider string
	Region   string
	Count    int64
}

// ClusterInventorySnapshot is the fleet-wide managed cluster inventory computed
// on each metrics scrape.
type ClusterInventorySnapshot struct {
	CreatedLast7Days  int64
	CreatedLast30Days int64
	Rows              []ClusterInventoryRow
}

func inventoryBucketString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func inventoryBucketOptional(value *string) string {
	if value == nil {
		return "unknown"
	}
	return inventoryBucketString(*value)
}

// clusterProvider resolves the provider for the inventory metric. Registered
// clusters self-declare provider through the capability model and no longer write
// the legacy Provider column, so the column is preferred (manually created
// records set it directly) and the provider/ capability label is the fallback.
// Without this, every registered cluster would bucket as provider="unknown".
func clusterProvider(cluster *ManagedCluster) string {
	if strings.TrimSpace(cluster.Provider) != "" {
		return cluster.Provider
	}
	return capabilityValue(cluster.Capabilities, providerNamespace)
}

// clusterRegion resolves the region for the inventory metric with the same
// column-then-capability precedence as clusterProvider.
func clusterRegion(cluster *ManagedCluster) string {
	if cluster.Region != nil && strings.TrimSpace(*cluster.Region) != "" {
		return *cluster.Region
	}
	return capabilityValue(cluster.Capabilities, regionNamespace)
}

func buildClusterInventorySnapshot(
	clusters ManagedClusterList,
	evaluationTime time.Time,
) *ClusterInventorySnapshot {
	window7DaysStart := evaluationTime.UTC().Add(-inventoryLookback7Days)
	window30DaysStart := evaluationTime.UTC().Add(-inventoryLookback30Days)
	type inventoryKey struct {
		status   string
		provider string
		region   string
	}
	counts := make(map[inventoryKey]int64)
	var createdLast7Days int64
	var createdLast30Days int64

	for _, cluster := range clusters {
		key := inventoryKey{
			status:   inventoryBucketOptional(cluster.Status),
			provider: inventoryBucketString(clusterProvider(cluster)),
			region:   inventoryBucketString(clusterRegion(cluster)),
		}
		counts[key]++

		if !cluster.CreatedAt.IsZero() && !cluster.CreatedAt.Before(window7DaysStart) {
			createdLast7Days++
		}
		if !cluster.CreatedAt.IsZero() && !cluster.CreatedAt.Before(window30DaysStart) {
			createdLast30Days++
		}
	}

	rows := make([]ClusterInventoryRow, 0, len(counts))
	for key, count := range counts {
		rows = append(rows, ClusterInventoryRow{
			Status:   key.status,
			Provider: key.provider,
			Region:   key.region,
			Count:    count,
		})
	}

	return &ClusterInventorySnapshot{
		CreatedLast7Days:  createdLast7Days,
		CreatedLast30Days: createdLast30Days,
		Rows:              rows,
	}
}
