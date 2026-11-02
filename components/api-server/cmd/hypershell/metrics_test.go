package main

import (
	"testing"
	"time"

	"github.com/openshift-online/rh-trex-ai/components/api-server/pkg/db"
	"github.com/prometheus/client_golang/prometheus"
)

// After registration, a recorded advisory lock shows up in the default
// registry that /metrics serves, and registering twice is not an error.
//
// Verifies: API-OBS-10.01
func TestRegisterAdvisoryLockMetricsExposesFrameworkMetrics(t *testing.T) {
	t.Cleanup(db.UnregisterAdvisoryLockMetrics)

	if err := registerAdvisoryLockMetrics(); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if err := registerAdvisoryLockMetrics(); err != nil {
		t.Fatalf("second registration: %v", err)
	}

	db.UpdateAdvisoryLockCountMetric(db.Events, "OK")
	db.UpdateAdvisoryLockDurationMetric(db.Events, "OK", time.Now())

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	want := map[string]bool{"advisory_lock_count": false, "advisory_lock_duration": false}
	for _, family := range families {
		if _, ok := want[family.GetName()]; ok {
			want[family.GetName()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("%s is not exposed by the default registry", name)
		}
	}
}
