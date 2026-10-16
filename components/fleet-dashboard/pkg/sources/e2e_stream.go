package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// This file implements the live side of the e2e-run plane
// (e2e-run-log-visibility.spec E2E-LOG-05/06): a Server-Sent Events handler that
// bridges the initial snapshot to a Loki tail without losing events, and a
// per-tenant tail broker that shares a bounded upstream feed across subscribed
// browsers within one replica.

// streamHeartbeat is the interval between heartbeat frames, so a browser and any
// intermediary can tell a quiet-but-live feed from a dead one.
const streamHeartbeat = 15 * time.Second

// streamFrame is one SSE payload. Exactly one of Event or Status is set, except
// for a heartbeat (Kind == "heartbeat") which carries neither.
type streamFrame struct {
	Kind   string           `json:"kind"` // "event" | "status" | "heartbeat"
	Event  *E2EStreamEvent  `json:"event,omitempty"`
	Status *E2EStreamStatus `json:"status,omitempty"`
}

// E2EStreamEvent is a forwarded structured suite event. The UI reduces these
// (ordering, dedup by runId/sequence, step-state derivation) in its framework-free
// domain (E2E-LOG-11), so the BFF forwards events rather than a re-reduced view.
type E2EStreamEvent struct {
	Event        string       `json:"event"`
	RunID        string       `json:"runId"`
	Sequence     int64        `json:"sequence"`
	Timestamp    string       `json:"timestamp"`
	StepID       string       `json:"stepId,omitempty"`
	Name         string       `json:"name,omitempty"`
	Outcome      string       `json:"outcome,omitempty"`
	DurationMs   *int64       `json:"durationMs,omitempty"`
	Message      string       `json:"message,omitempty"`
	Truncated    bool         `json:"truncated,omitempty"`
	PlannedSteps []E2EPlanned `json:"plannedSteps,omitempty"`
}

// E2EStreamStatus reports the live connection state so the UI never renders a
// disconnected feed as healthy (E2E-LOG-06/10). droppedEntries surfaces a Loki
// drop or a slow-consumer backpressure drop.
type E2EStreamStatus struct {
	State          string `json:"state"` // "live" | "disconnected" | "gap"
	DroppedEntries int    `json:"droppedEntries,omitempty"`
}

const (
	streamStateLive         = "live"
	streamStateDisconnected = "disconnected"
	streamStateGap          = "gap"
)

// Stream is the GET /api/e2e/stream handler. It addresses one run by the "run"
// query parameter (the collision-free id), derives the trusted tenant from that
// id, replays the run's recent events as the snapshot/backfill, then follows the
// shared Loki tail. Dedup by sequence within the connection, combined with the
// UI's own dedup across reconnects, means no event is delivered twice or lost
// across the snapshot-to-tail bridge (E2E-LOG-06).
func (e *E2E) Stream(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("run")
	tenant, namespace, runID, err := parseRunID(id)
	if err != nil {
		http.Error(w, "invalid run id", http.StatusBadRequest)
		return
	}
	if !e.tenantAllowed(tenant) {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	rc := http.NewResponseController(w)
	ctx := r.Context()

	var writeErr error
	send := func(frame streamFrame) {
		if writeErr != nil {
			return
		}
		b, err := json.Marshal(frame)
		if err != nil {
			writeErr = err
			return
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			writeErr = err
			return
		}
		if err := rc.Flush(); err != nil {
			writeErr = err
		}
	}

	seen := map[int64]bool{}
	forward := func(ev e2eEvent) {
		if ev.RunID != runID || (ev.Namespace != "" && ev.Namespace != namespace) {
			return
		}
		if seen[ev.Sequence] {
			return
		}
		seen[ev.Sequence] = true
		send(streamFrame{Kind: "event", Event: toStreamEvent(ev)})
	}

	// Backfill: replay the run's recent events from history before going live, so
	// the bridge has no gap between the snapshot and the tail.
	end := time.Now()
	start := end.Add(-e.window)
	if entries, _, err := e.loki.QueryRange(ctx, tenant, e.selector, start, end, lokiQueryLimit); err == nil {
		events := parseAndFilter(entries, runID, namespace)
		for _, ev := range events {
			forward(ev)
		}
	} else {
		e.logger.WarnContext(ctx, "e2e stream backfill query failed", "tenant", tenant, "err", err)
	}
	send(streamFrame{Kind: "status", Status: &E2EStreamStatus{State: streamStateLive}})
	if writeErr != nil {
		return
	}

	sub, unsub, err := e.broker.Subscribe(tenant)
	if err != nil {
		send(streamFrame{Kind: "status", Status: &E2EStreamStatus{State: streamStateDisconnected}})
		return
	}
	defer unsub()

	ticker := time.NewTicker(streamHeartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// The request context ended: the client went away, or the fronting
			// oauth-proxy tore down the connection when the session expired (the
			// backend auth middleware authorizes once at request start; in-band
			// revalidation of a long-lived SSE is the proxy's connection teardown,
			// surfaced here as ctx cancellation, E2E-LOG-08). End the stream.
			return
		case <-ticker.C:
			send(streamFrame{Kind: "heartbeat"})
			if writeErr != nil {
				return
			}
		case msg, ok := <-sub:
			if !ok {
				// Upstream tail closed: a disconnect, never a healthy idle feed.
				send(streamFrame{Kind: "status", Status: &E2EStreamStatus{State: streamStateDisconnected}})
				return
			}
			if msg.DroppedEntries > 0 {
				send(streamFrame{Kind: "status", Status: &E2EStreamStatus{State: streamStateGap, DroppedEntries: msg.DroppedEntries}})
			}
			for _, ev := range parseAndFilter(msg.Entries, runID, namespace) {
				forward(ev)
			}
			if writeErr != nil {
				return
			}
		}
	}
}

