package sources

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// readFrames reads SSE "data:" frames from an open response body and sends each
// decoded frame to out until the body closes.
func readFrames(t *testing.T, body *bufio.Reader, out chan<- streamFrame) {
	for {
		line, err := body.ReadString('\n')
		if err != nil {
			close(out)
			return
		}
		line = strings.TrimRight(line, "\n")
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var f streamFrame
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &f); err != nil {
			t.Errorf("decode frame %q: %v", line, err)
			continue
		}
		out <- f
	}
}

// awaitFrame reads frames until pred matches or the deadline elapses.
func awaitFrame(t *testing.T, frames <-chan streamFrame, what string, pred func(streamFrame) bool) streamFrame {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatalf("stream closed before %s", what)
			}
			if pred(f) {
				return f
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func startStream(t *testing.T, e *E2E, id string) (<-chan streamFrame, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(e.Stream))
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/e2e/stream?run="+id, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	frames := make(chan streamFrame, 64)
	go readFrames(t, bufio.NewReader(resp.Body), frames)
	cleanup := func() {
		cancel()
		_ = resp.Body.Close()
		srv.Close()
	}
	return frames, cleanup
}

func TestStreamBackfillThenLiveWithDedup(t *testing.T) {
	loki := &fakeLoki{history: map[string][]LokiEntry{
		"hub-00": {{Line: viaQLine(t, ev(e2eEventStepStarted, 2, map[string]any{"stepId": "P1.1", "name": "provisioning"}))}},
	}}
	e := testE2E(loki)
	tail := loki.tailChan("hub-00")

	frames, cleanup := startStream(t, e, makeRunID("hub-00", "hypershell", "run-1"))
	defer cleanup()

	// Backfill delivers the history event (seq 2) before live.
	awaitFrame(t, frames, "backfill event seq2", func(f streamFrame) bool {
		return f.Kind == "event" && f.Event != nil && f.Event.Sequence == 2
	})
	awaitFrame(t, frames, "live status", func(f streamFrame) bool {
		return f.Kind == "status" && f.Status != nil && f.Status.State == streamStateLive
	})

	// A live tail event (seq 3) is forwarded.
	tail <- TailMessage{Entries: []LokiEntry{{Line: viaQLine(t, ev(e2eEventStepFinished, 3, map[string]any{"stepId": "P1.1", "name": "provisioning", "outcome": "passed"}))}}}
	awaitFrame(t, frames, "live event seq3", func(f streamFrame) bool {
		return f.Kind == "event" && f.Event != nil && f.Event.Sequence == 3 && f.Event.Outcome == "passed"
	})

	// A duplicate of seq 2 (reconnect overlap) must NOT be re-delivered: the next
	// genuinely new event is seq 4.
	tail <- TailMessage{Entries: []LokiEntry{
		{Line: viaQLine(t, ev(e2eEventStepStarted, 2, map[string]any{"stepId": "P1.1", "name": "provisioning"}))}, // dup
		{Line: viaQLine(t, ev(e2eEventRunFinished, 4, map[string]any{"outcome": "passed"}))},
	}}
	f := awaitFrame(t, frames, "event after dup", func(f streamFrame) bool {
		return f.Kind == "event" && f.Event != nil
	})
	if f.Event.Sequence != 4 {
		t.Fatalf("dedup failed: next event seq = %d, want 4 (seq 2 should not repeat)", f.Event.Sequence)
	}
}

func TestStreamSurfacesDroppedEntries(t *testing.T) {
	loki := &fakeLoki{history: map[string][]LokiEntry{"hub-00": nil}}
	e := testE2E(loki)
	tail := loki.tailChan("hub-00")

	frames, cleanup := startStream(t, e, makeRunID("hub-00", "hypershell", "run-1"))
	defer cleanup()
	awaitFrame(t, frames, "live status", func(f streamFrame) bool {
		return f.Kind == "status" && f.Status != nil && f.Status.State == streamStateLive
	})

	tail <- TailMessage{DroppedEntries: 5}
	gap := awaitFrame(t, frames, "gap status", func(f streamFrame) bool {
		return f.Kind == "status" && f.Status != nil && f.Status.State == streamStateGap
	})
	if gap.Status.DroppedEntries != 5 {
		t.Errorf("dropped = %d, want 5", gap.Status.DroppedEntries)
	}
}

func TestStreamDisconnectWhenUpstreamCloses(t *testing.T) {
	loki := &fakeLoki{history: map[string][]LokiEntry{"hub-00": nil}}
	e := testE2E(loki)
	tail := loki.tailChan("hub-00")

	frames, cleanup := startStream(t, e, makeRunID("hub-00", "hypershell", "run-1"))
	defer cleanup()
	awaitFrame(t, frames, "live status", func(f streamFrame) bool {
		return f.Kind == "status" && f.Status != nil && f.Status.State == streamStateLive
	})

	close(tail) // upstream disconnect
	awaitFrame(t, frames, "disconnected status", func(f streamFrame) bool {
		return f.Kind == "status" && f.Status != nil && f.Status.State == streamStateDisconnected
	})
}

func TestStreamSharesUpstreamWithinReplica(t *testing.T) {
	loki := &fakeLoki{history: map[string][]LokiEntry{"hub-00": nil}}
	e := testE2E(loki)
	_ = loki.tailChan("hub-00")

	f1, c1 := startStream(t, e, makeRunID("hub-00", "hypershell", "run-1"))
	defer c1()
	awaitFrame(t, f1, "sub1 live", func(f streamFrame) bool { return f.Kind == "status" && f.Status != nil })

	f2, c2 := startStream(t, e, makeRunID("hub-00", "hypershell", "run-2"))
	defer c2()
	awaitFrame(t, f2, "sub2 live", func(f streamFrame) bool { return f.Kind == "status" && f.Status != nil })

	if got := loki.calls(); got != 1 {
		t.Errorf("upstream tail opened %d times, want 1 shared tail for the tenant", got)
	}
}

func TestTailBrokerCancelsContextWhenUpstreamClosesItself(t *testing.T) {
	// When the upstream tail ends on its own (a flaky or closed endpoint), the pump
	// cleanup must cancel the tail's context; otherwise the unsubscribe path can no
	// longer reach the cancel (the tenant entry is already gone) and each
	// self-terminating tail leaks one goroutine and one context per reconnect.
	loki := &fakeLoki{}
	ch := loki.tailChan("hub-00")
	broker := newTailBroker(loki, `{}`, slog.Default())

	sub, unsub, err := broker.Subscribe("hub-00")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsub()

	tailCtx := loki.tailContext("hub-00")
	if tailCtx == nil {
		t.Fatal("tail context not captured")
	}

	close(ch) // upstream ends on its own

	// The subscriber channel is closed by the pump as it tears down.
	select {
	case _, ok := <-sub:
		for ok {
			_, ok = <-sub
		}
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber channel not closed after upstream close")
	}

	select {
	case <-tailCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("tail context not cancelled after upstream close (goroutine/context leak)")
	}
}

func TestStreamRejectsBadRunID(t *testing.T) {
	e := testE2E(&fakeLoki{})
	srv := httptest.NewServer(http.HandlerFunc(e.Stream))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/e2e/stream?run=not-a-valid-id")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a malformed run id", resp.StatusCode)
	}
}
