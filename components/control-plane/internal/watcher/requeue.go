package watcher

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"k8s.io/client-go/util/workqueue"
)

// Gateway reconcile retry tuning. A failed reconcile is requeued with capped
// exponential backoff and retried indefinitely -- until the reconcile succeeds
// (Handle returns nil) or the watcher context ends. A newer desired state does
// not cancel the retry; it coalesces into the queue's latest payload, so the
// next attempt reconciles the freshest state. The gateway watch stream does not
// replay state on (re)connect, so the queue is the only mechanism that re-drives
// a reconcile that failed without a subsequent spec change; a finite attempt
// budget would reintroduce the permanent-stranded-gateway bug once the budget was
// spent during a long outage.
const (
	gatewayRequeueBaseDelay = 2 * time.Second
	gatewayRequeueMaxDelay  = 30 * time.Second
	// gatewayReconcileWorkers bounds how many *distinct* gateways reconcile
	// concurrently. The queue serializes work per gateway regardless (a key in
	// flight is never handed to a second worker), so this only caps cross-gateway
	// parallelism -- keeping one slow provision (which blocks on a multi-minute
	// TLS-secret wait) from stalling every other gateway behind it, as the old
	// synchronous Recv loop did.
	gatewayReconcileWorkers = 4
)

// preservePayloadForRetryError marks a handler failure whose retry must receive
// the untransformed latest payload. It is used for work intentionally performed
// behind a caller's gate, where applying the normal recovery transform would
// change the operation's scope (for gateways, from Keycloak-only repair to full
// Kubernetes provisioning).
type preservePayloadForRetryError struct {
	err error
}

func (e *preservePayloadForRetryError) Error() string { return e.err.Error() }
func (e *preservePayloadForRetryError) Unwrap() error { return e.err }

// PreservePayloadForRetry wraps err so the reconcile queue retries it without
// applying its retry transform. If forced recovery is also requested, the queue
// defers that transformed pass until the preserved operation succeeds.
func PreservePayloadForRetry(err error) error {
	if err == nil {
		return nil
	}
	return &preservePayloadForRetryError{err: err}
}

// PreservesPayloadForRetry reports whether err requests a retry with the
// original payload. It is exported so handlers can assert that narrowly scoped
// failures retain their queue semantics through additional error wrapping.
func PreservesPayloadForRetry(err error) bool {
	var marked *preservePayloadForRetryError
	return errors.As(err, &marked)
}

