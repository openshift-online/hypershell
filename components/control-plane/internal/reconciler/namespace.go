package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// defaultNamespaceGCInterval is the cadence at which the control plane sweeps
	// managed gateway namespaces looking for orphans.
	defaultNamespaceGCInterval = 5 * time.Minute

	// defaultNamespaceGCGracePeriod is how long a namespace must remain orphaned
	// (no live Gateway) before it is deleted. The delay avoids reaping a
	// namespace during a transient window (e.g. a delete event that is quickly
	// followed by a recreate, or an API server blip).
	defaultNamespaceGCGracePeriod = 10 * time.Minute

	// gatewayListTimeout bounds each paginated Gateway inventory read so a stalled
	// API server cannot block an entire GC sweep forever.
	gatewayListTimeout = 30 * time.Second

	// namespaceListTimeout bounds each managed-namespace list operation so a hung
	// API server cannot block the GC loop.
	namespaceListTimeout = 30 * time.Second

	// namespaceOperationTimeout bounds each per-namespace API operation (label
	// patches, summaries, event writes, and deletes). On timeout, the sweep item
	// is deferred and retried later.
	namespaceOperationTimeout = 30 * time.Second
)

// NamespaceGCReconciler periodically garbage-collects gateway namespaces that
// this control-plane instance created but that no longer have a live Gateway in
// this instance's API server. Other HyperShell instances on the same cluster are
// ignored: the sweep selects on hypershell.redhat.io/instance=<HYPERSHELL_NAMESPACE>.
// Legacy gateway namespaces that predate the instance label must be labeled
// manually (both management labels, no instance label) before they become
// visible to a sweep; this reconciler never claims them itself. This reaps
// namespaces orphaned by a delete event missed while the control plane was
// down, and namespaces whose gateway failed to bootstrap and was then deleted.
// Reaping is best-effort and idempotent, and is delayed by a grace period
// recorded on the namespace itself so it survives restarts.
//
// See openshell-gateway-namespace-gc.spec.md (HYPERSHELL-78).
type NamespaceGCReconciler struct {
	client      kubernetes.Interface
	grpcConn    *grpc.ClientConn
	interval    time.Duration
	gracePeriod time.Duration
	// cpNamespace is the control-plane namespace where GC Events are recorded so
	// they outlive the namespace being deleted.
	cpNamespace string
	// now is overridable in tests for deterministic grace-period evaluation.
	now func() time.Time
	// liveNamespaces returns the set of namespaces currently backed by a live
	// Gateway. It seeds each sweep and, critically, is re-read immediately before
	// a destructive delete to guard against a Gateway created for the namespace
	// after the sweep captured its (now stale) live set. It is a seam so the
	// reap logic is testable without a live gRPC server; the default pages the
	// whole Gateway fleet.
	liveNamespaces func(ctx context.Context) (map[string]struct{}, error)
}

// NewNamespaceGCReconciler builds a NamespaceGCReconciler, applying defaults for
// any non-positive interval or grace period.
func NewNamespaceGCReconciler(client kubernetes.Interface, grpcConn *grpc.ClientConn, interval, gracePeriod time.Duration, cpNamespace string) *NamespaceGCReconciler {
	if interval <= 0 {
		interval = defaultNamespaceGCInterval
	}
	if gracePeriod <= 0 {
		gracePeriod = defaultNamespaceGCGracePeriod
	}
	r := &NamespaceGCReconciler{
		client:      client,
		grpcConn:    grpcConn,
		interval:    interval,
		gracePeriod: gracePeriod,
		cpNamespace: cpNamespace,
		now:         time.Now,
	}
	r.liveNamespaces = r.grpcLiveNamespaces
	return r
}

// Run drives the garbage-collection loop until the context is cancelled.
func (r *NamespaceGCReconciler) Run(ctx context.Context) error {
	log.Printf("INFO namespace GC reconciler started (interval=%s grace=%s instance=%s)", r.interval, r.gracePeriod, r.cpNamespace)
	r.reconcileOnce(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.reconcileOnce(ctx)
		}
	}
}

