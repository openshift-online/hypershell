package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
)

// BackfillInstanceLabels stamps this control-plane instance's identity label onto
// the legacy gateway namespaces it already owns per its API server, so periodic
// GC -- which selects on hypershell.redhat.io/instance -- can reclaim them once
// their Gateway is deleted. It is a one-shot startup task.
//
// The normal reconcile path (EnsureManagedNamespace) migrates a live gateway's
// pre-label namespace only when the Gateway is actually reconciled. But a Gateway
// in a steady-state phase (Running/Provisioning/Degraded) is skipped by the
// reconciler's phase gate on (re)start, so its namespace would otherwise never
// gain the instance label and a later missed-delete would leak it past the
// instance-scoped sweep -- precisely the namespaces GC exists to reap. This
// backfill closes that gap by driving the migration from the Gateway inventory
// directly rather than through the gated reconcile.
//
// It is DB-driven and safe on shared clusters: it only ever touches namespaces a
// Gateway in THIS instance's API server records, and even then only namespaces
// that already carry both management labels and no foreign instance label (see
// gateway.BackfillInstanceLabel). Namespaces this instance does not know about --
// including those another HyperShell owns -- are never read or written. It is
// idempotent and best-effort: a per-namespace failure is collected and the sweep
// continues, so one unreachable namespace cannot block the rest. It returns the
// number of namespaces newly labeled and a joined error of any per-namespace
// failures.
//
// Namespaces already orphaned (no Gateway row) before this instance ever labeled
// them cannot be reclaimed here: with no Gateway recording their name and only the
// two generic management labels, they are indistinguishable from another
// HyperShell's namespaces on a shared cluster, so claiming them is unsafe. Those
// require manual cleanup.
func BackfillInstanceLabels(ctx context.Context, client kubernetes.Interface, gwClient pb.GatewayServiceClient, instance string) (int, error) {
	if instance == "" {
		return 0, fmt.Errorf("refusing to backfill instance labels without a control-plane instance identity")
	}
	gateways, err := listAllGateways(ctx, gwClient)
	if err != nil {
		// A partial inventory would silently skip namespaces that need the label,
		// leaving them to leak; fail so the caller can log and retry on the next
		// restart rather than accept an incomplete backfill.
		return 0, fmt.Errorf("list gateways for instance-label backfill: %w", err)
	}

	var errs error
	labeled := 0
	seen := make(map[string]struct{}, len(gateways))
	for _, gw := range gateways {
		ns := gw.GetNamespace()
		if ns == "" {
			// A gateway with no recorded namespace gives nothing deterministic to
			// label; the reconciler assigns and labels it when the Gateway
			// provisions. Never synthesize a namespace name here.
			continue
		}
		if _, dup := seen[ns]; dup {
			continue
		}
		seen[ns] = struct{}{}

		did, err := gateway.BackfillInstanceLabel(ctx, client, ns, instance)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("namespace %s: %w", ns, err))
			continue
		}
		if did {
			labeled++
		}
	}
	return labeled, errs
}

// RunInstanceLabelBackfill performs the startup instance-label backfill against a
// live gRPC connection and logs a summary. It is best-effort: a failure is logged
// but never aborts controller startup, because the periodic GC sweep still
// functions for already-labeled namespaces and the backfill is retried on the
// next restart.
func RunInstanceLabelBackfill(ctx context.Context, client kubernetes.Interface, conn *grpc.ClientConn, instance string) {
	gwClient := pb.NewGatewayServiceClient(conn)
	labeled, err := BackfillInstanceLabels(ctx, client, gwClient, instance)
	if err != nil {
		log.Printf("WARN instance-label backfill completed with errors (labeled=%d): %v", labeled, err)
		return
	}
	log.Printf("INFO instance-label backfill complete: labeled %d legacy namespace(s) for instance %s", labeled, instance)
}
