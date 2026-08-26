package watcher

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/util/workqueue"
)

// recordingHandler records every payload it is invoked with and can fail the
// first failUntil calls, so tests can drive retry, coalescing, and serialization
// behavior. An optional block channel lets a test hold a call in-flight.
type recordingHandler struct {
	mu        sync.Mutex
	calls     int
	failUntil int
	seen      []string
	inFlight  int
	maxInFl   int
	enter     chan struct{}
	release   chan struct{}
	onCall    func() // invoked inside Handle, e.g. to simulate a self-status event
}

func (h *recordingHandler) Handle(_ context.Context, ev Event[string]) error {
	h.mu.Lock()
	h.calls++
	h.inFlight++
	if h.inFlight > h.maxInFl {
		h.maxInFl = h.inFlight
	}
	h.seen = append(h.seen, ev.Resource)
	fail := h.calls <= h.failUntil
	enter, release, onCall := h.enter, h.release, h.onCall
	h.mu.Unlock()

	if onCall != nil {
		onCall()
	}

	if enter != nil {
		enter <- struct{}{}
		<-release
	}

	h.mu.Lock()
	h.inFlight--
	h.mu.Unlock()

	if fail {
		return errors.New("boom")
	}
	return nil
}

func (h *recordingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

func (h *recordingHandler) lastSeen() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.seen) == 0 {
		return ""
	}
	return h.seen[len(h.seen)-1]
}

func (h *recordingHandler) maxConcurrent() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.maxInFl
}

// waitForCount polls until the handler has been invoked want times, failing the
// test if that does not happen within the deadline.
func waitForCount(t *testing.T, h *recordingHandler, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if h.count() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("handler called %d times, want >= %d", h.count(), want)
}

// fastLimiter is a short-backoff rate limiter so retry tests run quickly.
func fastLimiter() workqueue.TypedRateLimiter[string] {
	return workqueue.NewTypedItemExponentialFailureRateLimiter[string](time.Millisecond, 5*time.Millisecond)
}

// A reconcile that fails transiently must be retried until it succeeds, then stop
// -- this is the durable recovery the gateway watch stream cannot provide itself.
func TestReconcileQueue_RetriesUntilSuccess(t *testing.T) {
	h := &recordingHandler{failUntil: 2}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1))
	defer q.stop()

	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "v1"})

	waitForCount(t, h, 3) // 2 failures + 1 success
	time.Sleep(30 * time.Millisecond)
	if got := h.count(); got != 3 {
		t.Fatalf("handler called %d times, want exactly 3 (stops after success)", got)
	}
	if got := q.queue.NumRequeues("gw-1"); got != 0 {
		t.Fatalf("NumRequeues = %d, want 0 after success (Forget)", got)
	}
}

// A reconcile that keeps failing must be retried indefinitely (capped backoff),
// never abandoned after a fixed budget -- the watch stream does not replay state,
// so giving up would re-strand the gateway. This is the key fix over the previous
// finite-attempt requeuer.
func TestReconcileQueue_RetriesIndefinitely(t *testing.T) {
	h := &recordingHandler{failUntil: 1 << 30}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1))
	defer q.stop()

	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "v1"})

	// Far more than the old 8-attempt budget: proves retries do not stop.
	waitForCount(t, h, 20)
}

func TestReconcileQueue_RetryPolicyDropsRejectedError(t *testing.T) {
	h := &recordingHandler{failUntil: 1 << 30}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1),
		withRetryIf[string](func(error) bool { return false }))
	defer q.stop()

	q.enqueue(Event[string]{ResourceID: "binding-1", Resource: "v1"})
	waitForCount(t, h, 1)
	time.Sleep(30 * time.Millisecond)
	if got := h.count(); got != 1 {
		t.Fatalf("handler called %d times, want 1", got)
	}

	q.enqueue(Event[string]{ResourceID: "binding-1", Resource: "v2"})
	waitForCount(t, h, 2)
}

