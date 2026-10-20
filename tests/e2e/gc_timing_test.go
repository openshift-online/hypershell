package e2e

import (
	"context"
	"testing"
	"time"
)

type fakeGCTimingDriver struct {
	configureCalls int
	restoreCalls   int
	interval       time.Duration
	grace          time.Duration
}

func (d *fakeGCTimingDriver) ConfigureNamespaceGCTiming(_ context.Context, interval, grace time.Duration) error {
	d.configureCalls++
	d.interval = interval
	d.grace = grace
	return nil
}

func (d *fakeGCTimingDriver) RestoreNamespaceGCTiming(context.Context) error {
	d.restoreCalls++
	return nil
}

func TestNamespaceGCTimingLeaseConfiguresAndRestoresOnce(t *testing.T) {
	d := &fakeGCTimingDriver{}
	lease, err := acquireNamespaceGCTiming(context.Background(), d, 30*time.Second, 15*time.Second)
	if err != nil {
		t.Fatalf("acquireNamespaceGCTiming() error = %v", err)
	}
	if d.configureCalls != 1 || d.interval != 30*time.Second || d.grace != 15*time.Second {
		t.Fatalf("configure calls = %d, interval = %s, grace = %s; want 1, 30s, 15s", d.configureCalls, d.interval, d.grace)
	}

	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if d.restoreCalls != 1 {
		t.Fatalf("restore calls = %d, want 1", d.restoreCalls)
	}
}
