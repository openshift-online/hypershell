package gateway

import (
	"time"
)

// Attention windows for sandbox status counts. Measured in wall-clock UTC from
// evaluation time (gateway-sandbox-attention-status.spec.md SSA-02 / SSA-03).
const (
	ExpiringSoonWindow = 24 * time.Hour
	IdleStaleActivity  = 1 * time.Hour
	IdleNeverUsedAge   = 24 * time.Hour
)

// SandboxObservation is one active-or-not sandbox used to derive attention counts.
// Active means the corresponding agent sandbox pod is Pending or Running.
// ShutdownTime / LastActivityTime come from the Sandbox CR when observed;
// CreationTime is the Sandbox creation timestamp, or the pod's when no Sandbox
// object is available.
type SandboxObservation struct {
	Namespace        string
	Active           bool
	ShutdownTime     *time.Time
	LastActivityTime *time.Time
	CreationTime     time.Time
}

// AttentionCounts is the fleet (or cluster-local) tally of attention buckets.
type AttentionCounts struct {
	Orphaned int
	Expiring int
	Idle     int
}

// ClassifyAttentionCounts derives orphaned, expiring-soon, and idle counts from
// sandbox observations. liveGatewayNamespaces is the set of namespaces backed by
// a live HyperShell Gateway. Non-active observations are ignored. Idle and
// orphaned are mutually exclusive; orphaned+expiring and idle+expiring may overlap.
func ClassifyAttentionCounts(
	now time.Time,
	liveGatewayNamespaces map[string]struct{},
	observations []SandboxObservation,
) AttentionCounts {
	var counts AttentionCounts
	expiringDeadline := now.Add(ExpiringSoonWindow)

	for i := range observations {
		obs := &observations[i]
		if !obs.Active {
			continue
		}

		_, hasGateway := liveGatewayNamespaces[obs.Namespace]
		orphaned := !hasGateway
		if orphaned {
			counts.Orphaned++
		}

		if obs.ShutdownTime != nil && !obs.ShutdownTime.After(expiringDeadline) {
			counts.Expiring++
		}

		if orphaned {
			// Idle requires a live owning Gateway (SSA-03).
			continue
		}
		if isIdleSandbox(now, obs) {
			counts.Idle++
		}
	}

	return counts
}

func isIdleSandbox(now time.Time, obs *SandboxObservation) bool {
	if obs.LastActivityTime != nil {
		return !obs.LastActivityTime.After(now.Add(-IdleStaleActivity))
	}
	// Never-used: no activity signal and age at or beyond the grace window.
	if obs.CreationTime.IsZero() {
		return false
	}
	return !obs.CreationTime.After(now.Add(-IdleNeverUsedAge))
}