func TestReconcileQueue_StopsAtRetryLimit(t *testing.T) {
	h := &recordingHandler{failUntil: 1 << 30}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1), withMaxRetries[string](2))
	defer q.stop()

	q.enqueue(Event[string]{ResourceID: "binding-1", Resource: "v1"})
	waitForCount(t, h, 3)
	time.Sleep(30 * time.Millisecond)
	if got := h.count(); got != 3 {
		t.Fatalf("handler called %d times, want 3", got)
	}
}

// The reconciler's own phase-status writes emit watch events that re-enqueue (and
// mark dirty) the key while it is being processed. client-go would then re-queue
// it for immediate handling on Done, bypassing the retry backoff and spinning --
// re-hammering the API server and Keycloak. The backoff floor must survive those
// dirty re-adds: a persistently failing key that re-enqueues itself on every call
// must still be handled at the backoff cadence, not in a tight loop.
func TestReconcileQueue_PreservesBackoffAgainstDirtyReadds(t *testing.T) {
	h := &recordingHandler{failUntil: 1 << 30}
	// base 25ms backoff; a spin would produce hundreds of calls in the window below.
	limiter := workqueue.NewTypedItemExponentialFailureRateLimiter[string](25*time.Millisecond, 50*time.Millisecond)
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](limiter), withWorkers[string](1))
	defer q.stop()
	// Each Handle re-enqueues the same key, simulating the reconciler's self-status
	// write marking the key dirty during processing.
	h.mu.Lock()
	h.onCall = func() { q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "v1"}) }
	h.mu.Unlock()

	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "v1"})

	time.Sleep(200 * time.Millisecond)
	// ~200ms / 25-50ms backoff => a handful of calls. Well under any spin, but more
	// than one (proves it still retries).
	got := h.count()
	if got < 2 {
		t.Fatalf("handler called %d times, want >= 2 (must keep retrying)", got)
	}
	if got > 15 {
		t.Fatalf("handler called %d times in 200ms: backoff bypassed by dirty re-adds (spin)", got)
	}
}

// Version-aware coalescing must keep the newest observed payload regardless of
// enqueue order: a stale seed/resync snapshot (lower version) must not clobber a
// newer live event, and a buffered out-of-order live event must not clobber a
// newer seed. This is what lets a one-shot seed coexist with live traffic.
func TestReconcileQueue_VersionCoalescingKeepsNewest(t *testing.T) {
	h := &recordingHandler{
		enter:   make(chan struct{}),
		release: make(chan struct{}),
	}
	version := func(ev Event[string]) int64 {
		v := map[string]int64{"old": 1, "new": 2}
		return v[ev.Resource]
	}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1),
		withVersion(version))
	defer q.stop()

	// Hold a first reconcile in-flight so later enqueues coalesce into latest.
	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "new"})
	<-h.enter
	// A stale snapshot (lower version) arrives while the newer payload is pending.
	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "old"})
	h.release <- struct{}{} // first reconcile ("new") completes

	// If a second reconcile runs at all, it must not be the stale "old" payload.
	select {
	case <-h.enter:
		got := h.lastSeen()
		h.release <- struct{}{}
		if got == "old" {
			t.Fatalf("reconciled stale payload %q; version coalescing must keep the newest", got)
		}
	case <-time.After(50 * time.Millisecond):
		// No second reconcile: the stale enqueue was dropped entirely. Correct.
	}
}

