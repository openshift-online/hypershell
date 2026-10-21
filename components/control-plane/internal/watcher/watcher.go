package watcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/api-server/pkg/gatewayhealth"
	"github.com/openshift-online/hypershell/components/control-plane/internal/keycloak"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type EventType int

const (
	EventCreated EventType = iota
	EventUpdated
	EventDeleted
)

func (e EventType) String() string {
	switch e {
	case EventCreated:
		return "reconcile"
	case EventUpdated:
		return "reconcile"
	case EventDeleted:
		return "delete"
	default:
		return "reconcile"
	}
}

type Event[T any] struct {
	Type       EventType
	ResourceID string
	Resource   T
	// PhaseBeforeRetry contains the Gateway phase that the retry adapter
	// cleared. Other resource types leave this field empty.
	PhaseBeforeRetry string
}

type Handler[T any] interface {
	Handle(ctx context.Context, event Event[T]) error
}

func toEventType(t pb.EventType) EventType {
	switch t {
	case pb.EventType_EVENT_TYPE_CREATED:
		return EventCreated
	case pb.EventType_EVENT_TYPE_UPDATED:
		return EventUpdated
	case pb.EventType_EVENT_TYPE_DELETED:
		return EventDeleted
	default:
		return EventCreated
	}
}

func WatchManagedClusters(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.ManagedCluster], idleTimeout time.Duration) error {
	client := pb.NewManagedClusterServiceClient(conn)
	return watchLoop(ctx, "ManagedCluster", idleTimeout, func(ctx context.Context, heartbeat func()) error {
		stream, err := client.WatchManagedClusters(ctx, &pb.WatchManagedClustersRequest{})
		if err != nil {
			return fmt.Errorf("starting managed cluster watch: %w", err)
		}
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("receiving managed cluster event: %w", err)
			}
			heartbeat()
			if err := handler.Handle(ctx, Event[*pb.ManagedCluster]{
				Type:       toEventType(event.Type),
				ResourceID: event.ResourceId,
				Resource:   event.ManagedCluster,
			}); err != nil {
				log.Printf("ERROR handling managed cluster %s: %v", event.ResourceId, err)
			}
		}
	})
}

// WatchRepositories streams Repository events and drives each through the handler.
// Repository is a platform-level resource, not cluster-scoped, so the watch takes
// no cluster filter, mirroring WatchManagedClusters. It drives the event-driven
// declarative `.hypershell` import: a created or changed Repository is reconciled in
// real time instead of waiting for the slow periodic drift re-check
// (control-plane.spec.md, "Declarative `.hypershell` Import Reconciliation"). Like
// the other watches it reconnects with backoff and is covered by the idle watchdog;
// it does not replay existing Repositories, so the reconciler's periodic pass seeds
// the ones already registered at startup.
func WatchRepositories(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.Repository], idleTimeout time.Duration) error {
	client := pb.NewRepositoryServiceClient(conn)
	return watchLoop(ctx, "Repository", idleTimeout, func(ctx context.Context, heartbeat func()) error {
		stream, err := client.WatchRepositories(ctx, &pb.WatchRepositoriesRequest{})
		if err != nil {
			return fmt.Errorf("starting repository watch: %w", err)
		}
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("receiving repository event: %w", err)
			}
			heartbeat()
			if err := handler.Handle(ctx, Event[*pb.Repository]{
				Type:       toEventType(event.Type),
				ResourceID: event.ResourceId,
				Resource:   event.Repository,
			}); err != nil {
				log.Printf("ERROR handling repository %s: %v", event.ResourceId, err)
			}
		}
	})
}

// ErrMissingClusterID is returned by every cluster-scoped list/watch (gateways
// and role bindings) when it is asked to run without this control plane's
// registered cluster id. Every control
// plane is registered (specs/platform/control-plane.spec.md, "Mandatory Cluster
// Identity"), so an empty id is a programming error, never an "unfiltered" mode.
var ErrMissingClusterID = errors.New("refusing a cluster-scoped list/watch without this control plane's registered cluster_id")

// ClusterFilter maps this control plane's registered cluster id to the proto
// cluster_id filter the api-server scopes gateway and role binding lists and
// watches by. The
// filter is mandatory: an empty id is rejected with ErrMissingClusterID rather
// than sent as "no filter". Exported and shared with the reconciler's list
// helper so the mapping stays single-sourced.
func ClusterFilter(clusterID string) (*string, error) {
	if strings.TrimSpace(clusterID) == "" {
		return nil, ErrMissingClusterID
	}
	return &clusterID, nil
}

// gatewayWorkerCount clamps a configured gateway reconcile worker count to a
// positive value. Configuration validation already keeps the value positive, so
// this is defense in depth: a non-positive count (from a caller or a future
// config path) would otherwise leave the pool with no workers and stall gateway
// reconciliation entirely, so it falls back to the built-in default instead.
func gatewayWorkerCount(configured int) int {
	if configured < 1 {
		return gatewayReconcileWorkers
	}
	return configured
}

// GatewayReconcileQueue is a shareable handle to the gateway reconcile queue. It
// lets an out-of-band reconciler request a gateway be re-reconciled through the
// same serialized, retrying, phase-gate-bypassing path the gateway watch stream
// uses, without blocking the caller on the (potentially multi-minute) reconcile.
type GatewayReconcileQueue struct {
	q *reconcileQueue[*pb.Gateway]
}

