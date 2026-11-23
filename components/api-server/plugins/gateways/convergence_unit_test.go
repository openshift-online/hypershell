package gateways

import (
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/api-server/pkg/gatewayhealth"
)

func strptr(s string) *string { return &s }

// TestIsConverged covers the convergence condition of OBS-SLI-01: converged iff
// observed_generation == generation AND phase == Running for the newest
// generation. Degraded, Failed, or observed_generation < generation is not
// converged.
func TestIsConverged(t *testing.T) {
	cases := []struct {
		name       string
		generation int64
		observed   int64
		phase      *string
		want       bool
	}{
		{"running and observed", 2, 2, strptr(string(gatewayhealth.PhaseRunning)), true},
		{"observed trails generation", 2, 1, strptr(string(gatewayhealth.PhaseRunning)), false},
		{"degraded is not converged", 1, 1, strptr(string(gatewayhealth.PhaseDegraded)), false},
		{"failed is not converged", 1, 1, strptr(string(gatewayhealth.PhaseFailed)), false},
		{"provisioning is not converged", 1, 1, strptr(string(gatewayhealth.PhaseProvisioning)), false},
		{"nil phase is not converged", 1, 1, nil, false},
		{"created latch is not converged", 1, 0, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &Gateway{Phase: tc.phase}
			g.Generation = tc.generation
			g.ObservedGeneration = tc.observed
			if got := isConverged(g); got != tc.want {
				t.Fatalf("isConverged() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestChangedConditions proves a condition-transition timestamp is stamped only
// for a new or changed provisioning condition (OBS-SLI-01), never for an
// unchanged replay.
func TestChangedConditions(t *testing.T) {
	cur := strptr(`[{"type":"GatewayHealthy","condition_status":"InProgress","message":""}]`)
	next := strptr(`[{"type":"GatewayHealthy","condition_status":"Complete","message":""},{"type":"RouteReady","condition_status":"InProgress","message":""}]`)

	changed := changedConditions(cur, next)
	gotTypes := map[string]string{}
	for _, c := range changed {
		gotTypes[c.Type] = c.ConditionStatus
	}
	if gotTypes["GatewayHealthy"] != "Complete" {
		t.Fatalf("expected GatewayHealthy->Complete change, got %v", gotTypes)
	}
	if gotTypes["RouteReady"] != "InProgress" {
		t.Fatalf("expected new RouteReady condition, got %v", gotTypes)
	}

	// An identical replay yields no change.
	if got := changedConditions(next, next); len(got) != 0 {
		t.Fatalf("unchanged conditions reported %d changes, want 0", len(got))
	}
	// A nil incoming document yields no change (nothing to stamp).
	if got := changedConditions(cur, nil); len(got) != 0 {
		t.Fatalf("nil incoming reported %d changes, want 0", len(got))
	}
}

// TestGatewayCreatedAtOr covers the create-time fallback used to compute the
// provision duration when a partial status write does not carry Meta.
func TestGatewayCreatedAtOr(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	withMeta := &Gateway{}
	withMeta.CreatedAt = created
	if got := withMeta.GatewayCreatedAtOr(nil); !got.Equal(created) {
		t.Fatalf("own create time = %v, want %v", got, created)
	}

	zero := &Gateway{}
	fallback := &Gateway{}
	fallback.CreatedAt = created
	if got := zero.GatewayCreatedAtOr(fallback); !got.Equal(created) {
		t.Fatalf("fallback create time = %v, want %v", got, created)
	}
}

// TestConvergenceLabelDomainsBounded guards the OBS-SLI-03 cardinality cap: the
// counter carries only three change types and three outcomes (at most nine
// series), and the provision buckets cover 1 second through 15 minutes.
func TestConvergenceLabelDomainsBounded(t *testing.T) {
	if len(changeTypes) != 3 {
		t.Fatalf("change types = %v, want exactly 3", changeTypes)
	}
	if len(outcomes) != 3 {
		t.Fatalf("outcomes = %v, want exactly 3", outcomes)
	}
	if len(changeTypes)*len(outcomes) != 9 {
		t.Fatalf("series cap = %d, want 9", len(changeTypes)*len(outcomes))
	}
	if provisionDurationBuckets[0] != 1 {
		t.Fatalf("first bucket = %v, want 1 second", provisionDurationBuckets[0])
	}
	if last := provisionDurationBuckets[len(provisionDurationBuckets)-1]; last != 900 {
		t.Fatalf("last bucket = %v, want 900 seconds (15 minutes)", last)
	}
}