// reconcileQueue serializes reconciliations per resource. It retries errors
// by default, with optional limits and error filters for callers that need them.
// It replaces the fire-and-forget requeue goroutine that could race the reconciler.
//
// It is a thin wrapper over client-go's rate-limiting workqueue plus a map of the
// latest event seen per resource. Every observed event calls enqueue, which
// records the latest payload and Adds the resource key. Worker goroutines pull
// keys and invoke the handler with the *latest* payload for that key. This gives
// three properties the previous requeuer lacked:
//
//   - Per-resource serialization: the workqueue never hands the same key to two
//     workers at once, so a retry can never run concurrently with a live event
//     for the same gateway. There is no nil-result cancellation to get wrong: a
//     successful (or legitimately phase-gated) reconcile simply Forgets the key,
//     and only a returned error requeues it.
//   - Coalescing to latest desired state: a retry reconciles the newest observed
//     payload, not a frozen original. A gateway un-routed after a failed
//     provisioning attempt is therefore torn down on retry, never resurrected.
//   - Capped-backoff recovery: the default policy retries until the handler
//     succeeds. A caller can use a smaller policy when an error is permanent.
//
// The retryTransform hook lets a caller adjust the payload used *only* on retries
// (NumRequeues > 0). Gateways use it to clear the phase the reconciler itself
// stamps (Provisioning) before doing its work: that self-write coalesces into the
// latest payload, so a retry that reused it verbatim would hit the phase gate and
// silently no-op, re-stranding the very gateway the retry exists to recover. The
// first attempt of a freshly observed event runs untransformed so the phase gate
// still governs ordinary create/update traffic. A handler can wrap an error with
// PreservePayloadForRetry when the failed operation deliberately ran behind that
// gate and its retry must remain there rather than becoming a full recovery pass.
type reconcileQueue[T any] struct {
	baseCtx        context.Context
	handler        Handler[T]
	kind           string
	workers        int
	maxRetries     int
	retryIf        func(error) bool
	retryTransform func(Event[T]) Event[T]
	// versionOf returns a monotonically increasing version for an event's payload
	// (for gateways, the resource's updated_at). When set, enqueue coalesces to the
	// highest-versioned payload so a stale snapshot -- from the startup/reconnect
	// seed or the periodic resync -- can never clobber a newer live event, and a
	// buffered out-of-order live event can never clobber a newer seed. Deletes are
	// terminal and bypass the check. Nil disables version-aware coalescing (plain
	// last-writer-wins), which is fine for streams that never seed.
	versionOf func(Event[T]) int64
	now       func() time.Time

	queue   workqueue.TypedRateLimitingInterface[string]
	limiter workqueue.TypedRateLimiter[string]

	mu     sync.Mutex
	latest map[string]Event[T]
	// forced is a per-key recovery bit set by enqueueForced. It marks a key whose
	// next real handler attempt must run through retryTransform even on its first
	// attempt (NumRequeues == 0), so a forced payload (e.g. a startup seed of an
	// active-phase gateway) bypasses the reconciler's phase gate. It is tracked
	// independently of latest: a live event can coalesce a newer payload over the
	// seed's -- even one with an equal version that overwrites the phase-cleared
	// clone -- without erasing the bypass, because the bit, not the payload, drives
	// the transform.
	//
	// The mark is coalesced with incoming events under the queue lock:
	//  - A version-dropped forced seed still propagates its mark onto the
	//    retained newer payload and reschedules the key. The seed itself proves
	//    startup recovery was requested; the retained payload's phase is NOT
	//    authoritative, because the independent GatewayHealthReconciler may have
	//    written a newer Running while GatewayReconciler.Handle is still failing.
	//  - An existing mark is preserved across equal- or newer-version non-delete
	//    payloads regardless of phase. A newer Running/healthy payload does NOT
	//    clear it for the same reason: that phase does not prove the forced
	//    recovery's Handle succeeded, so clearing it could strand a failed or
	//    incomplete reconcile behind the phase gate.
	//  - A delete clears force (the gateway is going away).
	//  - The mark is cleared when a transformed handler attempt consumes it (or
	//    the key is pruned), never by a mere backoff re-defer. If a preserved
	//    lightweight retry is pending, force remains sticky until that operation
	//    succeeds, then drives exactly one deferred transformed pass.
	forced map[string]bool
	// preservePayload marks a failed key whose next retry must bypass
	// retryTransform. It takes precedence over forced recovery so a dependency
	// failure behind a phase gate cannot expand into full provisioning. The handler
	// refreshes this policy on every failure; success, deletion, or pruning clears
	// it while leaving any deferred force intent available for the next pass.
	preservePayload map[string]bool
	// readyAt stores the first time that a pending key became eligible for a
	// worker. Coalesced updates keep the first value. A retry replaces it with the
	// end of its scheduled backoff so the backoff is not queue wait.
	readyAt map[string]time.Time
	// notBefore is the earliest time a failed key may be handled again. It defends
	// the AddRateLimited backoff against client-go's dirty-key semantics: the
	// reconciler's own phase-status writes emit watch events that Add (mark dirty)
	// the key while it is being processed, so Done would otherwise re-queue it for
	// immediate handling and bypass the backoff delay -- a hot spin that re-hammers
	// the API server and Keycloak (and re-writes Provisioning each pass, sustaining
	// the self-event storm). While a key is within its backoff, processNext
	// re-defers it cheaply instead of invoking the handler.
	notBefore map[string]time.Time
	// gen is a per-key accepted-payload generation, bumped every time enqueue
	// accepts/replaces latest for a key (including every delete, even a version-
	// equal replay). processNext snapshots it alongside the payload before Handle
	// and uses it to detect whether a newer payload coalesced in while it worked --
	// which version comparison cannot do reliably, because a replayed delete can
	// carry the same updated_at as the in-flight one (and versionOf may be nil).
	// It is deleted with the key on prune.
	gen                  map[string]int64
	recordQueueWait      func(time.Duration)
	unregisterQueueDepth func() error
	wg                   sync.WaitGroup
	stopOnce             sync.Once
	unregisterOnce       sync.Once
	stopCh               chan struct{}
}

