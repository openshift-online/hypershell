package managedClusters

import (
	"strings"
	"time"
)

const inventoryLookback = 30 * 24 * time.Hour

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

func inventoryLookbackStart(evaluationTime time.Time) time.Time {
	return evaluationTime.UTC().Add(-inventoryLookback)
}

func buildClusterInventorySnapshot(
	clusters ManagedClusterList,
	evaluationTime time.Time,
) *ClusterInventorySnapshot {
	windowStart := inventoryLookbackStart(evaluationTime)
	type inventoryKey struct {
		status   string
		provider string
		region   string
	}
	counts := make(map[inventoryKey]int64)
	var createdLast30Days int64

	for _, cluster := range clusters {
		key := inventoryKey{
			status:   inventoryBucketOptional(cluster.Status),
			provider: inventoryBucketString(cluster.Provider),
			region:   inventoryBucketOptional(cluster.Region),
		}
		counts[key]++

		if !cluster.CreatedAt.IsZero() && !cluster.CreatedAt.Before(windowStart) {
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
		CreatedLast30Days: createdLast30Days,
		Rows:              rows,
	}
}
