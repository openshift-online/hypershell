package watcher

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"google.golang.org/grpc"
)

// WatchDefaultWorkspaceMemberships streams DefaultWorkspaceMembership events
// for this control plane's own cluster (data-model.spec.md, PLAT-DATA-27) and
// dispatches each to the handler through a reconcile queue keyed by GATEWAY id,
// not by the membership row's own id. CTRL-CORE-20 reconciles a gateway's FULL
// live membership set as one unit, so keying the queue by gateway gives the
// "coalesced per gateway" batching the spec requires for free: multiple
// membership events for the same gateway arriving close together collapse into
// one Handle call against the latest desired set, using the same per-key
// coalescing discipline the gateway reconcile queue already uses
// (gateway-reconcile-concurrency.spec.md).
//
// Handle always re-lists the full desired set for its gateway (see
// DefaultWorkspaceMembershipReconciler), so the individual row carried as an
// event's Resource is not load-bearing for reconciliation; it is still
// delivered so the OBS-CRM-14 pending/failed gauges can be kept current
// independently of whether an OpenShell session factory is configured.
func WatchDefaultWorkspaceMemberships(ctx context.Context, conn *grpc.ClientConn, handler Handler[*pb.DefaultWorkspaceMembership], clusterID string, idleTimeout, resyncInterval time.Duration) error {
	clusterFilter, err := ClusterFilter(clusterID)
	if err != nil {
		return err
	}
	client := pb.NewDefaultWorkspaceMembershipServiceClient(conn)
	rq := newReconcileQueue(ctx, "DefaultWorkspaceMembership", handler)
	defer rq.stop()
	return watchLoop(ctx, "DefaultWorkspaceMembership", idleTimeout, func(ctx context.Context, heartbeat func()) error {
		runCtx, runCancel := context.WithCancel(ctx)
		defer runCancel()

		stream, err := client.WatchDefaultWorkspaceMemberships(runCtx, &pb.WatchDefaultWorkspaceMembershipsRequest{ClusterId: clusterFilter})
		if err != nil {
			return fmt.Errorf("starting default workspace membership watch: %w", err)
		}
		if _, err := stream.Header(); err != nil {
			return fmt.Errorf("awaiting default workspace membership watch subscription header: %w", err)
		}
		heartbeat()

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
					streamErr <- fmt.Errorf("receiving default workspace membership event: %w", err)
					return
				}
				heartbeat()
				enqueueDefaultWorkspaceMembershipEvent(ctx, clusterID, rq, toEventType(event.Type), event.ResourceId, event.DefaultWorkspaceMembership)
			}
		}()

		if err := seedDefaultWorkspaceMemberships(runCtx, client, rq, clusterID); err != nil {
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
					if err := seedDefaultWorkspaceMemberships(runCtx, client, rq, clusterID); err != nil && runCtx.Err() == nil {
						slog.WarnContext(runCtx, "Default workspace membership resync failed, retrying next interval", "error", err)
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

type defaultWorkspaceMembershipSeedSink interface {
	enqueue(Event[*pb.DefaultWorkspaceMembership])
}

var _ defaultWorkspaceMembershipSeedSink = (*reconcileQueue[*pb.DefaultWorkspaceMembership])(nil)

// seedDefaultWorkspaceMemberships lists every DefaultWorkspaceMembership for
// this control plane's cluster and enqueues each for reconciliation, so a
// gateway whose membership set changed while the watch stream was down (or
// whose create/delete event the stream missed) still converges, mirroring
// seedRoleBindings.
func seedDefaultWorkspaceMemberships(ctx context.Context, client pb.DefaultWorkspaceMembershipServiceClient, sink defaultWorkspaceMembershipSeedSink, clusterID string) error {
	clusterFilter, err := ClusterFilter(clusterID)
	if err != nil {
		return err
	}
	resp, err := client.ListDefaultWorkspaceMemberships(ctx, &pb.ListDefaultWorkspaceMembershipsRequest{ClusterId: clusterFilter})
	if err != nil {
		return fmt.Errorf("listing default workspace memberships to seed reconcile queue: %w", err)
	}
	var seeded int
	for _, m := range resp.GetItems() {
		if m == nil {
			continue
		}
		enqueueDefaultWorkspaceMembershipEvent(ctx, clusterID, sink, EventUpdated, m.GetGatewayId(), m)
		seeded++
	}
	slog.InfoContext(ctx, "Seeded default workspace memberships into reconcile queue on watch (re)connect/resync", "count", seeded)
	return nil
}

// enqueueDefaultWorkspaceMembershipEvent enqueues ev keyed by the membership's
// gateway id (not its own id, per the per-gateway batching this Kind requires)
// and republishes the OBS-CRM-14 pending/failed gauges for m's observed status,
// independently of reconciliation (which may be inert when no OpenShell session
// factory is configured).
func enqueueDefaultWorkspaceMembershipEvent(ctx context.Context, clusterID string, sink defaultWorkspaceMembershipSeedSink, eventType EventType, fallbackGatewayID string, m *pb.DefaultWorkspaceMembership) {
	gatewayID := fallbackGatewayID
	if m != nil && m.GetGatewayId() != "" {
		gatewayID = m.GetGatewayId()
	}
	if gatewayID == "" {
		return
	}
	sink.enqueue(Event[*pb.DefaultWorkspaceMembership]{
		Type:       eventType,
		ResourceID: gatewayID,
		Resource:   m,
	})
	if eventType == EventDeleted {
		if m != nil {
			cpotel.RecordDefaultWorkspaceMembershipState(ctx, clusterID, m.GetMetadata().GetId(), "", true)
		}
		return
	}
	if m != nil {
		cpotel.RecordDefaultWorkspaceMembershipState(ctx, clusterID, m.GetMetadata().GetId(), m.GetReconciliationStatus(), false)
	}
}