// queueOption customizes a reconcileQueue before its workers start.
type queueOption[T any] func(*reconcileQueue[T])

// withRetryTransform sets the payload transform applied on retry attempts.
func withRetryTransform[T any](f func(Event[T]) Event[T]) queueOption[T] {
	return func(q *reconcileQueue[T]) { q.retryTransform = f }
}

// withRateLimiter overrides the queue's rate limiter (used by tests to shorten
// backoff).
func withRateLimiter[T any](l workqueue.TypedRateLimiter[string]) queueOption[T] {
	return func(q *reconcileQueue[T]) {
		q.limiter = l
		q.queue = workqueue.NewTypedRateLimitingQueue(l)
	}
}

// withNow overrides the queue clock for tests.
func withNow[T any](now func() time.Time) queueOption[T] {
	return func(q *reconcileQueue[T]) { q.now = now }
}

// withQueueWaitRecorder enables queue-wait tracking with a test recorder.
func withQueueWaitRecorder[T any](record func(time.Duration)) queueOption[T] {
	return func(q *reconcileQueue[T]) { q.recordQueueWait = record }
}

// withWorkers overrides the worker count (used by tests).
func withWorkers[T any](n int) queueOption[T] {
	return func(q *reconcileQueue[T]) { q.workers = n }
}

// withMaxRetries sets the number of retries after the first attempt. A negative
// value keeps the default unlimited retry behavior.
func withMaxRetries[T any](n int) queueOption[T] {
	return func(q *reconcileQueue[T]) { q.maxRetries = n }
}

// withRetryIf limits retries to errors that the function accepts.
func withRetryIf[T any](f func(error) bool) queueOption[T] {
	return func(q *reconcileQueue[T]) { q.retryIf = f }
}

// withVersion enables version-aware coalescing keyed on f (see the versionOf
// field). Used so seed/resync snapshots and live events converge on the newest
// observed state regardless of the order they are enqueued.
func withVersion[T any](f func(Event[T]) int64) queueOption[T] {
	return func(q *reconcileQueue[T]) { q.versionOf = f }
}

// newReconcileQueue builds and starts a reconcile queue. Workers run on baseCtx
// (the watcher lifetime), not any per-stream context, so retries survive a stream
// reconnect. Call stop to drain and shut down.
func newReconcileQueue[T any](baseCtx context.Context, kind string, handler Handler[T], opts ...queueOption[T]) *reconcileQueue[T] {
	limiter := workqueue.NewTypedItemExponentialFailureRateLimiter[string](gatewayRequeueBaseDelay, gatewayRequeueMaxDelay)
	q := &reconcileQueue[T]{
		baseCtx:         baseCtx,
		handler:         handler,
		kind:            kind,
		workers:         gatewayReconcileWorkers,
		maxRetries:      -1,
		now:             time.Now,
		limiter:         limiter,
		queue:           workqueue.NewTypedRateLimitingQueue(limiter),
		latest:          make(map[string]Event[T]),
		forced:          make(map[string]bool),
		preservePayload: make(map[string]bool),
		notBefore:       make(map[string]time.Time),
		gen:             make(map[string]int64),
		stopCh:          make(chan struct{}),
	}
	for _, opt := range opts {
		opt(q)
	}
	if q.recordQueueWait != nil || cpotel.MetricsEnabled() {
		q.readyAt = make(map[string]time.Time)
	}
	if q.recordQueueWait == nil && cpotel.MetricsEnabled() {
		q.recordQueueWait = func(duration time.Duration) {
			cpotel.RecordReconcileQueueWaitDuration(q.baseCtx, q.kind, duration)
		}
	}
	if cpotel.MetricsEnabled() {
		unregister, err := cpotel.RegisterReconcileQueueDepth(q.kind, func() int64 {
			return q.readyDepth()
		})
		if err != nil {
			log.Printf("WARN register %s reconcile queue depth metric: %v", q.kind, err)
		} else {
			q.unregisterQueueDepth = unregister
		}
	}
	// Shut the queue down when the watcher context ends so blocked workers wake and
	// exit even if stop is never reached; stopCh does the same for an explicit stop
	// (baseCtx may be context.Background, whose Done never fires). ShutDown is
	// idempotent, so both paths are safe.
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		select {
		case <-baseCtx.Done():
		case <-q.stopCh:
		}
		q.queue.ShutDown()
	}()
	q.start()
	return q
}

