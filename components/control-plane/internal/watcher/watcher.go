package watcher

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
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

type Event[T any] struct {
	Type       EventType
	ResourceID string
	Resource   T
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

func WatchFleets(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.Fleet]) error {
	client := pb.NewFleetServiceClient(conn)
	return watchLoop(ctx, "Fleet", func(ctx context.Context) error {
		stream, err := client.WatchFleets(ctx, &pb.WatchFleetsRequest{})
		if err != nil {
			return fmt.Errorf("starting fleet watch: %w", err)
		}
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("receiving fleet event: %w", err)
			}
			if err := handler.Handle(ctx, Event[*pb.Fleet]{
				Type:       toEventType(event.Type),
				ResourceID: event.ResourceId,
				Resource:   event.Fleet,
			}); err != nil {
				log.Printf("ERROR handling fleet %s: %v", event.ResourceId, err)
			}
		}
	})
}

func WatchManagedClusters(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.ManagedCluster]) error {
	client := pb.NewManagedClusterServiceClient(conn)
	return watchLoop(ctx, "ManagedCluster", func(ctx context.Context) error {
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

func WatchManagedDatabases(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.ManagedDatabase]) error {
	client := pb.NewManagedDatabaseServiceClient(conn)
	return watchLoop(ctx, "ManagedDatabase", func(ctx context.Context) error {
		// Derive a cancelable child so the concurrent seed below is torn down when
		// the receiver ends (stream error/EOF), and vice versa.
		runCtx, runCancel := context.WithCancel(ctx)
		defer runCancel()

		stream, err := client.WatchManagedDatabases(runCtx, &pb.WatchManagedDatabasesRequest{})
		if err != nil {
			return fmt.Errorf("starting managed database watch: %w", err)
		}

		// Block on the stream header before seeding. Opening the stream is not a
		// subscription handshake: client.WatchManagedDatabases can return before the
		// server registers its broker subscription, so a seed issued immediately
		// could LIST state, then miss an event that fires before the subscription
		// goes live. The server flushes the header only after it has subscribed (see
		// the WatchManagedDatabases handler), so blocking here closes that
		// list-watch gap -- the seed's LIST captures everything before this point
		// and the watch captures everything after.
		if _, err := stream.Header(); err != nil {
			return fmt.Errorf("awaiting managed database watch subscription header: %w", err)
		}

		// Seed the reconciler from the current inventory while the receiver below
		// drains live events. The watch stream sends only future events and never
		// replays existing state on (re)connect, so this LIST is the only path that
		// recovers a ManagedDatabase whose create event fired while the stream was
		// disconnected -- e.g. the openshell-db database kind-up seeds while a
		// component image swap is restarting the api-server. Seeding runs
		// concurrently so a slow provisioning reconcile never stalls the live drain
		// (which would let the api server's per-subscriber event buffer overflow and
		// drop events permanently).
		seedDone := make(chan struct{})
		go func() {
			defer close(seedDone)
			if err := seedManagedDatabases(runCtx, client, handler); err != nil {
				log.Printf("WARN seeding ManagedDatabases on watch (re)connect: %v", err)
			}
		}()

		recvErr := func() error {
			for {
				event, err := stream.Recv()
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return fmt.Errorf("receiving managed database event: %w", err)
				}
				if err := handler.Handle(ctx, Event[*pb.ManagedDatabase]{
					Type:       toEventType(event.Type),
					ResourceID: event.ResourceId,
					Resource:   event.ManagedDatabase,
				}); err != nil {
					log.Printf("ERROR handling managed database %s: %v", event.ResourceId, err)
				}
			}
		}()

		// The receiver ended; cancel and join the seed so a slow in-flight seed
		// reconcile does not outlive this connect attempt.
		runCancel()
		<-seedDone
		return recvErr
	})
}

// managedDatabaseSeedPageSize is the page size used when listing existing
// ManagedDatabases to seed the reconciler. It matches the API server's maximum
// page size so a typical fleet is covered in a single request.
const managedDatabaseSeedPageSize = 500

// seedManagedDatabases lists the current ManagedDatabase inventory and drives a
// reconcile for each, recovering any whose create event the watch stream will
// never replay (it sends only future events on (re)connect). It is the LIST half
// of the standard controller LIST-then-WATCH pattern.
//
// Unlike seedGateways this needs no phase-gate bypass, no forced retries, and no
// absence pruning: the ManagedDatabase reconciler is level-based and idempotent
// (it checks the actual CNPG cluster's readiness and no-ops an already-ready
// database), and it holds no long-lived per-resource retry that a stale seed
// could strand. A single paginated pass is likewise sufficient rather than the
// stable-pass repetition seedGateways needs -- ManagedDatabases are few (about
// one per fleet) and fit a single page, so offset-pagination skew cannot silently
// omit one across a page boundary.
func seedManagedDatabases(ctx context.Context, client pb.ManagedDatabaseServiceClient, handler Handler[*pb.ManagedDatabase]) error {
	inventory, err := listAllManagedDatabases(ctx, client)
	if err != nil {
		return err
	}
	for _, db := range inventory {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ev := Event[*pb.ManagedDatabase]{
			Type:       EventUpdated,
			ResourceID: db.GetMetadata().GetId(),
			Resource:   db,
		}
		if err := handler.Handle(ctx, ev); err != nil {
			log.Printf("ERROR seeding managed database %s: %v", ev.ResourceID, err)
		}
	}
	log.Printf("INFO seeded %d managed database(s) into reconciler on watch (re)connect", len(inventory))
	return nil
}

// listAllManagedDatabases pages through the gRPC ManagedDatabase inventory and
// returns every database. The list endpoint is server-side paginated, so a
// single unpaged request cannot be relied on to return the whole set.
func listAllManagedDatabases(ctx context.Context, client pb.ManagedDatabaseServiceClient) ([]*pb.ManagedDatabase, error) {
	var all []*pb.ManagedDatabase
	for page := int32(1); ; page++ {
		resp, err := client.ListManagedDatabases(ctx, &pb.ListManagedDatabasesRequest{
			Page: page,
			Size: managedDatabaseSeedPageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("listing managed databases to seed reconciler: %w", err)
		}
		items := resp.GetItems()
		all = append(all, items...)

		// Stop on the authoritative Total, or a short/empty page (defensive, so a
		// misreported Total cannot spin forever) -- mirrors listAllGateways.
		total := int(resp.GetMetadata().GetTotal())
		if len(items) == 0 || len(items) < managedDatabaseSeedPageSize || (total > 0 && len(all) >= total) {
			return all, nil
		}
	}
}

func WatchGatewayReleases(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.GatewayRelease]) error {
	client := pb.NewGatewayReleaseServiceClient(conn)
	return watchLoop(ctx, "GatewayRelease", func(ctx context.Context) error {
		stream, err := client.WatchGatewayReleases(ctx, &pb.WatchGatewayReleasesRequest{})
		if err != nil {
			return fmt.Errorf("starting gateway release watch: %w", err)
		}
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("receiving gateway release event: %w", err)
			}
			if err := handler.Handle(ctx, Event[*pb.GatewayRelease]{
				Type:       toEventType(event.Type),
				ResourceID: event.ResourceId,
				Resource:   event.GatewayRelease,
			}); err != nil {
				log.Printf("ERROR handling gateway release %s: %v", event.ResourceId, err)
			}
		}
	})
}

