package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitDisruptiveGateOpensWhenFileAppears(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "disruptive-gate")
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = os.WriteFile(gate, nil, 0o600)
	}()

	opened, err := waitDisruptiveGate(t.Context(), gate, 5*time.Second, 10*time.Millisecond)
	if err != nil || !opened {
		t.Fatalf("waitDisruptiveGate() = %v, %v; want true, nil", opened, err)
	}
}

func TestWaitDisruptiveGateTimesOutWithoutError(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "never")

	opened, err := waitDisruptiveGate(t.Context(), gate, 30*time.Millisecond, 10*time.Millisecond)
	if err != nil || opened {
		t.Fatalf("waitDisruptiveGate() = %v, %v; want false, nil", opened, err)
	}
}

func TestWaitDisruptiveGateStopsOnCancel(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "never")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := waitDisruptiveGate(ctx, gate, time.Minute, 10*time.Millisecond); err == nil {
		t.Fatal("waitDisruptiveGate() with a cancelled context returned no error")
	}
}