// A pending (not-yet-processed) delete is terminal: a non-delete event -- e.g. a
// stale seed snapshot or a buffered out-of-order live update -- must not overwrite
// it and resurrect a resource whose deletion has not been reconciled.
func TestReconcileQueue_PendingDeleteNotResurrected(t *testing.T) {
	h := &recordingHandler{
		enter:   make(chan struct{}),
		release: make(chan struct{}),
	}
	version := func(ev Event[string]) int64 {
		// The resurrecting update even claims a *higher* version, to prove the delete
		// wins on type, not on version.
		if ev.Resource == "resurrect" {
			return 100
		}
		return 1
	}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1),
		withVersion(version))
	defer q.stop()

	// Occupy the single worker so the delete stays pending in latest while the
	// resurrecting update is enqueued.
	q.enqueue(Event[string]{ResourceID: "blocker", Resource: "x"})
	<-h.enter

	q.enqueue(Event[string]{Type: EventDeleted, ResourceID: "gw-1", Resource: "gone"})
	q.enqueue(Event[string]{Type: EventUpdated, ResourceID: "gw-1", Resource: "resurrect"})

	// The pending payload for gw-1 must still be the delete.
	q.mu.Lock()
	got := q.latest["gw-1"]
	q.mu.Unlock()
	if got.Type != EventDeleted {
		t.Fatalf("gw-1 pending event = %v (%q), want the delete to stay terminal", got.Type, got.Resource)
	}

	h.release <- struct{}{} // release blocker
	// Drain any remaining processing so stop() is clean.
	go func() {
		for {
			select {
			case <-h.enter:
				h.release <- struct{}{}
			case <-time.After(100 * time.Millisecond):
				return
			}
		}
	}()
	time.Sleep(120 * time.Millisecond)
}

// A forced enqueue (a startup/reconnect seed of an active-phase gateway) must
// bypass the phase gate on its FIRST handler attempt, and that bypass must survive
// an equal-version live event overwriting the payload. The forced mark is tracked
// independently of the payload for exactly this reason: the seed clones the source
// with the same updated_at, so a same-version live event buffered during the LIST
// is not version-dropped and overwrites the payload -- yet the transform must
// still apply (NumRequeues is 0 on the first attempt, so only the forced mark can
// carry the bypass).
func TestReconcileQueue_ForcedBypassSurvivesEqualVersionOverwrite(t *testing.T) {
	h := &recordingHandler{
		enter:   make(chan struct{}),
		release: make(chan struct{}),
	}
	transform := func(ev Event[string]) Event[string] {
		ev.Resource = "forced:" + ev.Resource
		return ev
	}
	version := func(_ Event[string]) int64 { return 1 } // all payloads share a version

	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1),
		withRetryTransform(transform), withVersion(version))
	defer q.stop()

	// Occupy the single worker so the forced enqueue and the overwriting live event
	// both land while gw-1 is still pending.
	q.enqueue(Event[string]{ResourceID: "blocker", Resource: "x"})
	<-h.enter

	q.enqueueForced(Event[string]{ResourceID: "gw-1", Resource: "seed"})
	// An equal-version live event overwrites the forced payload. Version coalescing
	// does not drop it (not strictly older), so the payload becomes "live"; the
	// bypass must persist regardless.
	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "live"})

	h.release <- struct{}{} // release blocker; worker advances to gw-1
	<-h.enter
	got := h.lastSeen()
	h.release <- struct{}{}
	if got != "forced:live" {
		t.Fatalf("first attempt saw %q, want %q (forced bypass must survive an equal-version overwrite and apply on the first attempt)", got, "forced:live")
	}
}

// A retry must reconcile the latest observed payload, not the one that failed:
// coalescing to newest desired state is what prevents a stale (e.g. still-routed)
// payload from being replayed after the resource changed.
func TestReconcileQueue_CoalescesToLatestPayload(t *testing.T) {
	h := &recordingHandler{
		failUntil: 1, // fail the first attempt so a retry happens
		enter:     make(chan struct{}),
		release:   make(chan struct{}),
	}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1))
	defer q.stop()

	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "v1"})

	<-h.enter // first attempt is in-flight with "v1"
	// A newer event arrives while the first attempt runs.
	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "v2"})
	h.release <- struct{}{} // first attempt fails -> requeued

	<-h.enter // retry is in-flight; it must carry the latest payload
	got := h.lastSeen()
	h.release <- struct{}{}
	if got != "v2" {
		t.Fatalf("retry reconciled %q, want the latest payload %q", got, "v2")
	}
}

