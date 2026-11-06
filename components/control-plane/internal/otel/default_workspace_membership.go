package otel

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// defaultWorkspaceMembershipPopulations tracks, per cluster, the last observed
// reconciliation_status of every live DefaultWorkspaceMembership row this
// control plane's watch has seen, so the pending/failed gauges (OBS-CRM-14)
// reflect the current watch population. Unlike the Gateway staleness tracker
// above, this has no seed-phase debounce: a transient recompute mid-seed is
// acceptable here because every row the watch delivers carries its own
// authoritative status, so there is no "stale until the whole seed lands"
// window to protect against.
var defaultWorkspaceMembershipPopulations = struct {
	sync.Mutex
	byCluster map[string]map[string]string // cluster -> membership id -> reconciliation_status
}{byCluster: make(map[string]map[string]string)}

// RecordDefaultWorkspaceMembershipState updates the tracked reconciliation
// status of one DefaultWorkspaceMembership row (from a seed, resync, or live
// watch event) and republishes the pending/failed gauges for its cluster.
// deleted removes the row from the tracked population instead of recording a
// status.
func RecordDefaultWorkspaceMembershipState(ctx context.Context, clusterID, membershipID, status string, deleted bool) {
	if !enabled || clusterID == "" || membershipID == "" {
		return
	}
	defaultWorkspaceMembershipPopulations.Lock()
	population := defaultWorkspaceMembershipPopulations.byCluster[clusterID]
	if population == nil {
		population = make(map[string]string)
		defaultWorkspaceMembershipPopulations.byCluster[clusterID] = population
	}
	if deleted {
		delete(population, membershipID)
	} else {
		population[membershipID] = status
	}
	pending, failed := 0, 0
	for _, s := range population {
		switch s {
		case "pending":
			pending++
		case "failed":
			failed++
		}
	}
	defaultWorkspaceMembershipPopulations.Unlock()

	attrs := metric.WithAttributes(attribute.String("hypershell.cluster_id", clusterID))
	if defaultWorkspaceMembershipPending != nil {
		defaultWorkspaceMembershipPending.Record(ctx, int64(pending), attrs)
	}
	if defaultWorkspaceMembershipFailed != nil {
		defaultWorkspaceMembershipFailed.Record(ctx, int64(failed), attrs)
	}
}

// RecordDefaultWorkspaceReconcileSuccess advances the last-success timestamp
// gauge (OBS-CRM-14.03). Call it only on a reconcile pass that converges a
// gateway's default workspace membership without error -- never on a pass that
// fails, and never on a noop pass that found nothing to do -- so a stalled
// reconciler is visible as a gauge that stops advancing.
func RecordDefaultWorkspaceReconcileSuccess(ctx context.Context, clusterID string, at time.Time) {
	if !enabled || defaultWorkspaceReconcileLastOK == nil {
		return
	}
	defaultWorkspaceReconcileLastOK.Record(ctx, at.Unix(), metric.WithAttributes(attribute.String("hypershell.cluster_id", clusterID)))
}