// NewGatewayReconcileQueue builds and starts the shared gateway reconcile queue.
// The caller owns its lifecycle and must call Stop (or cancel ctx) to drain it.
// Pass the returned queue to both WatchGateways and any out-of-band enqueuer.
// workers bounds how many distinct gateways reconcile concurrently (see
// gateway-reconcile-concurrency.spec.md); a value below 1 falls back to the
// built-in default so the pool always has at least one worker.
func NewGatewayReconcileQueue(ctx context.Context, handler Handler[*pb.Gateway], workers int) *GatewayReconcileQueue {
	return &GatewayReconcileQueue{
		q: newReconcileQueue(ctx, "Gateway", handler,
			withRetryTransform(clearGatewayPhaseForRetry),
			withVersion(gatewayEventVersion),
			withWorkers[*pb.Gateway](gatewayWorkerCount(workers))),
	}
}

// EnqueueForced requests reconciliation of the given gateway, marking it so the
// next handler attempt bypasses the reconciler phase gate (as recovery seeds do).
// This is what lets a release image change re-reconcile a gateway that is already
// Running, which the phase gate would otherwise skip.
func (g *GatewayReconcileQueue) EnqueueForced(ev Event[*pb.Gateway]) { g.q.enqueueForced(ev) }

// Stop drains and shuts the queue down.
func (g *GatewayReconcileQueue) Stop() { g.q.stop() }

// WatchGateways streams gateway events and drives them through the caller-owned
// per-resource reconcile queue. The watch and its seed lists are scoped
// server-side to gateways with this control plane's registered cluster_id, so it
// only ever reconciles its own gateways (the pull model); an empty clusterID is
// rejected. The queue is owned and stopped by the caller (main).
func WatchGateways(ctx context.Context, conn *grpc.ClientConn, queue *GatewayReconcileQueue, clusterID string, idleTimeout time.Duration) error {
	clusterFilter, err := ClusterFilter(clusterID)
	if err != nil {
		return err
	}
	client := pb.NewGatewayServiceClient(conn)
	// Gateway reconciliation is driven through a per-resource reconcile queue rather
	// than invoked inline: the watch stream does not replay state on reconnect, so a
	// reconcile that fails (e.g. an API-server outage that also blocks recording a
	// Failed phase) would otherwise strand the gateway until its spec next changes.
	// The queue serializes work per gateway, coalesces to the latest observed state,
	// and retries failures indefinitely with capped backoff -- all on the watcher
	// lifetime context so recovery survives a stream reconnect. The queue is owned
	// by the caller (main).
	rq := queue.q
	return watchLoop(ctx, "Gateway", idleTimeout, func(ctx context.Context, heartbeat func()) error {
		// Derive a cancelable child before creating the stream so either the
		// receiver or the seed can cancel and join the other without waiting
		// for watchLoop to cancel the parent attempt ctx.
		runCtx, runCancel := context.WithCancel(ctx)
		defer runCancel()

		stream, err := client.WatchGateways(runCtx, &pb.WatchGatewaysRequest{ClusterId: clusterFilter})
		if err != nil {
			return fmt.Errorf("starting gateway watch: %w", err)
		}
		// Wait for the stream header before seeding. Opening the stream is not a
		// subscription handshake: client.WatchGateways can return before the server
		// registers its broker subscription, so a seed issued immediately could
		// list state, then miss an event that fires before the subscription goes
		// live. The server flushes the header only after it has subscribed (see the
		// WatchGateways handler), so blocking on Header() closes that list-watch gap
		// -- every event after this point is captured by the watch, and the seed's
		// list captures everything before it. A one-shot seed per connect is then
		// sufficient; no periodic forced resync is needed (which would re-provision
		// gateways the health reconciler legitimately owns in an active phase).
		if _, err := stream.Header(); err != nil {
			return fmt.Errorf("awaiting gateway watch subscription header: %w", err)
		}
		// The subscription is live: count it as activity so the idle watchdog does
		// not fire during the seed window before the first event arrives.
		heartbeat()
		// Drain the watch stream concurrently while seeding. The API server's
		// event broker drops events when its per-subscriber buffer (256 slots)
		// fills, and seeding -- which performs multiple paginated LISTs plus
		// point-read confirmations -- can take long enough for that to happen.
		// If the stream is not read during the seed, dropped events are
		// permanently lost (the stream never replays them), so a gateway whose
		// only spec change occurred during the seed window would be stranded.
		// Draining into the reconcile queue keeps the buffer empty; version-
		// aware coalescing in the queue ensures a live event and its seed
		// counterpart converge to the newest state regardless of arrival order.

		streamErr := make(chan error, 1)
		go func() {
			defer close(streamErr)
			for {
				event, err := stream.Recv()
				if err == io.EOF {
					// Cancel BEFORE publishing the result: the seed path classifies
					// itself as the root cause only when runCtx is still live, so if
					// the send landed first there would be a scheduler window where a
					// concurrently-returning seed sees runCtx live, self-cancels, and
					// masks this receiver termination. The channel is buffered, so
					// canceling first cannot block. Ordering holds for the error
					// branch below for the same reason.
					runCancel()
					streamErr <- nil
					return
				}
				if err != nil {
					runCancel()
					streamErr <- fmt.Errorf("receiving gateway event: %w", err)
					return
				}
				heartbeat()
				rq.enqueue(Event[*pb.Gateway]{
					Type:       toEventType(event.Type),
					ResourceID: event.ResourceId,
					Resource:   event.Gateway,
				})
			}
		}()

		// Seed the queue from the current inventory while the goroutine above
		// drains live events. The watch stream sends only future events and
		// never replays existing state on (re)connect, so this LIST is the
		// only path that recovers a gateway whose reconcile never completed --
		// e.g. one persisted at Provisioning when the controller died before
		// creating its workload or recording a terminal phase. runCtx ties
		// the seed to the receiver: a Recv error cancels runCtx, aborting the
		// seed's in-flight RPCs so mutations during the dead window are not
		// masked by an unchanged stable ID set.
		if err := seedGateways(runCtx, client, rq, clusterID); err != nil {
			// Distinguish two causes so a genuine seed failure is never masked by
			// the cancellation we would cause ourselves. If runCtx is already
			// canceled, the receiver ended first (its Recv error/EOF canceled
			// runCtx, which in turn aborted the seed's in-flight RPCs): the
			// receiver is the root cause, so join it and prefer its error. If
			// runCtx is still live, the seed failed on its own (a real List/Get
			// error): cancel the receiver so its Recv unblocks, join it, and
			// return the seed error -- the receiver's error is only the
			// cancellation we just triggered.
			if runCtx.Err() != nil {
				recvErr := <-streamErr
				if recvErr != nil {
					return recvErr
				}
				return err
			}
			runCancel()
			<-streamErr
			return err
		}

		// The seed completed; wait for the drain goroutine to finish (stream
		// error or EOF). runCancel (via defer) or watchLoop canceling the
		// parent attempt ctx breaks the Recv and lets this return promptly.
		return <-streamErr
	})
}