func (r *NamespaceGCReconciler) reconcileOnce(ctx context.Context) {
	ctx, endSpan := cpotel.StartReconcileSpan(ctx, "namespace-gc", "reconcile")
	var tickErr error
	defer func() { endSpan(tickErr) }()

	// An empty instance identity would list every HyperShell-managed namespace
	// on the cluster. Abort rather than treat another instance's gateways as
	// orphans of this one.
	if r.cpNamespace == "" {
		tickErr = fmt.Errorf("no control-plane namespace configured; refusing to sweep")
		log.Printf("WARN namespace gc: %v", tickErr)
		return
	}

	// Build the set of namespaces backed by a live Gateway. If we cannot list
	// gateways we must abort the whole sweep: an empty or failed list would make
	// every managed namespace look orphaned and risk reaping live ones.
	live, err := r.liveNamespaces(ctx)
	if err != nil {
		tickErr = err
		log.Printf("WARN namespace gc: build live gateway set: %v", err)
		return
	}

	selector, err := gateway.ManagedNamespaceSelector(r.cpNamespace)
	if err != nil {
		tickErr = err
		log.Printf("WARN namespace gc: %v", tickErr)
		return
	}
	listCtx, cancel := context.WithTimeout(ctx, namespaceListTimeout)
	namespaces, err := r.client.CoreV1().Namespaces().List(listCtx, metav1.ListOptions{
		LabelSelector: selector,
	})
	cancel()
	if err != nil {
		tickErr = err
		log.Printf("WARN namespace gc: list managed namespaces: %v", err)
		return
	}

	for i := range namespaces.Items {
		ns := &namespaces.Items[i]
		if err := r.reconcileNamespace(ctx, ns, live); err != nil {
			tickErr = errors.Join(tickErr, err)
			log.Printf("WARN namespace gc: %s: %v", ns.Name, err)
		}
	}
}

// grpcLiveNamespaces returns the set of namespaces backed by a live Gateway,
// paging the entire fleet behind a bounded timeout. It is the default
// liveNamespaces seam and is called both to seed a sweep and to re-confirm
// liveness immediately before a delete.
//
// The list is paginated, so it pages through the entire fleet; a truncated
// (first-page) view would omit later gateways from the live set and orphan
// their live namespaces. This is a destructive path, so the live set must key
// on the real namespace, never a synthesized guess: a live gateway with no
// namespace means we cannot know which namespace backs it, so it returns an
// error rather than a partial set (do not fall back to gatewayNamespace here).
// The caller aborts the sweep, or defers the delete, on that error rather than
// risk reaping a namespace that is actually in use.
func (r *NamespaceGCReconciler) grpcLiveNamespaces(ctx context.Context) (map[string]struct{}, error) {
	listCtx, cancel := context.WithTimeout(ctx, gatewayListTimeout)
	defer cancel()
	client := pb.NewGatewayServiceClient(r.grpcConn)
	gateways, err := listAllGateways(listCtx, client)
	if err != nil {
		return nil, fmt.Errorf("list gateways: %w", err)
	}
	live := make(map[string]struct{}, len(gateways))
	for _, gw := range gateways {
		ns := gw.GetNamespace()
		if ns == "" {
			return nil, fmt.Errorf("gateway %s has no namespace; refusing to build a live set that could orphan a live namespace", gw.GetMetadata().GetId())
		}
		live[ns] = struct{}{}
	}
	return live, nil
}