func WatchGateways(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.Gateway]) error {
	client := pb.NewGatewayServiceClient(conn)
	// Gateway reconciliation is driven through a per-resource reconcile queue rather
	// than invoked inline: the watch stream does not replay state on reconnect, so a
	// reconcile that fails (e.g. an API-server outage that also blocks recording a
	// Failed phase) would otherwise strand the gateway until its spec next changes.
	// The queue serializes work per gateway, coalesces to the latest observed state,
	// and retries failures indefinitely with capped backoff -- all on the watcher
	// lifetime context so recovery survives a stream reconnect.
	rq := newReconcileQueue(ctx, "Gateway", handler,
		withRetryTransform(clearGatewayPhaseForRetry),
		withVersion(gatewayEventVersion))
	defer rq.stop()
	return watchLoop(ctx, "Gateway", func(ctx context.Context) error {
		// Derive a cancelable child before creating the stream so either the
		// receiver or the seed can cancel and join the other without waiting
		// for watchLoop to cancel the parent attempt ctx.
		runCtx, runCancel := context.WithCancel(ctx)
		defer runCancel()

		stream, err := client.WatchGateways(runCtx, &pb.WatchGatewaysRequest{})
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
		if err := seedGateways(runCtx, client, rq); err != nil {
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
// torn down, not resurrected. proto.Clone avoids mutating the shared latest entry
// (and copying the message value, which vet forbids).
func clearGatewayPhaseForRetry(ev Event[*pb.Gateway]) Event[*pb.Gateway] {
	if ev.Resource == nil {
		return ev
	}
	clone := proto.Clone(ev.Resource).(*pb.Gateway)
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
func seedGateways(ctx context.Context, client pb.GatewayServiceClient, sink gatewaySeedSink) error {
	inventory, err := listGatewaysStable(ctx, client)
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
func listGatewaysStable(ctx context.Context, client pb.GatewayServiceClient) (map[string]*pb.Gateway, error) {
	const maxSeedListPasses = 5
	var prevIDs map[string]struct{}
	for pass := 1; pass <= maxSeedListPasses; pass++ {
		current, err := listGatewaysOnce(ctx, client)
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
func listGatewaysOnce(ctx context.Context, client pb.GatewayServiceClient) (map[string]*pb.Gateway, error) {
	inventory := make(map[string]*pb.Gateway)
	for page := int32(1); ; page++ {
		resp, err := client.ListGateways(ctx, &pb.ListGatewaysRequest{Page: page, Size: gatewaySeedPageSize})
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
	case "Provisioning", "Degraded":
		return true
	default:
		return false
	}
}

func WatchGatewayNetworks(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.GatewayNetwork]) error {
	client := pb.NewGatewayNetworkServiceClient(conn)
	return watchLoop(ctx, "GatewayNetwork", func(ctx context.Context) error {
		stream, err := client.WatchGatewayNetworks(ctx, &pb.WatchGatewayNetworksRequest{})
		if err != nil {
			return fmt.Errorf("starting gateway network watch: %w", err)
		}
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("receiving gateway network event: %w", err)
			}
			if err := handler.Handle(ctx, Event[*pb.GatewayNetwork]{
				Type:       toEventType(event.Type),
				ResourceID: event.ResourceId,
				Resource:   event.GatewayNetwork,
			}); err != nil {
				log.Printf("ERROR handling gateway network %s: %v", event.ResourceId, err)
			}
		}
	})
}

func WatchRoleBindings(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.RoleBinding]) error {
	client := pb.NewRoleBindingServiceClient(conn)
	return watchLoop(ctx, "RoleBinding", func(ctx context.Context) error {
		stream, err := client.WatchRoleBindings(ctx, &pb.WatchRoleBindingsRequest{})
		if err != nil {
			return fmt.Errorf("starting role binding watch: %w", err)
		}
		for {
			event, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("receiving role binding event: %w", err)
			}
			if err := handler.Handle(ctx, Event[*pb.RoleBinding]{
				Type:       toEventType(event.Type),
				ResourceID: event.ResourceId,
				Resource:   event.RoleBinding,
			}); err != nil {
				log.Printf("ERROR handling role binding %s: %v", event.ResourceId, err)
			}
		}
	})
}

func watchLoop(ctx context.Context, kind string, connectAndRecv func(ctx context.Context) error) error {
	backoff := time.Second
	maxBackoff := 30 * time.Second

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
		err := connectAndRecv(attemptCtx)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}

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
