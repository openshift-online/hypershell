package gateway

import (
	"testing"
	"time"
)

func TestClassifyAttentionCounts(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	live := map[string]struct{}{"openshell-live": {}}

	ptr := func(d time.Duration) *time.Time {
		ts := now.Add(d)
		return &ts
	}

	tests := []struct {
		name         string
		observations []SandboxObservation
		want         AttentionCounts
	}{
		{
			name: "orphaned active pods counted; succeeded excluded",
			observations: []SandboxObservation{
				{Namespace: "openshell-orphan", Active: true, CreationTime: now.Add(-time.Hour)},
				{Namespace: "openshell-orphan", Active: true, CreationTime: now.Add(-time.Hour)},
				{Namespace: "openshell-orphan", Active: false, CreationTime: now.Add(-time.Hour)},
			},
			want: AttentionCounts{Orphaned: 2},
		},
		{
			name: "active with live gateway is not orphaned",
			observations: []SandboxObservation{
				{Namespace: "openshell-live", Active: true, CreationTime: now.Add(-time.Hour)},
				{Namespace: "openshell-live", Active: true, CreationTime: now.Add(-time.Hour)},
				{Namespace: "openshell-live", Active: true, CreationTime: now.Add(-time.Hour)},
			},
			want: AttentionCounts{},
		},
		{
			name: "shutdown within 24h counts as expiring",
			observations: []SandboxObservation{
				{Namespace: "openshell-live", Active: true, ShutdownTime: ptr(6 * time.Hour), CreationTime: now.Add(-time.Hour)},
			},
			want: AttentionCounts{Expiring: 1},
		},
		{
			name: "past shutdown still active counts as expiring",
			observations: []SandboxObservation{
				{Namespace: "openshell-live", Active: true, ShutdownTime: ptr(-2 * time.Hour), CreationTime: now.Add(-time.Hour)},
			},
			want: AttentionCounts{Expiring: 1},
		},
		{
			name: "no shutdownTime does not count as expiring",
			observations: []SandboxObservation{
				{Namespace: "openshell-live", Active: true, CreationTime: now.Add(-time.Hour)},
			},
			want: AttentionCounts{},
		},
		{
			name: "far-future shutdown does not count as expiring",
			observations: []SandboxObservation{
				{Namespace: "openshell-live", Active: true, ShutdownTime: ptr(48 * time.Hour), CreationTime: now.Add(-time.Hour)},
			},
			want: AttentionCounts{},
		},
		{
			name: "stale lastActivityTime counts as idle",
			observations: []SandboxObservation{
				{Namespace: "openshell-live", Active: true, LastActivityTime: ptr(-90 * time.Minute), CreationTime: now.Add(-2 * time.Hour)},
			},
			want: AttentionCounts{Idle: 1},
		},
		{
			name: "recent lastActivityTime is not idle",
			observations: []SandboxObservation{
				{Namespace: "openshell-live", Active: true, LastActivityTime: ptr(-10 * time.Minute), CreationTime: now.Add(-2 * time.Hour)},
			},
			want: AttentionCounts{},
		},
		{
			name: "unset lastActivity and age over 24h is idle",
			observations: []SandboxObservation{
				{Namespace: "openshell-live", Active: true, CreationTime: now.Add(-30 * time.Hour)},
			},
			want: AttentionCounts{Idle: 1},
		},
		{
			name: "unset lastActivity and age under 24h is not idle",
			observations: []SandboxObservation{
				{Namespace: "openshell-live", Active: true, CreationTime: now.Add(-2 * time.Hour)},
			},
			want: AttentionCounts{},
		},
		{
			name: "orphaned is not also idle",
			observations: []SandboxObservation{
				{Namespace: "openshell-orphan", Active: true, LastActivityTime: ptr(-2 * time.Hour), CreationTime: now.Add(-3 * time.Hour)},
			},
			want: AttentionCounts{Orphaned: 1},
		},
		{
			name: "orphaned and expiring dual membership",
			observations: []SandboxObservation{
				{Namespace: "openshell-orphan", Active: true, ShutdownTime: ptr(6 * time.Hour), CreationTime: now.Add(-time.Hour)},
			},
			want: AttentionCounts{Orphaned: 1, Expiring: 1},
		},
		{
			name: "idle and expiring dual membership",
			observations: []SandboxObservation{
				{
					Namespace:        "openshell-live",
					Active:           true,
					LastActivityTime: ptr(-2 * time.Hour),
					ShutdownTime:     ptr(6 * time.Hour),
					CreationTime:     now.Add(-3 * time.Hour),
				},
			},
			want: AttentionCounts{Idle: 1, Expiring: 1},
		},
		{
			name: "exactly 1h stale activity is idle",
			observations: []SandboxObservation{
				{Namespace: "openshell-live", Active: true, LastActivityTime: ptr(-IdleStaleActivity), CreationTime: now.Add(-2 * time.Hour)},
			},
			want: AttentionCounts{Idle: 1},
		},
		{
			name: "exactly 24h never-used age is idle",
			observations: []SandboxObservation{
				{Namespace: "openshell-live", Active: true, CreationTime: now.Add(-IdleNeverUsedAge)},
			},
			want: AttentionCounts{Idle: 1},
		},
		{
			name: "exactly now+24h shutdown is expiring",
			observations: []SandboxObservation{
				{Namespace: "openshell-live", Active: true, ShutdownTime: ptr(ExpiringSoonWindow), CreationTime: now.Add(-time.Hour)},
			},
			want: AttentionCounts{Expiring: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyAttentionCounts(now, live, tt.observations)
			if got != tt.want {
				t.Fatalf("ClassifyAttentionCounts() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
