package watcher

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWatchLoopGracefulEOF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := watchLoop(ctx, "Test", func(_ context.Context) error {
		calls++
		if calls >= 2 {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if calls < 2 {
		t.Errorf("expected at least 2 calls (reconnect after EOF), got %d", calls)
	}
}

func TestWatchLoopDisconnectError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	watchErr := errors.New("connection reset")
	err := watchLoop(ctx, "Test", func(_ context.Context) error {
		calls++
		if calls >= 2 {
			cancel()
		}
		return watchErr
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if calls < 2 {
		t.Errorf("expected at least 2 calls (reconnect after error), got %d", calls)
	}
}

func TestWatchLoopParentCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := watchLoop(ctx, "Test", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}