// The retry transform must be applied only to retries, never to the first attempt,
// so ordinary create/update traffic is reconciled verbatim while recovery gets the
// adjusted (e.g. phase-cleared) payload.
func TestReconcileQueue_RetryTransformOnlyOnRetry(t *testing.T) {
	h := &recordingHandler{failUntil: 1} // first attempt fails, forcing one retry
	transform := func(ev Event[string]) Event[string] {
		ev.Resource = "transformed"
		return ev
	}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1),
		withRetryTransform(transform))
	defer q.stop()

	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "original"})

	waitForCount(t, h, 2)
	time.Sleep(20 * time.Millisecond)

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.seen) < 2 {
		t.Fatalf("want at least 2 calls, got %d", len(h.seen))
	}
	if h.seen[0] != "original" {
		t.Fatalf("first attempt saw %q, want the untransformed %q", h.seen[0], "original")
	}
	if h.seen[1] != "transformed" {
		t.Fatalf("retry saw %q, want the transformed payload", h.seen[1])
	}
}

// The queue must never run two reconciles for the same resource concurrently, so a
// retry can never race a live event for that resource.
func TestReconcileQueue_SerializesPerResource(t *testing.T) {
	h := &recordingHandler{
		enter:   make(chan struct{}),
		release: make(chan struct{}),
	}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](4))
	defer q.stop()

	// Two events for the same key; with 4 workers they could run concurrently if the
	// queue did not serialize per key.
	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "a"})
	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "b"})

	<-h.enter
	h.release <- struct{}{}
	// A second processing happens only if a newer payload coalesced in; drain it if
	// present so stop() is clean.
	select {
	case <-h.enter:
		h.release <- struct{}{}
	case <-time.After(50 * time.Millisecond):
	}

	if got := h.maxConcurrent(); got > 1 {
		t.Fatalf("max concurrent reconciles for one key = %d, want 1", got)
	}
}

func TestReconcileQueue_ProcessesDifferentResourcesInParallel(t *testing.T) {
	h := &recordingHandler{
		enter:   make(chan struct{}),
		release: make(chan struct{}),
	}
	q := newReconcileQueue(context.Background(), "Test", h, withWorkers[string](2))
	defer q.stop()

	q.enqueue(Event[string]{ResourceID: "binding-1", Resource: "slow"})
	<-h.enter
	q.enqueue(Event[string]{ResourceID: "binding-2", Resource: "ready"})

	select {
	case <-h.enter:
	case <-time.After(time.Second):
		t.Fatal("the second resource did not start while the first resource was blocked")
	}

	h.release <- struct{}{}
	h.release <- struct{}{}
}

// stop must drain the workers and return, after which no further reconciles run.
func TestReconcileQueue_StopDrainsWorkers(t *testing.T) {
	h := &recordingHandler{}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](2))

	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "v1"})
	waitForCount(t, h, 1)
	q.stop()

	before := h.count()
	q.enqueue(Event[string]{ResourceID: "gw-2", Resource: "v1"}) // ignored: queue shut down
	time.Sleep(20 * time.Millisecond)
	if got := h.count(); got != before {
		t.Fatalf("handler called %d times after stop, want %d (no work after shutdown)", got, before)
	}
}

// Canceling the watcher context must shut the queue down even without an explicit
// stop, so workers never leak past the watcher's lifetime.
func TestReconcileQueue_ContextCancelShutsDown(t *testing.T) {
	h := &recordingHandler{}
	ctx, cancel := context.WithCancel(context.Background())
	q := newReconcileQueue(ctx, "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](2))

	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "v1"})
	waitForCount(t, h, 1)

	cancel()
	// stop should return promptly because the context watcher already shut the queue
	// down; a leaked worker would hang this.
	done := make(chan struct{})
	go func() { q.stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop did not return after context cancel; workers leaked")
	}
}

