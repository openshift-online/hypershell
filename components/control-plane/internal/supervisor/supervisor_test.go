package supervisor

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	fn := func(c context.Context) error {
		calls++
		if calls == 1 {
			return nil // Return nil on first call, should restart
		}
		if calls == 2 {
			return errors.New("some error") // Return error on second call, should restart
		}
		// On third call, cancel context to break the loop
		cancel()
		return c.Err()
	}

	RunWithBackoff(ctx, "test-component", time.Millisecond, fn)

	if calls != 3 {
		t.Errorf("expected fn to be called 3 times, got %d", calls)
	}
}
