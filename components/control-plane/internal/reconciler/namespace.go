package reconciler

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
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
)

// NamespaceGCReconciler periodically garbage-collects gateway namespaces that
// the control plane created but that no longer have a live Gateway backing them.
// This reaps namespaces orphaned by a delete event missed while the control
// plane was down, and namespaces whose gateway failed to bootstrap and was then
// deleted. Reaping is best-effort and idempotent, and is delayed by a grace
// period recorded on the namespace itself so it survives restarts.
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
	return &NamespaceGCReconciler{
		client:      client,
		grpcConn:    grpcConn,
		interval:    interval,
		gracePeriod: gracePeriod,
		cpNamespace: cpNamespace,
		now:         time.Now,
	}
}

// Run drives the garbage-collection loop until the context is cancelled.
func (r *NamespaceGCReconciler) Run(ctx context.Context) error {
	log.Printf("INFO namespace GC reconciler started (interval=%s grace=%s)", r.interval, r.gracePeriod)
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
	// Build the set of namespaces backed by a live Gateway. If we cannot list
	// gateways we must abort the whole sweep: an empty or failed list would make
	// every managed namespace look orphaned and risk reaping live ones. The list
	// is paginated, so page through the entire fleet; a truncated (first-page)
	// view would omit later gateways from the live set and orphan their live
	// namespaces.
	client := pb.NewGatewayServiceClient(r.grpcConn)
	gateways, err := listAllGateways(ctx, client)
	if err != nil {
		log.Printf("WARN namespace gc: list gateways: %v", err)
		return
	}
	live := make(map[string]struct{}, len(gateways))
	for _, gw := range gateways {
		// This is a destructive path: the live set must key on the real
		// namespace, never a synthesized guess. A live gateway with no namespace
		// means we cannot know which namespace backs it, so building the set
		// without it would risk reaping a namespace that is actually in use.
		// Abort the whole sweep rather than guess (do not fall back to
		// gatewayNamespace here).
		ns := gw.GetNamespace()
		if ns == "" {
			log.Printf("WARN namespace gc: gateway %s has no namespace; aborting sweep to avoid reaping a live namespace", gw.GetMetadata().GetId())
			return
		}
		live[ns] = struct{}{}
	}

	namespaces, err := r.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: gateway.ManagedNamespaceSelector,
	})
	if err != nil {
		log.Printf("WARN namespace gc: list managed namespaces: %v", err)
		return
	}

	for i := range namespaces.Items {
		ns := &namespaces.Items[i]
		if err := r.reconcileNamespace(ctx, ns, live); err != nil {
			log.Printf("WARN namespace gc: %s: %v", ns.Name, err)
		}
	}
}

// reconcileNamespace evaluates a single managed namespace against the set of
// namespaces backed by a live Gateway and reaps it if it has been orphaned past
// the grace period. It is best-effort and idempotent.
func (r *NamespaceGCReconciler) reconcileNamespace(ctx context.Context, ns *corev1.Namespace, live map[string]struct{}) error {
	// Defense in depth: only ever act on namespaces this control plane manages,
	// even if the server-side label selector over-returns.
	if !gateway.IsManagedNamespace(ns) {
		return nil
	}
	// A namespace already terminating needs no further action.
	if ns.DeletionTimestamp != nil {
		return nil
	}

	// Backed by a live Gateway: not orphaned. Reset any grace timer left from a
	// prior orphaned observation (e.g. the Gateway was recreated).
	if _, ok := live[ns.Name]; ok {
		return gateway.ClearGCEligible(ctx, r.client, ns)
	}

	// Orphaned. Start (or read) the grace timer, persisted on the namespace.
	eligibleSince, err := gateway.MarkGCEligible(ctx, r.client, ns, r.now())
	if err != nil {
		return err
	}

	elapsed := r.now().UTC().Sub(eligibleSince)
	if elapsed < r.gracePeriod {
		log.Printf("INFO namespace gc: %s orphaned for %s, within grace period %s; deferring",
			ns.Name, elapsed.Round(time.Second), r.gracePeriod)
		return nil
	}

	// Grace elapsed: summarize workloads for debuggability, record an event, and
	// reap. Summaries are best-effort; failure to gather them must not block the
	// reap of an already-orphaned namespace.
	summary, sErr := gateway.SummarizeNamespace(ctx, r.client, ns.Name)
	if sErr != nil {
		log.Printf("WARN namespace gc: %s: summarize pods: %v", ns.Name, sErr)
	}
	activeSandboxes, aErr := gateway.CountActiveSandboxes(ctx, r.client, ns.Name)
	if aErr != nil {
		log.Printf("WARN namespace gc: %s: count sandboxes: %v", ns.Name, aErr)
	}

	reason := fmt.Sprintf("orphaned for %s (no live gateway); workloads: %s; active sandboxes: %d",
		elapsed.Round(time.Second), summary.String(), activeSandboxes)
	log.Printf("INFO namespace gc: reaping namespace %s: %s", ns.Name, reason)
	r.recordGCEvent(ctx, ns.Name, reason)

	if _, err := gateway.DeleteManagedNamespace(ctx, r.client, ns.Name); err != nil {
		return err
	}
	return nil
}

// recordGCEvent records a Kubernetes Event describing a garbage-collection
// action. The Event is created in the control-plane namespace so it outlives the
// namespace being deleted, giving operators a durable record. Best-effort:
// event creation failures are logged, never fatal.
func (r *NamespaceGCReconciler) recordGCEvent(ctx context.Context, namespace, message string) {
	if r.cpNamespace == "" {
		return
	}
	now := metav1.NewTime(r.now())
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "namespace-gc-",
			Namespace:    r.cpNamespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: "Namespace",
			Name: namespace,
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
		log.Printf("WARN namespace gc: record event for %s: %v", namespace, err)
	}
}