// enqueue records the latest payload for a resource and schedules it for
// reconciliation, coalescing with any pending work for the same resource.
func (q *reconcileQueue[T]) enqueue(ev Event[T]) {
	q.enqueueWithForce(ev, false)
}

// enqueueForced is enqueue plus a sticky recovery mark (see the forced field): the
// key's next real handler attempt runs through retryTransform even on its first
// attempt, so a forced payload bypasses a caller's phase gate. The mark survives
// later coalescing -- including an equal-version live event that overwrites the
// forced payload -- so the bypass is not lost before it is applied. A forced
// enqueue that is itself dropped (e.g. blocked by a pending terminal delete) does
// not set the mark, so a resource mid-deletion is not force-recovered.
func (q *reconcileQueue[T]) enqueueForced(ev Event[T]) {
	q.enqueueWithForce(ev, true)
}

func (q *reconcileQueue[T]) enqueueWithForce(ev Event[T], force bool) {
	q.mu.Lock()
	// A delete always wins: overwrite whatever is pending and schedule it. A
	// delete also clears any pending force mark -- the gateway is going away, so
	// forced recovery is moot, and leaving it set could corrupt a later create.
	// The retry counter/backoff are intentionally left intact: a delete is itself
	// a reconcile whose teardown may fail and must be retried durably like any
	// other Handle error.
	if ev.Type == EventDeleted {
		q.latest[ev.ResourceID] = ev
		q.gen[ev.ResourceID]++
		delete(q.forced, ev.ResourceID)
		delete(q.preservePayload, ev.ResourceID)
		q.markReadyLocked(ev.ResourceID)
		q.mu.Unlock()
		q.queue.Add(ev.ResourceID)
		return
	}
	if prev, ok := q.latest[ev.ResourceID]; ok {
		// A delete is terminal, both while it is pending and after its teardown
		// succeeds: a non-delete event -- a stale seed/resync snapshot or a
		// buffered out-of-order live event -- must not overwrite it and resurrect
		// a resource whose deletion has been (or is being) reconciled. After a
		// successful delete, processNext leaves a compact tombstone (EventDeleted
		// with a zeroed payload) in latest rather than removing the key, and that
		// tombstone keeps blocking non-delete events here. This is safe because
		// gateway IDs are immutable KSUIDs (gateways/model.go BeforeCreate always
		// overwrites the ID), so a create/update for a deleted ID is always stale,
		// never a legitimate reuse. Only a duplicate delete may replace the
		// pending payload or the tombstone (handled by the delete branch above),
		// reconciling idempotently.
		if prev.Type == EventDeleted {
			q.mu.Unlock()
			return
		}
		// Version-aware coalescing: drop an event older than the pending payload
		// so a stale snapshot never clobbers newer live state (and vice versa).
		// The newer payload is already queued, so there is normally nothing to
		// schedule -- except that a dropped FORCED seed must still leave its
		// recovery mark on the retained payload and reschedule the key. The seed
		// itself proves startup recovery was requested; the retained payload's
		// phase is NOT authoritative (the independent GatewayHealthReconciler may
		// have written a newer Running while GatewayReconciler.Handle is still
		// failing), so force is propagated regardless of that phase. The key is
		// (re)scheduled because the retained payload may already have been handled
		// and dropped from the workqueue, so its mere presence in latest does not
		// prove it is queued.
		if q.versionOf != nil && q.versionOf(ev) < q.versionOf(prev) {
			if force {
				q.forced[ev.ResourceID] = true
				q.markReadyLocked(ev.ResourceID)
				q.mu.Unlock()
				q.queue.Add(ev.ResourceID)
				return
			}
			q.mu.Unlock()
			return
		}
		// The incoming event is accepted (equal or newer version). An existing
		// force mark is preserved regardless of the incoming phase: a newer
		// Running/healthy payload does NOT clear it, because that phase is written
		// independently by the GatewayHealthReconciler from Deployment and route
		// readiness and does not prove the forced recovery's Handle succeeded --
		// clearing it here could strand a failed or incomplete reconcile behind
		// the phase gate. The mark is cleared only by a delete or once a handler
		// attempt consumes it.
	}
	q.latest[ev.ResourceID] = ev
	q.gen[ev.ResourceID]++
	if force {
		q.forced[ev.ResourceID] = true
	}
	q.markReadyLocked(ev.ResourceID)
	q.mu.Unlock()
	q.queue.Add(ev.ResourceID)
}

