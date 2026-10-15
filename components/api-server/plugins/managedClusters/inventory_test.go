package managedClusters

import (
	"testing"
	"time"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/api"
)

func TestBuildClusterInventorySnapshot(t *testing.T) {
	evaluationTime := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	recentCreatedAt := evaluationTime.Add(-10 * 24 * time.Hour)
	veryRecentCreatedAt := evaluationTime.Add(-3 * 24 * time.Hour)
	oldCreatedAt := evaluationTime.Add(-60 * 24 * time.Hour)

	clusters := ManagedClusterList{
		{
			Meta:     api.Meta{CreatedAt: recentCreatedAt},
			Provider: "aws",
			Region:   strPtr("us-east-1"),
			Status:   strPtr("Ready"),
		},
		{
			Meta:     api.Meta{CreatedAt: oldCreatedAt},
			Provider: "openshift",
			Region:   nil,
			Status:   strPtr("Failed"),
		},
		{
			Meta:     api.Meta{CreatedAt: recentCreatedAt},
			Provider: "aws",
			Region:   strPtr(""),
			Status:   nil,
		},
		{
			Meta:     api.Meta{CreatedAt: veryRecentCreatedAt},
			Provider: "gcp",
			Region:   strPtr("eu-west-1"),
			Status:   strPtr("Ready"),
		},
	}

	snapshot := buildClusterInventorySnapshot(clusters, evaluationTime)

	if snapshot.CreatedLast30Days != 3 {
		t.Fatalf("expected 3 clusters created in the last 30 days, got %d", snapshot.CreatedLast30Days)
	}
	if snapshot.CreatedLast7Days != 1 {
		t.Fatalf("expected 1 cluster created in the last 7 days, got %d", snapshot.CreatedLast7Days)
	}

	if len(snapshot.Rows) != 4 {
		t.Fatalf("expected 4 inventory rows, got %d", len(snapshot.Rows))
	}
}

// Registered clusters self-declare provider/region through the capability model
// and no longer write the Provider/Region columns. The inventory metric must
// derive the buckets from the capability labels; otherwise every registered
// cluster reports provider="unknown".
func TestBuildClusterInventorySnapshotDerivesFromCapabilities(t *testing.T) {
	evaluationTime := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

	clusters := ManagedClusterList{
		{
			Meta:         api.Meta{CreatedAt: evaluationTime},
			Status:       strPtr("Ready"),
			Capabilities: []string{"provider/aws", "region/us-east-1"},
		},
	}

	snapshot := buildClusterInventorySnapshot(clusters, evaluationTime)

	if len(snapshot.Rows) != 1 {
		t.Fatalf("expected 1 inventory row, got %d", len(snapshot.Rows))
	}
	row := snapshot.Rows[0]
	if row.Provider != "aws" {
		t.Errorf("provider = %q, want %q", row.Provider, "aws")
	}
	if row.Region != "us-east-1" {
		t.Errorf("region = %q, want %q", row.Region, "us-east-1")
	}
}

func strPtr(value string) *string {
	return &value
}