// reconcileNamespace evaluates a single managed namespace against the set of
// namespaces backed by a live Gateway and reaps it if it has been orphaned past
// the grace period. It is best-effort and idempotent.
func (r *NamespaceGCReconciler) reconcileNamespace(ctx context.Context, ns *corev1.Namespace, live map[string]struct{}) error {
	// Defense in depth: only gateway workload namespaces are subject to this
	// reconciler, even if the server-side label selector over-returns.
	if !gateway.IsGatewayNamespaceForGC(ns, r.cpNamespace) {
		return nil
	}
	// A namespace already terminating needs no further action.
	if ns.DeletionTimestamp != nil {
		return nil
	}

	// Backed by a live Gateway: not orphaned. Reset any grace timer left from a
	// prior orphaned observation (e.g. the Gateway was recreated).
	if _, ok := live[ns.Name]; ok {
		clearCtx, cancel := context.WithTimeout(ctx, namespaceOperationTimeout)
		defer cancel()
		return gateway.ClearGCEligible(clearCtx, r.client, ns)
	}

	// Orphaned. Start (or read) the grace timer, persisted on the namespace.
	markCtx, cancel := context.WithTimeout(ctx, namespaceOperationTimeout)
	defer cancel()
	eligibleSince, err := gateway.MarkGCEligible(markCtx, r.client, ns, r.now())
	if err != nil {
		return err
	}

	elapsed := r.now().UTC().Sub(eligibleSince)
	if elapsed < r.gracePeriod {
		log.Printf("INFO namespace gc: %s orphaned for %s, within grace period %s; deferring",
			ns.Name, elapsed.Round(time.Second), r.gracePeriod)
		return nil
	}

	// Grace elapsed, but the `live` set was captured at the start of the sweep and
	// can be minutes stale by the time this namespace is reached. Re-confirm
	// liveness now, immediately before the destructive delete, so a Gateway
	// created for this namespace since the sweep began is not reaped.
	freshLive, err := r.liveNamespaces(ctx)
	if err != nil {
		// Cannot confirm liveness: defer the reap to a later sweep rather than
		// delete on a possibly stale view.
		return fmt.Errorf("re-check liveness before reaping %s: %w", ns.Name, err)
	}
	if _, ok := freshLive[ns.Name]; ok {
		log.Printf("INFO namespace gc: %s became live again before reap; clearing grace timer", ns.Name)
		clearCtx, cancel := context.WithTimeout(ctx, namespaceOperationTimeout)
		defer cancel()
		return gateway.ClearGCEligible(clearCtx, r.client, ns)
	}

	// Confirmed still orphaned past grace: summarize workloads for debuggability,
	// record an event, and reap. Summaries are best-effort; failure to gather them
	// must not block the reap of an already-orphaned namespace.
	summaryCtx, cancel := context.WithTimeout(ctx, namespaceOperationTimeout)
	summary, sErr := gateway.SummarizeNamespace(summaryCtx, r.client, ns.Name)
	cancel()
	if sErr != nil {
		log.Printf("WARN namespace gc: %s: summarize pods: %v", ns.Name, sErr)
	}
	countCtx, cancel := context.WithTimeout(ctx, namespaceOperationTimeout)
	activeSandboxes, aErr := gateway.CountActiveSandboxes(countCtx, r.client, ns.Name)
	cancel()
	if aErr != nil {
		log.Printf("WARN namespace gc: %s: count sandboxes: %v", ns.Name, aErr)
	}

	reason := fmt.Sprintf("orphaned for %s (no live gateway); workloads: %s; active sandboxes: %d",
		elapsed.Round(time.Second), summary.String(), activeSandboxes)
	log.Printf("INFO namespace gc: reaping namespace %s: %s", ns.Name, reason)
	eventCtx, cancel := context.WithTimeout(ctx, namespaceOperationTimeout)
	if err := r.recordGCEvent(eventCtx, ns.Name, reason); err != nil {
		cancel()
		return fmt.Errorf("record GC event for %s: %w", ns.Name, err)
	}
	cancel()

	deleteCtx, cancel := context.WithTimeout(ctx, namespaceOperationTimeout)
	defer cancel()
	if _, err := gateway.DeleteManagedNamespace(deleteCtx, r.client, ns.Name, r.cpNamespace); err != nil {
		return err
	}
	return nil
}

// recordGCEvent records a Kubernetes Event describing a garbage-collection
// action. The Event is created in the control-plane namespace so it outlives the
// namespace being deleted, giving operators a durable record.
func (r *NamespaceGCReconciler) recordGCEvent(ctx context.Context, namespace, message string) error {
	if r.cpNamespace == "" {
		// No control-plane namespace means no durable record is possible. A reap
		// without an audit Event is not allowed, so fail closed.
		return fmt.Errorf("no control-plane namespace configured; cannot record the required GC Event for %s", namespace)
	}
	now := metav1.NewTime(r.now())
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "namespace-gc-",
			Namespace:    r.cpNamespace,
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "Namespace",
			Name:       namespace,
			// The Event lives in the control-plane namespace so it outlives the
			// reaped namespace. Kubernetes requires involvedObject.namespace to
			// match event.namespace for namespaced Events; Name still identifies
			// the gateway namespace that was garbage collected.
			Namespace: r.cpNamespace,
		},
		Reason:         "GarbageCollected",
		Message:        message,
		Type:           corev1.EventTypeWarning,
		Source:         corev1.EventSource{Component: "hypershell-control-plane"},
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
	}
	if _, err := r.client.CoreV1().Events(r.cpNamespace).Create(ctx, event, metav1.CreateOptions{}); err != nil {
		return err
	}
	return nil
}