// markReadyLocked records when a key first becomes eligible for a worker. The
// caller must hold q.mu.
func (q *reconcileQueue[T]) markReadyLocked(id string) {
	if q.recordQueueWait == nil {
		return
	}
	if _, exists := q.readyAt[id]; exists {
		return
	}
	readyAt := q.now()
	if notBefore, exists := q.notBefore[id]; exists && notBefore.After(readyAt) {
		readyAt = notBefore
	}
	q.readyAt[id] = readyAt
}

// readyDepth returns the number of keys that are eligible for a worker. A key
// with a future ready time is in scheduled retry backoff and is not ready.
func (q *reconcileQueue[T]) readyDepth() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := q.now()
	var depth int64
	for _, readyAt := range q.readyAt {
		if !readyAt.After(now) {
			depth++
		}
	}
	return depth
}

// knownKeys returns a snapshot of the payloads the queue is currently tracking
// (one per resource with pending work). The seed uses it to reconcile an
// authoritative list against what the queue still believes exists. It is reached
// only through the gatewaySeedSink interface (whose compile-time assertion lives
// in watcher.go); golangci's unused analyzer does not trace that dispatch for a
// generic type, so the nolint below suppresses the false positive.
//
//nolint:unused // reached via gatewaySeedSink interface dispatch; see doc above.
func (q *reconcileQueue[T]) knownKeys() map[string]Event[T] {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make(map[string]Event[T], len(q.latest))
	for k, v := range q.latest {
		out[k] = v
	}
	return out
}

// pruneIfNonDelete atomically prunes a key only if the current queue entry
// still matches the knownKeys snapshot (same type and version). If the
// concurrent watch receiver enqueued a delete or a newer/different payload
// after the snapshot was taken, the prune is skipped so the pending delete's
// teardown still runs and newer state is preserved.
func (q *reconcileQueue[T]) pruneIfNonDelete(id string, snapshot Event[T]) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	cur, ok := q.latest[id]
	if !ok {
		return false
	}
	if cur.Type == EventDeleted {
		return false
	}
	if q.versionOf != nil && q.versionOf(cur) != q.versionOf(snapshot) {
		return false
	}
	if cur.Type != snapshot.Type {
		return false
	}
	delete(q.latest, id)
	delete(q.notBefore, id)
	delete(q.forced, id)
	delete(q.preservePayload, id)
	delete(q.readyAt, id)
	delete(q.gen, id)
	return true
}

