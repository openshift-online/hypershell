package managedClusters

import (
	"testing"
	"time"

	"github.com/openshift-online/rh-trex-ai/pkg/api"
)

func TestBuildClusterInventorySnapshot(t *testing.T) {
	evaluationTime := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	recentCreatedAt := evaluationTime.Add(-10 * 24 * time.Hour)
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
	}

	snapshot := buildClusterInventorySnapshot(clusters, evaluationTime)

	if snapshot.CreatedLast30Days != 2 {
		t.Fatalf("expected 2 clusters created in the last 30 days, got %d", snapshot.CreatedLast30Days)
	}

	if len(snapshot.Rows) != 3 {
		t.Fatalf("expected 3 inventory rows, got %d", len(snapshot.Rows))
	}
}

func strPtr(value string) *string {
	return &value
}