// When a version-dropped forced seed is discarded, the force mark must propagate
// onto the retained newer payload regardless of that payload's phase -- the seed
// itself proves startup recovery was requested, and the retained phase is not
// authoritative (the independent GatewayHealthReconciler may have written a newer
// Running while the provisioning Handle is still failing). The retained payload
// here plays the role of a newer Running that a naive "phase clears force" rule
// would wrongly let strand the recovery. The key must also be (re)scheduled (Add)
// because the retained payload may already have been processed and dropped from
// the queue -- without the Add the force mark would be orphaned.
func TestReconcileQueue_ForcePropagatesBehindNewerRetainedPayload(t *testing.T) {
	h := &recordingHandler{
		enter:   make(chan struct{}),
		release: make(chan struct{}),
	}
	transform := func(ev Event[string]) Event[string] {
		ev.Resource = "forced:" + ev.Resource
		return ev
	}
	version := func(ev Event[string]) int64 {
		v := map[string]int64{"old-seed": 1, "newer-running": 2}
		return v[ev.Resource]
	}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1),
		withRetryTransform(transform), withVersion(version))
	defer q.stop()

	// Occupy the worker so coalescing is visible.
	q.enqueue(Event[string]{ResourceID: "blocker", Resource: "x"})
	<-h.enter

	// A newer Running-like payload arrives first.
	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "newer-running"})
	// An older forced seed is version-dropped but must still propagate force.
	q.enqueueForced(Event[string]{ResourceID: "gw-1", Resource: "old-seed"})

	h.release <- struct{}{} // release blocker; worker advances to gw-1
	<-h.enter
	got := h.lastSeen()
	h.release <- struct{}{}

	if got != "forced:newer-running" {
		t.Fatalf("first attempt saw %q, want %q (force must propagate onto the retained newer payload regardless of phase)", got, "forced:newer-running")
	}
}

// A strictly newer non-delete payload must NOT erase an existing force mark. The
// phase it carries (e.g. Running) is written independently by the health
// reconciler and does not prove the forced recovery's Handle succeeded; clearing
// force here could strand a failed reconcile behind the phase gate. The mark must
// survive and still transform the coalesced newer payload on the first attempt.
func TestReconcileQueue_NewerRunningDoesNotEraseForce(t *testing.T) {
	h := &recordingHandler{
		enter:   make(chan struct{}),
		release: make(chan struct{}),
	}
	transform := func(ev Event[string]) Event[string] {
		ev.Resource = "forced:" + ev.Resource
		return ev
	}
	version := func(ev Event[string]) int64 {
		v := map[string]int64{"seed": 1, "running": 2}
		return v[ev.Resource]
	}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1),
		withRetryTransform(transform), withVersion(version))
	defer q.stop()

	// Occupy the worker.
	q.enqueue(Event[string]{ResourceID: "blocker", Resource: "x"})
	<-h.enter

	// Forced seed arrives, then a strictly newer Running payload coalesces in. The
	// force mark must survive so recovery still bypasses the phase gate.
	q.enqueueForced(Event[string]{ResourceID: "gw-1", Resource: "seed"})
	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "running"})

	h.release <- struct{}{} // release blocker
	<-h.enter
	got := h.lastSeen()
	h.release <- struct{}{}

	if got != "forced:running" {
		t.Fatalf("first attempt saw %q, want %q (a newer Running payload must not erase the force mark)", got, "forced:running")
	}
}

// Force propagation on a version-drop must schedule the key (queue.Add) so the
// forced recovery is actually processed. Without the Add, a key whose retained
// payload was already handled could sit in q.latest with a forced mark that is
// never consumed.
func TestReconcileQueue_ForcePropagationSchedulesKey(t *testing.T) {
	h := &recordingHandler{}
	version := func(ev Event[string]) int64 {
		v := map[string]int64{"newer": 2, "older-seed": 1}
		return v[ev.Resource]
	}
	transform := func(ev Event[string]) Event[string] {
		ev.Resource = "forced:" + ev.Resource
		return ev
	}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1),
		withRetryTransform(transform), withVersion(version))
	defer q.stop()

	// Enqueue and let the worker process "newer" normally (no force).
	q.enqueue(Event[string]{ResourceID: "gw-1", Resource: "newer"})
	waitForCount(t, h, 1)

	// Now enqueue a version-dropped forced seed. The payload stays "newer" but
	// the force mark must propagate AND the key must be re-scheduled.
	q.enqueueForced(Event[string]{ResourceID: "gw-1", Resource: "older-seed"})
	waitForCount(t, h, 2)

	h.mu.Lock()
	got := h.seen[1]
	h.mu.Unlock()

	if got != "forced:newer" {
		t.Fatalf("second attempt saw %q, want %q (force propagation must re-schedule the key)", got, "forced:newer")
	}
}

