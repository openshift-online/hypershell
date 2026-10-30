package e2e

import (
	"testing"
	"time"
)

func TestShouldSweepGateway(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ptr := func(d time.Duration) *time.Time {
		at := now.Add(d)
		return &at
	}

	tests := []struct {
		name      string
		gwName    string
		createdAt *time.Time
		want      bool
	}{
		{
			name:      "stale e2e gateway is swept",
			gwName:    "e2e-gw-12345",
			createdAt: ptr(-2 * time.Hour),
			want:      true,
		},
		{
			name:      "stale e2e deletion victim is swept",
			gwName:    "e2e-padmin-del-12345",
			createdAt: ptr(-90 * time.Minute),
			want:      true,
		},
		{
			name:      "fresh e2e gateway is spared (concurrent run)",
			gwName:    "e2e-gw-12345",
			createdAt: ptr(-5 * time.Minute),
			want:      false,
		},
		{
			name:      "e2e gateway from a concurrent long run is spared",
			gwName:    "e2e-sbx-99999",
			createdAt: ptr(-40 * time.Minute),
			want:      false,
		},
		{
			name:      "non-e2e gateway is never swept",
			gwName:    "prod-gateway",
			createdAt: ptr(-72 * time.Hour),
			want:      false,
		},
		{
			name:      "e2e gateway with no timestamp is spared",
			gwName:    "e2e-del-12345",
			createdAt: nil,
			want:      false,
		},
		{
			name:      "exactly at the age threshold is swept",
			gwName:    "e2e-gw-1",
			createdAt: ptr(-staleGatewaySweepAge),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSweepGateway(tt.gwName, tt.createdAt, now, staleGatewaySweepAge)
			if got != tt.want {
				t.Errorf("shouldSweepGateway(%q, %v) = %v, want %v", tt.gwName, tt.createdAt, got, tt.want)
			}
		})
	}
}
