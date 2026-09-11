package supervisor

import (
	"context"
	"log"
	"time"
)

// RestartCap bounds the backoff between restarts of a supervised background component.
const RestartCap = 30 * time.Second

// Run runs fn in a loop so a failure in one background component
// cannot take down the others by exiting the whole process. Every fn here is
// expected to block until ctx is done and return ctx.Err() at that point;
// a return before then is treated as a failure of that component alone; it is retried
// with a capped exponential backoff.
func Run(ctx context.Context, name string, fn func(context.Context) error) {
	RunWithBackoff(ctx, name, time.Second, fn)
}

// RunWithBackoff runs fn in a loop with a customizable initial backoff.
func RunWithBackoff(ctx context.Context, name string, initialBackoff time.Duration, fn func(context.Context) error) {
	backoff := initialBackoff
	for {
		err := fn(ctx)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			log.Printf("WARN %s exited unexpectedly with no error, restarting in %s", name, backoff)
		} else {
			log.Printf("WARN %s exited with error, restarting in %s: %v", name, backoff, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > RestartCap {
			backoff = RestartCap
		}
	}
}