// clearGatewayPhaseForRetry returns the event with the Gateway's phase cleared so
// a recovery retry bypasses the reconciler's phase gate. The reconciler stamps
// phase=Provisioning before it does the provisioning work, and that DB write emits
// a watch event that coalesces into the queue's latest payload; a retry that
// reused it verbatim would hit the phase gate (Provisioning => skip) and silently
// no-op, re-stranding a gateway whose original provisioning failed before it could
// record a terminal phase. Clearing the phase -- and only on retries -- restores
// the gate-bypassing recovery the watch stream cannot provide, while the rest of
// the payload still reflects the latest observed spec so an un-routed gateway is
// torn down, not resurrected. The event keeps the original phase so the
// reconciler can distinguish a recovery from a first provision. proto.Clone
// avoids mutating the shared latest entry (and copying the message value, which
// vet forbids).
func clearGatewayPhaseForRetry(ev Event[*pb.Gateway]) Event[*pb.Gateway] {
	if ev.Resource == nil {
		return ev
	}
	clone := proto.Clone(ev.Resource).(*pb.Gateway)
	ev.PhaseBeforeRetry = clone.GetPhase()
	clone.Phase = nil
	ev.Resource = clone
	return ev
}

// gatewayEventVersion returns a monotonically increasing version for a gateway
// event, taken from the resource's updated_at (the API server bumps it on every
// write and stamps it into both list and watch payloads). The reconcile queue
// uses it to coalesce to the newest observed state, so a stale seed/resync
// snapshot cannot clobber a newer live event. A missing resource or timestamp
// yields 0, degrading to plain last-writer-wins for that event.
func gatewayEventVersion(ev Event[*pb.Gateway]) int64 {
	return ev.Resource.GetMetadata().GetUpdatedAt().AsTime().UnixNano()
}

// gatewaySeedPageSize is the page size used when listing existing gateways to
// seed the reconcile queue. It matches the reconcilers' list page size so a
// typical fleet is covered in a single request.
const gatewaySeedPageSize = 500

// gatewaySeedSink is the subset of the reconcile queue that seedGateways drives:
// enqueue a gateway for reconciliation (optionally forcing a phase-gate bypass for
// recovery), snapshot the keys the queue still tracks, and prune a key whose
// resource no longer exists -- but only if the current entry is still a non-delete
// event, so a delete enqueued by the concurrent watch receiver is preserved.
type gatewaySeedSink interface {
	enqueue(Event[*pb.Gateway])
	enqueueForced(Event[*pb.Gateway])
	knownKeys() map[string]Event[*pb.Gateway]
	pruneIfNonDelete(id string, snapshot Event[*pb.Gateway]) bool
}

// The reconcile queue is the production gatewaySeedSink. This assertion documents
// that contract and, because reconcileQueue is generic, keeps its seed-only
// methods (enqueueForced, knownKeys, pruneIfNonDelete) recognized as used -- the
// unused linter does not otherwise trace them through the interface for a generic
// type.
var _ gatewaySeedSink = (*reconcileQueue[*pb.Gateway])(nil)

