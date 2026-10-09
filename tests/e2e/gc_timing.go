package e2e

import (
	"context"
	"fmt"
	"time"
)

// namespaceGCTimingDriver is the narrow part of an infra driver that changes
// the controller-wide namespace garbage-collection timing.
type namespaceGCTimingDriver interface {
	ConfigureNamespaceGCTiming(context.Context, time.Duration, time.Duration) error
	RestoreNamespaceGCTiming(context.Context) error
}

// namespaceGCTimingLease owns one controller-wide GC timing override. Matrix
// runs share a driver and controller, so the matrix acquires one lease before
// its parallel suites start and releases it only after they have all completed.
type namespaceGCTimingLease struct {
	driver namespaceGCTimingDriver
}

func acquireNamespaceGCTiming(ctx context.Context, d namespaceGCTimingDriver, interval, grace time.Duration) (*namespaceGCTimingLease, error) {
	if err := d.ConfigureNamespaceGCTiming(ctx, interval, grace); err != nil {
		return nil, fmt.Errorf("configure namespace GC timing: %w", err)
	}
	return &namespaceGCTimingLease{driver: d}, nil
}

func (l *namespaceGCTimingLease) Release(ctx context.Context) error {
	if err := l.driver.RestoreNamespaceGCTiming(ctx); err != nil {
		return fmt.Errorf("restore namespace GC timing: %w", err)
	}
	return nil
}