// parseAndFilter parses each entry's ViaQ envelope + message and returns the
// well-formed structured events for one run, ordered and deduplicated by
// sequence. Malformed records are skipped without dropping the rest.
func parseAndFilter(entries []LokiEntry, runID, namespace string) []e2eEvent {
	var events []e2eEvent
	for _, entry := range entries {
		ev, ok := parseViaQEvent(entry.Line)
		if !ok {
			continue
		}
		if ev.RunID != runID {
			continue
		}
		if ev.Namespace != "" && namespace != "" && ev.Namespace != namespace {
			continue
		}
		events = append(events, ev)
	}
	return dedupeBySequence(events)
}

func toStreamEvent(ev e2eEvent) *E2EStreamEvent {
	return &E2EStreamEvent{
		Event:        ev.Event,
		RunID:        ev.RunID,
		Sequence:     ev.Sequence,
		Timestamp:    ev.Timestamp,
		StepID:       ev.StepID,
		Name:         ev.Name,
		Outcome:      ev.Outcome,
		DurationMs:   ev.DurationMs,
		Message:      ev.Message,
		Truncated:    ev.Truncated,
		PlannedSteps: ev.PlannedSteps,
	}
}

// tailBroker shares one upstream Loki tail per tenant across subscribed browsers
// within this replica (E2E-LOG-05): the first subscriber for a tenant opens the
// upstream tail, later subscribers attach to the same feed, and the upstream is
// closed when the last subscriber leaves.
type tailBroker struct {
	loki     LokiClient
	selector string
	logger   *slog.Logger

	mu      sync.Mutex
	tenants map[string]*tenantTail
	nextID  int
}

func newTailBroker(loki LokiClient, selector string, logger *slog.Logger) *tailBroker {
	return &tailBroker{
		loki:     loki,
		selector: selector,
		logger:   logger,
		tenants:  map[string]*tenantTail{},
	}
}

// tenantTail is one shared upstream tail and its subscribers.
type tenantTail struct {
	cancel context.CancelFunc
	subs   map[int]*subscriber
}

// subscriber is one browser's bounded delivery channel. pendingDropped counts
// messages dropped under backpressure between successful sends; it is touched
// only by the pump goroutine (under the broker mutex), never by the reader.
type subscriber struct {
	ch             chan TailMessage
	pendingDropped int
	closeOnce      sync.Once
}