// seedGateways lists the current gateway inventory and enqueues every gateway so
// a controller (re)start re-drives reconciles the watch stream will never replay.
// It is the LIST half of the standard controller LIST-then-WATCH pattern: without
// it, a gateway stranded mid-reconcile by a restart is only ever re-enqueued if
// its spec later changes.
//
// Gateways in a phase the reconciler's phase gate suppresses (Provisioning,
// Degraded) are seeded through enqueueForced, which marks the key so the queue
// clears the phase on the very first handler attempt -- the "forced retry for
// active phases" recovery needs -- so the reconcile actually runs instead of
// being skipped by the gate. The mark is sticky and independent of the payload,
// so a same-version live event buffered during the list cannot overwrite the
// bypass before it is applied. Running gateways are seeded verbatim: their
// provisioning completed, so they hit the gate and no-op, avoiding a needless
// re-provision flap on every reconnect. Gateways with no phase (a create whose
// event was missed while the controller was down) or a terminal Failed phase
// already pass the gate, so they reconcile normally without forcing.
//
// The inventory is gathered with listGatewaysStable, which repeats full passes
// until two consecutive passes agree on the ID set. Offset pagination is not a
// consistent snapshot: a concurrent create or delete shifts offsets, so a single
// pass can silently skip a still-live gateway across a page boundary. On a fresh
// process that gateway has no tracked retry and emits no event (it pre-existed
// the watch), so a single-pass seed would strand it. Requiring a stable pass
// closes that startup gap.
//
// The stable inventory is also authoritative for absence, but only after
// confirmation. A gateway the queue is still retrying but that the inventory
// omits may have been deleted while the stream was disconnected (its delete event
// was never replayed) -- or skipped by an unlucky final pass. Pruning on absence
// alone would cancel a live gateway's only retry, so each omitted, still-tracked
// gateway is confirmed with a point GetGateway and pruned only on a NotFound; any
// other outcome (it still exists, or the confirmation itself failed) leaves the
// retry in place, and a confirmed-live gateway is re-seeded with the payload
// GetGateway returned. Cleanup of any orphaned namespace is left to the
// NamespaceGCReconciler, which rechecks liveness before it deletes -- safer than
// synthesizing a delete here. Absence is only trusted after a successful list:
// any page error aborts before pruning.
func seedGateways(ctx context.Context, client pb.GatewayServiceClient, sink gatewaySeedSink, clusterID string) error {
	inventory, err := listGatewaysStable(ctx, client, clusterID)
	if err != nil {
		return err
	}
	var forced int
	for _, gw := range inventory {
		if enqueueSeed(sink, gw) {
			forced++
		}
	}
	seeded := len(inventory)

	// Prune tracked keys the stable inventory omits -- but confirm each first,
	// because offset pagination can still omit a live gateway (see the doc comment).
	// Any EventDeleted entry is skipped: a pending delete (from the snapshot or
	// enqueued concurrently by the receiver) is left alone so its teardown still
	// runs, and a terminal tombstone left by a completed delete is inert and must
	// persist to keep blocking a stale non-delete resurrection.
	var pruned int
	knownSnapshot := sink.knownKeys()
	for id, ev := range knownSnapshot {
		if _, present := inventory[id]; present || ev.Type == EventDeleted {
			continue
		}
		// The list omitted this tracked gateway; confirm it is truly gone with a
		// point read before dropping its retry.
		resp, err := client.GetGateway(ctx, &pb.GetGatewayRequest{Id: id})
		if err != nil {
			if status.Code(err) != codes.NotFound {
				// Confirmation failed for another reason (transient RPC error, etc.):
				// absence is unproven, so keep the retry; a later seed reconfirms.
				log.Printf("WARN could not confirm gateway %s absence during seed; keeping its retry: %v", id, err)
				continue
			}
			// NotFound: the gateway really is gone. pruneIfNonDelete checks the
			// current queue entry under the lock: if the concurrent receiver
			// enqueued a delete (or a newer update) after the knownKeys snapshot,
			// that entry is preserved so its teardown still runs.
			if sink.pruneIfNonDelete(id, ev) {
				pruned++
			}
			continue
		}
		// The gateway still exists; the paginated list just missed it. Re-seed it
		// with the payload GetGateway returned rather than leaving the tracked
		// entry untouched: its spec may have changed while the stream was
		// disconnected, and because the list missed it there is no buffered watch
		// event to correct a stale payload. Enqueuing the current state keeps the
		// reconcile driving the right desired state (and version-aware coalescing
		// discards it as a no-op if the tracked payload is already newer).
		enqueueSeed(sink, resp.GetGateway())
	}

	log.Printf("INFO seeded %d gateway(s) into reconcile queue on watch (re)connect (%d forced past the phase gate for recovery, %d absent retries pruned)", seeded, forced, pruned)
	return nil
}

// enqueueSeed enqueues one gateway as a seed event and reports whether it was
// forced past the phase gate. A gateway in a gate-suppressed active phase
// (Provisioning, Degraded) is enqueued forced -- the seed of such a gateway is
// exactly a forced retry of a reconcile a restart lost -- so the queue clears its
// phase on the first handler attempt and the reconcile runs instead of being
// skipped by the gate. The phase is cleared by the queue (not baked into the
// payload here) so a same-version live event that later overwrites the payload
// cannot strip the bypass. Every other phase is seeded verbatim.
func enqueueSeed(sink gatewaySeedSink, gw *pb.Gateway) (forced bool) {
	ev := Event[*pb.Gateway]{
		Type:       EventUpdated,
		ResourceID: gw.GetMetadata().GetId(),
		Resource:   gw,
	}
	if forceSeedRecovery(gw) {
		sink.enqueueForced(ev)
		return true
	}
	sink.enqueue(ev)
	return false
}

