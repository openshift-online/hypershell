package managedDatabases

import "strings"

// DatabaseInventoryRow is one status bucket in the managed database inventory
// aggregate exposed to Prometheus.
type DatabaseInventoryRow struct {
	Status string
	Count  int64
}

// DatabaseInventorySnapshot is the fleet-wide managed database inventory
// computed on each metrics scrape.
type DatabaseInventorySnapshot struct {
	Rows []DatabaseInventoryRow
}

func inventoryBucketOptional(value *string) string {
	if value == nil {
		return "unknown"
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func buildDatabaseInventorySnapshot(databases ManagedDatabaseList) *DatabaseInventorySnapshot {
	counts := make(map[string]int64)
	for _, database := range databases {
		status := inventoryBucketOptional(database.Status)
		counts[status]++
	}

	rows := make([]DatabaseInventoryRow, 0, len(counts))
	for status, count := range counts {
		rows = append(rows, DatabaseInventoryRow{
			Status: status,
			Count:  count,
		})
	}

	return &DatabaseInventorySnapshot{Rows: rows}
}