// pruneIfNonDelete must skip a key whose version differs from the snapshot --
// a newer payload enqueued by the concurrent receiver after the knownKeys
// snapshot must not be erased.
func TestReconcileQueue_PruneIfNonDeleteSkipsNewerVersion(t *testing.T) {
	version := func(ev Event[string]) int64 {
		v := map[string]int64{"old": 1, "new": 2}
		return v[ev.Resource]
	}
	q := newReconcileQueue(context.Background(), "Test", &recordingHandler{},
		withRateLimiter[string](fastLimiter()), withWorkers[string](0),
		withVersion(version))
	defer q.stop()

	// Set up a tracked key with a newer version than the snapshot.
	q.mu.Lock()
	q.latest["gw-1"] = Event[string]{Type: EventUpdated, ResourceID: "gw-1", Resource: "new"}
	q.mu.Unlock()

	snapshot := Event[string]{Type: EventUpdated, ResourceID: "gw-1", Resource: "old"}
	if q.pruneIfNonDelete("gw-1", snapshot) {
		t.Fatal("pruneIfNonDelete must skip when current version differs from snapshot")
	}

	q.mu.Lock()
	_, stillPresent := q.latest["gw-1"]
	q.mu.Unlock()
	if !stillPresent {
		t.Fatal("the newer entry must not have been erased")
	}
}

// pruneIfNonDelete must skip a pending delete even if versions match.
func TestReconcileQueue_PruneIfNonDeleteSkipsPendingDelete(t *testing.T) {
	q := newReconcileQueue(context.Background(), "Test", &recordingHandler{},
		withRateLimiter[string](fastLimiter()), withWorkers[string](0))
	defer q.stop()

	q.mu.Lock()
	q.latest["gw-1"] = Event[string]{Type: EventDeleted, ResourceID: "gw-1", Resource: "v1"}
	q.mu.Unlock()

	snapshot := Event[string]{Type: EventUpdated, ResourceID: "gw-1", Resource: "v1"}
	if q.pruneIfNonDelete("gw-1", snapshot) {
		t.Fatal("pruneIfNonDelete must not prune a pending delete")
	}
}

// A delete clears any pending force mark so a deleted gateway is not
// force-recovered after re-creation.
func TestReconcileQueue_DeleteClearsForce(t *testing.T) {
	h := &recordingHandler{
		enter:   make(chan struct{}),
		release: make(chan struct{}),
	}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1))
	defer q.stop()

	// Occupy the worker.
	q.enqueue(Event[string]{ResourceID: "blocker", Resource: "x"})
	<-h.enter

	q.enqueueForced(Event[string]{ResourceID: "gw-1", Resource: "seed"})
	q.enqueue(Event[string]{Type: EventDeleted, ResourceID: "gw-1", Resource: "gone"})

	q.mu.Lock()
	forced := q.forced["gw-1"]
	evType := q.latest["gw-1"].Type
	q.mu.Unlock()

	h.release <- struct{}{}
	// Drain remaining work.
	go func() {
		for {
			select {
			case <-h.enter:
				h.release <- struct{}{}
			case <-time.After(100 * time.Millisecond):
				return
			}
		}
	}()
	time.Sleep(120 * time.Millisecond)

	if forced {
		t.Fatal("delete must clear the forced mark")
	}
	if evType != EventDeleted {
		t.Fatalf("pending event type = %v, want EventDeleted", evType)
	}
}