// listGatewaysStable returns the gateway inventory once two consecutive full
// list passes agree on the set of IDs. Offset pagination is not a consistent
// snapshot -- a concurrent create or delete shifts offsets between page fetches,
// so a single pass can skip a still-live gateway across a page boundary. On a
// fresh process such a gateway is never seeded and, having pre-existed the watch,
// emits no event to recover it. Requiring a stable pass makes a skip observable
// (the ID sets differ) and retried.
//
// If the set never settles within maxSeedListPasses (sustained churn), it returns
// an error rather than seeding from an unstable pass: seeding an incomplete
// inventory could permanently strand a pre-existing gateway, and a healthy watch
// stream that never reconnects would never reseed to correct it. The error aborts
// this connect attempt so watchLoop backs off and retries the whole seed on a
// fresh stream.
func listGatewaysStable(ctx context.Context, client pb.GatewayServiceClient, clusterID string) (map[string]*pb.Gateway, error) {
	const maxSeedListPasses = 5
	var prevIDs map[string]struct{}
	for pass := 1; pass <= maxSeedListPasses; pass++ {
		current, err := listGatewaysOnce(ctx, client, clusterID)
		if err != nil {
			return nil, err
		}
		ids := make(map[string]struct{}, len(current))
		for id := range current {
			ids[id] = struct{}{}
		}
		if prevIDs != nil && sameIDSet(prevIDs, ids) {
			return current, nil
		}
		prevIDs = ids
	}
	return nil, fmt.Errorf("gateway inventory did not stabilize after %d list passes; deferring seed so the watch reconnects and retries", maxSeedListPasses)
}

// listGatewaysOnce performs a single paginated pass over the gateway inventory,
// returning it keyed by ID (which also dedupes an item a concurrent create caused
// to appear on two pages).
func listGatewaysOnce(ctx context.Context, client pb.GatewayServiceClient, clusterID string) (map[string]*pb.Gateway, error) {
	clusterFilter, err := ClusterFilter(clusterID)
	if err != nil {
		return nil, err
	}
	inventory := make(map[string]*pb.Gateway)
	for page := int32(1); ; page++ {
		resp, err := client.ListGateways(ctx, &pb.ListGatewaysRequest{Page: page, Size: gatewaySeedPageSize, ClusterId: clusterFilter})
		if err != nil {
			return nil, fmt.Errorf("listing gateways to seed reconcile queue: %w", err)
		}
		items := resp.GetItems()
		for _, gw := range items {
			inventory[gw.GetMetadata().GetId()] = gw
		}
		// Stop on the authoritative Total, or a short/empty page (defensive, so a
		// misreported Total cannot spin forever) -- mirrors listAllGateways. The
		// Total comparison uses the UNIQUE inventory count, not the raw rows seen:
		// a concurrent delete can shift a row so the same gateway appears on two
		// pages, and counting raw rows would let that duplicate reach Total while a
		// distinct gateway is still missing -- exactly the omission this guards.
		total := int(resp.GetMetadata().GetTotal())
		if len(items) == 0 || len(items) < gatewaySeedPageSize || (total > 0 && len(inventory) >= total) {
			break
		}
	}
	return inventory, nil
}

// sameIDSet reports whether two ID sets are equal.
func sameIDSet(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for id := range a {
		if _, ok := b[id]; !ok {
			return false
		}
	}
	return true
}

// forceSeedRecovery reports whether a seeded gateway must bypass the phase gate
// to recover. Provisioning and Degraded denote reconciliation that never reached
// a healthy steady state, so a restart must re-drive them; the gate would
// otherwise skip these phases. Running is deliberately excluded so healthy
// gateways are not re-provisioned on every reconnect.
func forceSeedRecovery(gw *pb.Gateway) bool {
	switch gw.GetPhase() {
	case string(gatewayhealth.PhaseProvisioning), string(gatewayhealth.PhaseDegraded):
		return true
	default:
		return false
	}
}