func (s *subscriber) close() { s.closeOnce.Do(func() { close(s.ch) }) }

// Subscribe attaches a new subscriber to the tenant's shared tail, opening the
// upstream on the first subscriber. It returns the subscriber channel and an
// unsubscribe func that detaches it and tears down the upstream when it was the
// last. The upstream runs on the broker's own context, not the caller's, so one
// browser leaving does not end the shared feed for the others.
//
// The upstream Loki dial is performed OUTSIDE the broker mutex: the dial is a
// blocking, bounded-but-not-instant network operation, and holding the mutex
// across it would let a slow or black-holed Loki endpoint stall every other
// subscriber, pump, and unsubscribe on the replica. A double-check after dialing
// attaches to a tenant tail a racing subscriber may have created meanwhile.
func (b *tailBroker) Subscribe(tenant string) (<-chan TailMessage, func(), error) {
	b.mu.Lock()
	if tt, ok := b.tenants[tenant]; ok {
		id, sub := b.attachLocked(tt)
		b.mu.Unlock()
		return sub.ch, b.unsubFunc(tenant, id, sub), nil
	}
	b.mu.Unlock()

	// First subscriber for this tenant: dial the upstream off the lock.
	upCtx, cancel := context.WithCancel(context.Background())
	up, err := b.loki.Tail(upCtx, tenant, b.selector)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	b.mu.Lock()
	if tt, ok := b.tenants[tenant]; ok {
		// A concurrent Subscribe created the tenant tail while we dialed; discard
		// ours and attach to theirs.
		id, sub := b.attachLocked(tt)
		b.mu.Unlock()
		cancel()
		return sub.ch, b.unsubFunc(tenant, id, sub), nil
	}
	tt := &tenantTail{cancel: cancel, subs: map[int]*subscriber{}}
	b.tenants[tenant] = tt
	go b.pump(tenant, tt, up)
	id, sub := b.attachLocked(tt)
	b.mu.Unlock()
	return sub.ch, b.unsubFunc(tenant, id, sub), nil
}

// attachLocked registers a new subscriber on a tenant tail. The caller holds b.mu.
func (b *tailBroker) attachLocked(tt *tenantTail) (int, *subscriber) {
	id := b.nextID
	b.nextID++
	sub := &subscriber{ch: make(chan TailMessage, tailChannelBuffer)}
	tt.subs[id] = sub
	return id, sub
}

// unsubFunc returns the idempotent unsubscribe for one subscriber: it detaches
// the subscriber and tears down the upstream when it was the last.
func (b *tailBroker) unsubFunc(tenant string, id int, sub *subscriber) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			if cur, ok := b.tenants[tenant]; ok {
				if _, ok := cur.subs[id]; ok {
					delete(cur.subs, id)
					if len(cur.subs) == 0 {
						cur.cancel()
						delete(b.tenants, tenant)
					}
				}
			}
			b.mu.Unlock()
			sub.close()
		})
	}
}

// pump reads the shared upstream tail and fans each message out to every current
// subscriber with a non-blocking send, so one slow browser cannot stall the feed
// for the others. A dropped message is counted and surfaced on the next
// successful send (E2E-LOG-06). When the upstream closes, every subscriber
// channel is closed so each handler reports the disconnect.
func (b *tailBroker) pump(tenant string, tt *tenantTail, up <-chan TailMessage) {
	for msg := range up {
		b.mu.Lock()
		for _, s := range tt.subs {
			m := msg
			if s.pendingDropped > 0 {
				m.DroppedEntries += s.pendingDropped
			}
			select {
			case s.ch <- m:
				s.pendingDropped = 0
			default:
				s.pendingDropped += len(msg.Entries)
				if s.pendingDropped == 0 {
					s.pendingDropped = 1 // a zero-entry drop is still a gap
				}
			}
		}
		b.mu.Unlock()
	}
	// Upstream ended: tear down and signal every subscriber.
	b.mu.Lock()
	for _, s := range tt.subs {
		s.close()
	}
	if cur, ok := b.tenants[tenant]; ok && cur == tt {
		delete(b.tenants, tenant)
	}
	b.mu.Unlock()
}