func (q *reconcileQueue[T]) start() {
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go func() {
			defer q.wg.Done()
			for q.processNext() {
			}
		}()
	}
}

// processNext reconciles one key. It returns false only when the queue is shut
// down, so workers loop on it until then.
func (q *reconcileQueue[T]) processNext() bool {
	id, shutdown := q.queue.Get()
	if shutdown {
		return false
	}
	defer q.queue.Done(id)

	// Enforce backoff against dirty-key re-adds: if this key failed recently and its
	// delay has not elapsed, re-defer it without invoking the handler. A dirty re-add
	// (e.g. from the reconciler's own phase-status watch event) thus costs a cheap
	// Get/AddAfter/Done cycle instead of an immediate re-handle, so the backoff is
	// preserved and no spin re-hammers the API server or Keycloak.
	q.mu.Lock()
	nb, backingOff := q.notBefore[id]
	q.mu.Unlock()
	if backingOff {
		if remaining := nb.Sub(q.now()); remaining > 0 {
			q.mu.Lock()
			if q.recordQueueWait != nil {
				q.readyAt[id] = nb
			}
			q.mu.Unlock()
			q.queue.AddAfter(id, remaining)
			return true
		}
	}

	q.mu.Lock()
	ev, ok := q.latest[id]
	// Snapshot the accepted-payload generation with the payload so a post-Handle
	// compaction can tell whether a newer payload coalesced in while we worked.
	gen := q.gen[id]
	// Snapshot retry policy atomically with the payload. A preserved lightweight
	// retry takes precedence over force: consuming force here would either transform
	// this attempt into full recovery or lose the reconnect request. Instead, leave
	// force sticky until the preserved operation succeeds. Without preservation,
	// this attempt consumes force under the same lock; a force arriving later will
	// re-set the bit and dirty the key for a subsequent pass.
	preservePayload := q.preservePayload[id]
	forced := q.forced[id]
	deferredForced := preservePayload && forced
	readyAt, wasReady := q.readyAt[id]
	delete(q.readyAt, id)
	if !preservePayload {
		delete(q.forced, id)
	}
	q.mu.Unlock()
	if !ok {
		// No payload recorded (e.g. a delete already reconciled and pruned it).
		q.mu.Lock()
		delete(q.forced, id)
		delete(q.readyAt, id)
		q.mu.Unlock()
		q.queue.Forget(id)
		q.clearBackoff(id)
		return true
	}

	// A requeued attempt is out-of-band recovery for an earlier failure; let the
	// caller adapt the payload (gateways clear the phase so recovery bypasses the
	// phase gate). A forced key gets the same transform on its first attempt so a
	// startup/reconnect seed of an active-phase gateway bypasses the gate even
	// though NumRequeues is 0. A marked failure always preserves the payload,
	// including when force is pending, so work intentionally performed behind the
	// gate stays there. Ordinary first attempts run untransformed so the gate still
	// governs live traffic.
	if q.retryTransform != nil && !preservePayload && (forced || q.queue.NumRequeues(id) > 0) {
		ev = q.retryTransform(ev)
	}
	if wasReady && q.recordQueueWait != nil {
		if wait := q.now().Sub(readyAt); wait >= 0 {
			q.recordQueueWait(wait)
		}
	}

	if err := q.handler.Handle(q.baseCtx, ev); err != nil {
		retryAllowed := q.maxRetries < 0 || q.queue.NumRequeues(id) < q.maxRetries
		if q.retryIf != nil && !q.retryIf(err) {
			retryAllowed = false
		}
		if !retryAllowed {
			q.queue.Forget(id)
			q.clearBackoff(id)
			log.Printf("ERROR %s %s reconcile failed: %v", q.kind, id, err)
			return true
		}
		// When bumps the limiter and returns this attempt's capped-exponential
		// delay; record it as the key's backoff floor and schedule the retry for
		// then. Using AddAfter (not AddRateLimited) keeps the delay authoritative
		// even though dirty re-adds may reach the queue sooner -- the notBefore gate
		// above re-defers them. The retry remains durable until Handle succeeds,
		// subject to the queue's optional retry limit/filter. It applies or bypasses
		// retryTransform according to the error marker. Later Running/Degraded events
		// from the independent GatewayHealthReconciler do not cancel it: they reflect
		// workload readiness, not that the provisioning Handle recovered.
		delay := q.limiter.When(id)
		q.mu.Lock()
		notBefore := q.now().Add(delay)
		q.notBefore[id] = notBefore
		if q.recordQueueWait != nil {
			q.readyAt[id] = notBefore
		}
		if PreservesPayloadForRetry(err) {
			q.preservePayload[id] = true
		} else {
			delete(q.preservePayload, id)
		}
		q.mu.Unlock()
		q.queue.AddAfter(id, delay)
		log.Printf("WARN %s %s reconcile failed; retrying in %s: %v", q.kind, id, delay, err)
		return true
	}

	// Success or a legitimate phase-gated no-op: stop retrying. If a newer event
	// arrived while we worked, Add already re-dirtied the key so it is processed
	// again with the latest payload. When this successful attempt deliberately
	// preserved its payload while force was pending, clear the retry state and
	// explicitly schedule the still-sticky force bit. The next pass consumes it and
	// transforms exactly once, even if the forced enqueue itself was the queue item
	// consumed by this preserved attempt.
	q.queue.Forget(id)
	q.clearBackoff(id)
	if deferredForced {
		q.mu.Lock()
		forcePending := q.forced[id]
		if forcePending {
			q.markReadyLocked(id)
		}
		q.mu.Unlock()
		if forcePending {
			q.queue.Add(id)
		}
	}

	// Collapse a fully-handled delete to a compact terminal tombstone: keep the
	// key with EventDeleted + ResourceID but zero the payload so the deleted
	// protobuf is not retained. The key is intentionally NOT removed: removing it
	// reopens a resurrection race where a stale non-delete event captured before
	// the delete (e.g. a startup seed's list snapshot) finds no pending delete and
	// re-provisions the gone gateway. The tombstone keeps enqueue's terminal-delete
	// guard blocking such events; immutable KSUID IDs mean a create/update for this
	// ID is always stale. Only collapse if the accepted-payload generation is
	// unchanged since we snapshotted it: a newer payload that coalesced in while we
	// worked (a duplicate/replayed delete, which bumps gen even at an equal
	// updated_at) is fresh pending work that must retain its full payload so its
	// own teardown runs (the handler needs the resource -- namespace, name,
	// credential driver). It re-adds the key, so it is reprocessed next. The
	// tombstone keeps the same generation, so a stale enqueue that raced this
	// compaction is still detected as newer. Generation, not version equality, is
	// used because a replayed delete can carry an identical updated_at (and
	// versionOf may be nil).
	if ev.Type == EventDeleted {
		q.mu.Lock()
		if cur, ok := q.latest[id]; ok && cur.Type == EventDeleted && q.gen[id] == gen {
			q.latest[id] = Event[T]{Type: EventDeleted, ResourceID: id}
		}
		q.mu.Unlock()
	}
	return true
}

// clearBackoff forgets a key's recorded backoff floor (on success or once its
// payload is gone).
func (q *reconcileQueue[T]) clearBackoff(id string) {
	q.mu.Lock()
	delete(q.notBefore, id)
	delete(q.preservePayload, id)
	q.mu.Unlock()
}

// stop shuts the queue down and waits for all workers (and the context watcher)
// to exit. Safe to call more than once.
func (q *reconcileQueue[T]) stop() {
	q.stopOnce.Do(func() { close(q.stopCh) })
	q.queue.ShutDown()
	q.wg.Wait()
	q.unregisterOnce.Do(func() {
		if q.unregisterQueueDepth != nil {
			if err := q.unregisterQueueDepth(); err != nil {
				log.Printf("WARN unregister %s reconcile queue depth metric: %v", q.kind, err)
			}
		}
	})
}