// WatchRoleBindings streams the role bindings of this control plane's gateways.
// clusterID scopes the stream server-side: the api-server delivers only bindings
// whose gateway is assigned to that cluster (and no global bindings), and
// requires a registered caller to pass its own id. Like WatchGateways, the filter
// is mandatory; an empty id is rejected with ErrMissingClusterID.
func WatchRoleBindings(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.RoleBinding], clusterID string, idleTimeout, resyncInterval time.Duration) error {
	clusterFilter, err := ClusterFilter(clusterID)
	if err != nil {
		return err
	}
	client := pb.NewRoleBindingServiceClient(conn)
	// Retry assignment until it succeeds. The RoleBinding event often arrives
	// before the Keycloak client (or its roles) exist; a half-provisioned client
	// returns a generic 404 rather than ClientNotFoundError. A finite attempt
	// budget strands the owner without openshell-admin until the watch reconnects.
	// Missing-gateway errors are permanent (the binding's gateway was deleted).
	rq := newReconcileQueue(ctx, "RoleBinding", handler,
		withRetryIf[*pb.RoleBinding](isRoleBindingRetryable))
	defer rq.stop()
	return watchLoop(ctx, "RoleBinding", idleTimeout, func(ctx context.Context, heartbeat func()) error {
		// Derive a cancelable child so the receiver and the seed/resync goroutines
		// can cancel and join each other without waiting for watchLoop to cancel the
		// parent attempt ctx (mirrors WatchGateways).
		runCtx, runCancel := context.WithCancel(ctx)
		defer runCancel()

		stream, err := client.WatchRoleBindings(runCtx, &pb.WatchRoleBindingsRequest{ClusterId: clusterFilter})
		if err != nil {
			return fmt.Errorf("starting role binding watch: %w", err)
		}
		// Wait for the subscription header before seeding, so a binding event that
		// fires during the seed LIST is captured by the watch rather than lost -- the
		// same list-watch gap WatchGateways closes. The api-server flushes the header
		// only after it subscribes to the broker.
		if _, err := stream.Header(); err != nil {
			return fmt.Errorf("awaiting role binding watch subscription header: %w", err)
		}
		heartbeat()

		// Drain live events concurrently with the seed so the broker's bounded
		// per-subscriber buffer does not fill and drop events while the (potentially
		// slow) seed LIST runs.
		streamErr := make(chan error, 1)
		go func() {
			defer close(streamErr)
			for {
				event, err := stream.Recv()
				if err == io.EOF {
					runCancel()
					streamErr <- nil
					return
				}
				if err != nil {
					runCancel()
					streamErr <- fmt.Errorf("receiving role binding event: %w", err)
					return
				}
				heartbeat()
				rq.enqueue(Event[*pb.RoleBinding]{
					Type:       toEventType(event.Type),
					ResourceID: event.ResourceId,
					Resource:   event.RoleBinding,
				})
			}
		}()

		// Seed from the current inventory on every (re)connect. The watch stream
		// sends only future events and never replays existing bindings, so without
		// this LIST a binding whose create event was missed (or arrived on a stream
		// that later went silent) never assigns the owner's Keycloak roles and the
		// gateway console refuses the owner with "role 'openshell-user' required".
		// See specs/platform/control-plane.spec.md ("Role Binding Resync").
		if err := seedRoleBindings(runCtx, client, rq, clusterID); err != nil {
			// Distinguish a receiver-caused cancellation from a genuine seed failure
			// (same reasoning as WatchGateways) so neither masks the other.
			if runCtx.Err() != nil {
				recvErr := <-streamErr
				if recvErr != nil {
					return recvErr
				}
				return err
			}
			runCancel()
			<-streamErr
			return err
		}

		// Periodically re-seed while the stream stays connected, so an owner grant
		// is recovered even when a single event is missed and the stream never
		// reconnects on its own. A transient resync LIST failure is logged and
		// retried on the next tick rather than tearing down a healthy stream.
		resyncDone := make(chan struct{})
		go func() {
			defer close(resyncDone)
			if resyncInterval <= 0 {
				return
			}
			ticker := time.NewTicker(resyncInterval)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					if err := seedRoleBindings(runCtx, client, rq, clusterID); err != nil && runCtx.Err() == nil {
						log.Printf("WARN role binding resync failed; retrying next interval: %v", err)
					}
				}
			}
		}()

		recvErr := <-streamErr
		runCancel()
		<-resyncDone
		return recvErr
	})
}

// roleBindingSeedSink is the subset of the reconcile queue that seedRoleBindings
// drives: enqueue a role binding for reconciliation. A test seam.
type roleBindingSeedSink interface {
	enqueue(Event[*pb.RoleBinding])
}

// The reconcile queue is the production roleBindingSeedSink.
var _ roleBindingSeedSink = (*reconcileQueue[*pb.RoleBinding])(nil)

// seedRoleBindings lists every role binding for this control plane's cluster and
// enqueues each for reconciliation, so the owner's Keycloak client roles are
// (re)assigned even when the WatchRoleBindings stream missed the binding's create
// event or went silent. It is the LIST half of the list-then-watch pattern the
// gateway watch already uses (seedGateways).
//
// Unlike seedGateways it does NOT prune: a plain ListRoleBindings reports only
// surviving bindings, so an absence cannot be distinguished from a binding the
// list simply did not return, and the stream remains the authority for deletions
// (and thus for revoking roles a deleted binding no longer backs). The resync only
// restores missing assignments; the per-event union reconcile (GTW-ACC-02) makes every
// enqueue idempotent, so re-seeding bindings already reflected in Keycloak is a
// no-op. See specs/platform/control-plane.spec.md ("Role Binding Resync").
func seedRoleBindings(ctx context.Context, client pb.RoleBindingServiceClient, sink roleBindingSeedSink, clusterID string) error {
	clusterFilter, err := ClusterFilter(clusterID)
	if err != nil {
		return err
	}
	resp, err := client.ListRoleBindings(ctx, &pb.ListRoleBindingsRequest{ClusterId: clusterFilter})
	if err != nil {
		return fmt.Errorf("listing role bindings to seed reconcile queue: %w", err)
	}
	var seeded int
	for _, rb := range resp.GetItems() {
		if rb == nil {
			continue
		}
		sink.enqueue(Event[*pb.RoleBinding]{
			Type:       EventUpdated,
			ResourceID: rb.GetMetadata().GetId(),
			Resource:   rb,
		})
		seeded++
	}
	log.Printf("INFO seeded %d role binding(s) into reconcile queue on role binding watch (re)connect/resync", seeded)
	return nil
}

