package managedDatabases

import "testing"

func TestBuildDatabaseInventorySnapshot(t *testing.T) {
	databases := ManagedDatabaseList{
		{Status: strPtr("Ready")},
		{Status: nil},
		{Status: strPtr("")},
	}

	snapshot := buildDatabaseInventorySnapshot(databases)

	if len(snapshot.Rows) != 2 {
		t.Fatalf("expected 2 inventory rows, got %d", len(snapshot.Rows))
	}
}

func strPtr(value string) *string {
	return &value
}
