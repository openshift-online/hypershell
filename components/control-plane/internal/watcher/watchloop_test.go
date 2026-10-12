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
	err := watchLoop(ctx, "Test", 0, func(_ context.Context, _ func()) error {
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
	err := watchLoop(ctx, "Test", 0, func(_ context.Context, _ func()) error {
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

// A stream that delivers neither an event nor an error (never heartbeats) must be
// cancelled by the idle watchdog so watchLoop reconnects, rather than block
// forever. This is the silent-stream failure mode from HYPERSHELL issue #504.
func TestWatchLoopIdleWatchdogReconnects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := watchLoop(ctx, "Test", 20*time.Millisecond, func(streamCtx context.Context, _ func()) error {
		calls++
		if calls >= 2 {
			// The watchdog proved it reconnects; end the loop.
			cancel()
			return nil
		}
		// Never heartbeat: the watchdog must cancel this attempt.
		<-streamCtx.Done()
		return streamCtx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if calls < 2 {
		t.Errorf("expected the idle watchdog to force a reconnect, got %d call(s)", calls)
	}
}

// A stream that keeps heartbeating faster than the idle timeout must NOT be torn
// down: an ordinarily quiet-but-healthy stream stays connected.
func TestWatchLoopHeartbeatKeepsStreamAlive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := watchLoop(ctx, "Test", 50*time.Millisecond, func(streamCtx context.Context, heartbeat func()) error {
		calls++
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		beats := 0
		for {
			select {
			case <-streamCtx.Done():
				// A watchdog fire here would be a spurious cancellation.
				return streamCtx.Err()
			case <-ticker.C:
				heartbeat()
				beats++
				if beats >= 10 { // 100ms of beats, well past the 50ms idle timeout
					cancel()
					return nil
				}
			}
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Errorf("heartbeats should keep the stream alive without a reconnect, got %d call(s)", calls)
	}
}

func TestWatchLoopParentCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := watchLoop(ctx, "Test", 0, func(ctx context.Context, _ func()) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}