// WatchAgentRuntimes streams AgentRuntime events for this control plane's own
// cluster and dispatches each to the handler through a reconcile queue. The
// stream is cluster-scoped: the api-server resolves each AgentRuntime's effective
// cluster (its own cluster_id, or the cluster of its referenced gateway) and
// delivers only the records that match this control plane's registered
// cluster_id (managed-cluster-registration.spec.md, "Watch Stream Caller
// Binding"). On every (re)connect it seeds from ListAgentRuntimes so an event
// missed while the stream was down is still reconciled, mirroring WatchGateways
// and WatchRoleBindings; it reuses the shared watchLoop reconnect/backoff, the
// idle watchdog, and the connection-level HTTP/2 keepalive.
//
// The handler is the Wave A no-op reconciler (issue #521); cluster-side and
// gateway-side provisioning are deferred to Wave B.
func WatchAgentRuntimes(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.AgentRuntime], clusterID string, idleTimeout time.Duration) error {
	clusterFilter, err := ClusterFilter(clusterID)
	if err != nil {
		return err
	}
	client := pb.NewAgentRuntimeServiceClient(conn)
	rq := newReconcileQueue(ctx, "AgentRuntime", handler)
	defer rq.stop()
	return watchLoop(ctx, "AgentRuntime", idleTimeout, func(ctx context.Context, heartbeat func()) error {
		// Derive a cancelable child so the receiver and the seed goroutine can
		// cancel and join each other without waiting for watchLoop to cancel the
		// parent attempt ctx (mirrors WatchGateways and WatchRoleBindings).
		runCtx, runCancel := context.WithCancel(ctx)
		defer runCancel()

		stream, err := client.WatchAgentRuntimes(runCtx, &pb.WatchAgentRuntimesRequest{ClusterId: clusterFilter})
		if err != nil {
			return fmt.Errorf("starting agent runtime watch: %w", err)
		}
		// Wait for the subscription header before seeding, so an AgentRuntime event
		// that fires during the seed LIST is captured by the watch rather than lost
		// -- the same list-watch gap WatchGateways closes. The api-server flushes the
		// header only after it subscribes to the broker.
		if _, err := stream.Header(); err != nil {
			return fmt.Errorf("awaiting agent runtime watch subscription header: %w", err)
		}
		heartbeat()

		// Drain live events concurrently with the seed so the broker's bounded
		// per-subscriber buffer does not fill and drop events while the (potentially
		// slow) seed LIST runs.
		streamErr := make(chan error, 1)
		go func() {
			defer close(streamErr)
			for {
				event, err := stream.Recv()
				if err == io.EOF {
					runCancel()
					streamErr <- nil
					return
				}
				if err != nil {
					runCancel()
					streamErr <- fmt.Errorf("receiving agent runtime event: %w", err)
					return
				}
				heartbeat()
				rq.enqueue(Event[*pb.AgentRuntime]{
					Type:       toEventType(event.Type),
					ResourceID: event.ResourceId,
					Resource:   event.AgentRuntime,
				})
			}
		}()

		// Seed from the current inventory on every (re)connect. The watch stream
		// sends only future events and never replays existing runtimes, so without
		// this LIST an AgentRuntime whose create event was missed (or arrived on a
		// stream that later went silent) is never reconciled.
		if err := seedAgentRuntimes(runCtx, client, rq, clusterID); err != nil {
			// Distinguish a receiver-caused cancellation from a genuine seed failure
			// (same reasoning as WatchGateways) so neither masks the other.
			if runCtx.Err() != nil {
				recvErr := <-streamErr
				if recvErr != nil {
					return recvErr
				}
				return err
			}
			runCancel()
			<-streamErr
			return err
		}

		recvErr := <-streamErr
		runCancel()
		return recvErr
	})
}

// agentRuntimeSeedSink is the subset of the reconcile queue that
// seedAgentRuntimes drives: enqueue an AgentRuntime for reconciliation. A test
// seam.
type agentRuntimeSeedSink interface {
	enqueue(Event[*pb.AgentRuntime])
}

// The reconcile queue is the production agentRuntimeSeedSink.
var _ agentRuntimeSeedSink = (*reconcileQueue[*pb.AgentRuntime])(nil)

// seedAgentRuntimes lists every AgentRuntime for this control plane's cluster and
// enqueues each for reconciliation, so a runtime whose create event the watch
// missed or dropped while silent is still reconciled. It is the LIST half of the
// list-then-watch pattern the gateway and role-binding watches already use.
//
// Like seedRoleBindings it does NOT prune: a plain ListAgentRuntimes reports only
// surviving runtimes, so an absence cannot be distinguished from a runtime the
// list simply did not return, and the stream remains the authority for deletions.
func seedAgentRuntimes(ctx context.Context, client pb.AgentRuntimeServiceClient, sink agentRuntimeSeedSink, clusterID string) error {
	clusterFilter, err := ClusterFilter(clusterID)
	if err != nil {
		return err
	}
	resp, err := client.ListAgentRuntimes(ctx, &pb.ListAgentRuntimesRequest{ClusterId: clusterFilter})
	if err != nil {
		return fmt.Errorf("listing agent runtimes to seed reconcile queue: %w", err)
	}
	var seeded int
	for _, ar := range resp.GetItems() {
		if ar == nil {
			continue
		}
		sink.enqueue(Event[*pb.AgentRuntime]{
			Type:       EventUpdated,
			ResourceID: ar.GetMetadata().GetId(),
			Resource:   ar,
		})
		seeded++
	}
	log.Printf("INFO seeded %d agent runtime(s) into reconcile queue on agent runtime watch (re)connect", seeded)
	return nil
}

