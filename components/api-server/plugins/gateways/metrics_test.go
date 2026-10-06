package gateways

import (
	"context"
	"testing"

	"github.com/openshift-online/hypershell/components/api-server/pkg/gatewayhealth"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// ptr returns a pointer to v, for the *string/*int gateway fields.
func ptr[T any](v T) *T { return &v }

// gatherGauge collects a single collector into a throwaway registry and returns
// the emitted samples as labelset->value, keyed by the given label names in order.
func gatherGauge(t *testing.T, c prometheus.Collector, metricName string, labels ...string) map[[2]string]float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register collector: %v", err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := map[[2]string]float64{}
	for _, mf := range mfs {
		if mf.GetName() != metricName {
			continue
		}
		for _, m := range mf.GetMetric() {
			key := keyFor(m, labels)
			out[key] = m.GetGauge().GetValue()
		}
	}
	return out
}

// keyFor builds a fixed-size key from up to two requested labels (phase,
// managed_cluster is the widest label set in this package).
func keyFor(m *dto.Metric, want []string) [2]string {
	byName := map[string]string{}
	for _, lp := range m.GetLabel() {
		byName[lp.GetName()] = lp.GetValue()
	}
	var key [2]string
	for i, w := range want {
		if i < len(key) {
			key[i] = byName[w]
		}
	}
	return key
}

func TestGatewayCollector_AttributesByManagedCluster(t *testing.T) {
	// hyp0-spoke0: two Running; hyp0-spoke1: one Running + one Failed; a gateway
	// with no cluster_id -> "unknown"; a non-canonical phase -> "other".
	dao := NewMockGatewayDao()
	ctx := context.Background()
	_, _ = dao.Create(ctx, &Gateway{ClusterId: "hyp0-spoke0", Phase: ptr("Running")})
	_, _ = dao.Create(ctx, &Gateway{ClusterId: "hyp0-spoke0", Phase: ptr("Running")})
	_, _ = dao.Create(ctx, &Gateway{ClusterId: "hyp0-spoke1", Phase: ptr("Running")})
	_, _ = dao.Create(ctx, &Gateway{ClusterId: "hyp0-spoke1", Phase: ptr("Failed")})
	_, _ = dao.Create(ctx, &Gateway{ClusterId: "", Phase: ptr("Running")})
	_, _ = dao.Create(ctx, &Gateway{ClusterId: "hyp0-spoke1", Phase: ptr("Weird")})

	got := gatherGauge(t, newGatewayCollector(dao), "hypershell_gateways_total", "phase", "managed_cluster")

	// Per-spoke attribution: each spoke counted separately, not lumped.
	if v := got[[2]string{"Running", "hyp0-spoke0"}]; v != 2 {
		t.Errorf("hyp0-spoke0 Running = %v, want 2", v)
	}
	if v := got[[2]string{"Running", "hyp0-spoke1"}]; v != 1 {
		t.Errorf("hyp0-spoke1 Running = %v, want 1", v)
	}
	if v := got[[2]string{"Failed", "hyp0-spoke1"}]; v != 1 {
		t.Errorf("hyp0-spoke1 Failed = %v, want 1", v)
	}

	// Unresolved cluster_id is bucketed, not dropped.
	if v := got[[2]string{"Running", "unknown"}]; v != 1 {
		t.Errorf("unknown Running = %v, want 1", v)
	}

	// Non-canonical phase folds into the per-spoke "other" bucket.
	if v := got[[2]string{"other", "hyp0-spoke1"}]; v != 1 {
		t.Errorf("hyp0-spoke1 other = %v, want 1", v)
	}

	// Gapless: every canonical phase is emitted for a spoke that has any gateway,
	// even at zero (hyp0-spoke0 has no Failed).
	for _, phase := range gatewayhealth.PhaseStrings() {
		if _, ok := got[[2]string{phase, "hyp0-spoke0"}]; !ok {
			t.Errorf("canonical phase %q missing for hyp0-spoke0", phase)
		}
	}
	if v := got[[2]string{"Failed", "hyp0-spoke0"}]; v != 0 {
		t.Errorf("hyp0-spoke0 Failed = %v, want 0", v)
	}

	// Sum across all managed_cluster values equals the fleet total (6 gateways).
	var sum float64
	for _, v := range got {
		sum += v
	}
	if sum != 6 {
		t.Errorf("sum over managed_cluster = %v, want 6 (fleet total)", sum)
	}
}

func TestActiveSandboxesCollector_AttributesByManagedCluster(t *testing.T) {
	dao := NewMockGatewayDao()
	ctx := context.Background()
	_, _ = dao.Create(ctx, &Gateway{ClusterId: "hyp0-spoke0", Namespace: "a", ActiveSandboxCount: ptr(3)})
	_, _ = dao.Create(ctx, &Gateway{ClusterId: "hyp0-spoke0", Namespace: "b", ActiveSandboxCount: ptr(1)})
	_, _ = dao.Create(ctx, &Gateway{ClusterId: "hyp0-spoke1", Namespace: "c", ActiveSandboxCount: ptr(1)})
	_, _ = dao.Create(ctx, &Gateway{ClusterId: "", Namespace: "d", ActiveSandboxCount: ptr(2)})

	got := gatherGauge(t, newGatewayActiveSandboxesCollector(dao), "hypershell_gateways_active_sandboxes_total", "managed_cluster")

	if v := got[[2]string{"hyp0-spoke0"}]; v != 4 {
		t.Errorf("hyp0-spoke0 sandboxes = %v, want 4", v)
	}
	if v := got[[2]string{"hyp0-spoke1"}]; v != 1 {
		t.Errorf("hyp0-spoke1 sandboxes = %v, want 1", v)
	}
	if v := got[[2]string{"unknown"}]; v != 2 {
		t.Errorf("unknown sandboxes = %v, want 2", v)
	}

	var sum float64
	for _, v := range got {
		sum += v
	}
	if sum != 7 {
		t.Errorf("sum over managed_cluster = %v, want 7 (fleet total)", sum)
	}
}