// waitForTombstone polls until the key collapses to a terminal tombstone
// (EventDeleted with a zeroed payload), failing the test if it does not.
func waitForTombstone(t *testing.T, q *reconcileQueue[string], id string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		q.mu.Lock()
		cur, ok := q.latest[id]
		q.mu.Unlock()
		if ok && cur.Type == EventDeleted && cur.Resource == "" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("key %s never collapsed to a terminal tombstone", id)
}

// After a delete's teardown succeeds the queue leaves a terminal tombstone, and a
// stale forced seed (captured before the delete, so lower version) arriving later
// must be fully rejected: it cannot resurrect the gateway, replace the tombstone,
// or set the forced recovery bit. Without the tombstone, removing the key would
// let this seed find no pending delete and re-provision a gateway that is gone.
func TestReconcileQueue_TombstoneBlocksStaleForcedSeedAfterDelete(t *testing.T) {
	h := &recordingHandler{}
	transform := func(ev Event[string]) Event[string] {
		ev.Resource = "forced:" + ev.Resource
		return ev
	}
	version := func(ev Event[string]) int64 {
		v := map[string]int64{"gone": 2, "stale-seed": 1}
		return v[ev.Resource]
	}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1),
		withRetryTransform(transform), withVersion(version))
	defer q.stop()

	// Handle a delete to completion; the queue must leave a terminal tombstone.
	q.enqueue(Event[string]{Type: EventDeleted, ResourceID: "gw-1", Resource: "gone"})
	waitForCount(t, h, 1)
	waitForTombstone(t, q, "gw-1")

	// A stale forced seed (older version, captured before the delete) arrives.
	q.enqueueForced(Event[string]{Type: EventUpdated, ResourceID: "gw-1", Resource: "stale-seed"})

	q.mu.Lock()
	forced := q.forced["gw-1"]
	cur := q.latest["gw-1"]
	q.mu.Unlock()
	if forced {
		t.Fatal("a stale forced seed after a completed delete must not set the forced bit")
	}
	if cur.Type != EventDeleted || cur.Resource != "" {
		t.Fatalf("tombstone replaced: got type=%v resource=%q, want EventDeleted with a zeroed payload", cur.Type, cur.Resource)
	}

	// Give any errant reschedule a chance to run, then confirm the handler never
	// saw the seed -- no resurrection or recovery attempt.
	time.Sleep(50 * time.Millisecond)
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.seen {
		if s == "stale-seed" || s == "forced:stale-seed" {
			t.Fatalf("handler saw %q; a stale seed after a completed delete must not resurrect the gateway", s)
		}
	}
}

// A duplicate/replayed delete that coalesces in while the first delete is being
// handled is fresh pending work: it must NOT be collapsed to a zeroed tombstone
// (its handler needs the payload to tear down), and must be reprocessed with its
// full payload. This runs with NO version callback so the two deletes are
// version-equal -- the case the old version-equality guard got wrong; the
// accepted-payload generation must detect the duplicate regardless.
func TestReconcileQueue_DuplicateDeleteRetainsPayload(t *testing.T) {
	h := &recordingHandler{
		enter:   make(chan struct{}),
		release: make(chan struct{}),
	}
	q := newReconcileQueue(context.Background(), "Test", h,
		withRateLimiter[string](fastLimiter()), withWorkers[string](1))
	defer q.stop()

	// First delete enters Handle and is held in-flight.
	q.enqueue(Event[string]{Type: EventDeleted, ResourceID: "gw-1", Resource: "del-first"})
	<-h.enter

	// A replayed duplicate delete (same version -- there is no versionOf) coalesces
	// in while the first is still handling.
	q.enqueue(Event[string]{Type: EventDeleted, ResourceID: "gw-1", Resource: "del-second"})

	h.release <- struct{}{} // first delete's Handle returns nil

	// The worker must reprocess the duplicate with its full payload, not a zeroed
	// tombstone.
	<-h.enter
	got := h.lastSeen()
	h.release <- struct{}{}
	if got != "del-second" {
		t.Fatalf("reprocessed delete saw %q, want the duplicate's full payload %q", got, "del-second")
	}
}