func isMissingKeycloakClient(err error) bool {
	var notFound *keycloak.ClientNotFoundError
	return errors.As(err, &notFound)
}

func isMissingKeycloakUser(err error) bool {
	var notFound *keycloak.UserNotFoundError
	return errors.As(err, &notFound)
}

func isMissingGateway(err error) bool {
	for err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return true
		}
		err = errors.Unwrap(err)
	}
	return false
}

func isUnresolvedRoleBindingIdentity(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "username not yet resolved") ||
		strings.Contains(msg, "role name not yet resolved")
}

func isKeycloakHTTPNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "returned 404")
}

// isRoleBindingRetryable reports whether a RoleBinding reconcile failure should
// be retried. Missing client, missing user, generic Keycloak 404
// (half-provisioned roles), and unresolved username/role are provisioning
// races. Missing-gateway, auth, and config errors are terminal.
func isRoleBindingRetryable(err error) bool {
	if err == nil || isMissingGateway(err) {
		return false
	}
	return isMissingKeycloakClient(err) ||
		isMissingKeycloakUser(err) ||
		isUnresolvedRoleBindingIdentity(err) ||
		isKeycloakHTTPNotFound(err)
}

// watchLoop connects a watch stream, drives connectAndRecv, and reconnects with
// exponential backoff on any return. connectAndRecv receives a heartbeat callback
// it MUST call on every received event (and on the subscription header) so the
// idle watchdog can tell a healthy-but-quiet stream from a silently wedged one;
// idleTimeout <= 0 disables the watchdog.
func watchLoop(ctx context.Context, kind string, idleTimeout time.Duration, connectAndRecv func(ctx context.Context, heartbeat func()) error) error {
	backoff := time.Second
	maxBackoff := 30 * time.Second
	reconnected := false

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Printf("INFO connecting %s watch stream...", kind)
		// Scope each attempt to its own cancelable context so the RPC -- and its
		// server-side broker subscription -- is torn down before we reconnect.
		// connectAndRecv can return while the stream is still healthy (e.g. a
		// post-header seed LIST failed), so relying on the stream erroring to end
		// the RPC would leak one subscription per reconnect. Any reconcile queue is
		// created on the parent ctx, not this per-attempt one, so pending retries
		// survive the reconnect.
		attemptCtx, cancel := context.WithCancel(ctx)

		streamCtx, endSpan := cpotel.StartWatchSpan(attemptCtx, kind)

		if reconnected {
			cpotel.RecordWatchReconnect(ctx, kind)
		}

		// Idle watchdog: a long-lived stream can stop delivering events without
		// returning an error (an intermediary drops the server-to-client flow of an
		// otherwise live connection), leaving a blocked Recv that never reconnects
		// and never re-seeds. gRPC keepalive cannot catch this on its own because
		// the shared connection stays live. The watchdog cancels this attempt -- which
		// breaks the Recv and so triggers a reconnect and reseed -- when no activity
		// is reported for idleTimeout; each received event resets it via heartbeat.
		// See specs/platform/control-plane.spec.md ("Watch Stream Liveness").
		heartbeat, stopWatchdog := startIdleWatchdog(streamCtx, kind, idleTimeout, cancel)

		err := connectAndRecv(streamCtx, heartbeat)
		cancel()
		stopWatchdog()
		if ctx.Err() != nil {
			endSpan(nil)
			return ctx.Err()
		}

		endSpan(err)

		reconnected = true
		log.Printf("WARN %s watch stream disconnected: %v; reconnecting in %v", kind, err, backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// startIdleWatchdog launches a timer that calls cancel when no heartbeat is
// observed for idleTimeout, so a silent-but-open stream is torn down and the
// watch loop reconnects. It returns a heartbeat func the caller invokes on every
// received event (and on the subscription header) to reset the timer, and a stop
// func the caller MUST invoke once the attempt ends. The caller is expected to
// cancel ctx before calling stop (watchLoop does), which lets the watchdog
// goroutine exit; stop then joins it. A non-positive idleTimeout disables the
// watchdog entirely: heartbeat is a no-op and stop returns immediately.
func startIdleWatchdog(ctx context.Context, kind string, idleTimeout time.Duration, cancel context.CancelFunc) (heartbeat func(), stop func()) {
	if idleTimeout <= 0 {
		return func() {}, func() {}
	}
	beat := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(idleTimeout)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-beat:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(idleTimeout)
			case <-timer.C:
				log.Printf("WARN %s watch stream idle for %s with no events or error; cancelling the attempt to reconnect and reseed", kind, idleTimeout)
				cancel()
				return
			}
		}
	}()
	heartbeat = func() {
		select {
		case beat <- struct{}{}:
		default:
		}
	}
	stop = func() { <-done }
	return heartbeat, stop
}
