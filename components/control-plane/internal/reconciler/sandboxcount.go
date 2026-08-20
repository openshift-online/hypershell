package reconciler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// defaultSandboxCountResyncInterval is the cadence at which the sandbox-count
// reconciler both re-lists sandbox pods into its cache (informer resync) and
// self-heals every gateway's stored count to the absolute number observed in
// that cache. It corrects drift from any missed event and, after a restart,
// recovers the accurate count without waiting for a sandbox create or delete.
const defaultSandboxCountResyncInterval = 2 * time.Minute

// sandboxCountRPCTimeout bounds each adjust or set RPC the reconciler issues,
// whether from an informer event handler or a self-heal pass, so a wedged API
// server cannot stall pod-event delivery or the self-heal loop.
const sandboxCountRPCTimeout = 10 * time.Second

// sandboxCountGatewayListTimeout bounds each paginated gateway namespace read
// so a stalled API server cannot block the entire self-heal pass forever.
const sandboxCountGatewayListTimeout = 30 * time.Second

// SandboxCountReconciler maintains each Gateway's active_sandbox_count from a
// single label-selected informer over agent sandbox pods, rather than a periodic
// full-namespace pod LIST. It increments the owning gateway's count as sandbox
// pods enter the active set (Running/Pending) and decrements as they leave it or
// are deleted, applying every change atomically at the API server. A periodic
// self-heal reconciles each gateway's stored count to the absolute number of
// active sandbox pods held in the informer's in-memory cache, correcting drift
// and recovering the count after a restart.
//
// See openshell-gateway-sandbox-count.spec.md (HYPERSHELL-78).
type SandboxCountReconciler struct {
	client         kubernetes.Interface
	grpcConn       *grpc.ClientConn
	resyncInterval time.Duration

	// adjust, set, and namespaces are the seams to the API server, overridable in
	// tests so the reconciliation logic can be exercised without a live gRPC
	// server. adjust applies a relative delta; set writes an absolute count; and
	// namespaces enumerates the gateway namespaces the self-heal must reconcile
	// (including those whose sandbox count should converge back to zero).
	adjust     func(ctx context.Context, namespace string, delta int) error
	set        func(ctx context.Context, namespace string, count int) error
	namespaces func(ctx context.Context) ([]string, error)

	// synced gates the incremental Add handler. The informer delivers its initial
	// LIST as a burst of Add events; counting those as +1 deltas would stack them
	// on top of the already-persisted count. Deltas are enabled only once the
	// cache has synced, and the baseline is (re-)established by the self-heal.
	synced atomic.Bool

	// baseCtx is the reconciler's run context, captured so event handlers (which
	// receive no context of their own) issue RPCs that are cancelled on shutdown.
	baseCtx context.Context

	// nsLocks serializes the reconciler's own count mutations per gateway
	// namespace, so an event-driven delta and a periodic self-heal SET for the
	// same gateway never issue overlapping RPCs and a self-heal reads its cache
	// snapshot and writes it back without a delta interleaving in between. This
	// orders the control plane's writes per gateway; it does not claim global
	// real-time consistency, and any residual drift from a delta that races the
	// snapshot is corrected by the next self-heal pass (the count is advisory,
	// per openshell-gateway-sandbox-count.spec.md). The map is keyed by namespace
	// and grows only with the gateway namespaces observed, so it is bounded by
	// fleet size. nsMu guards the map itself, not the per-namespace locks.
	nsMu    sync.Mutex
	nsLocks map[string]*sync.Mutex
}

// NewSandboxCountReconciler builds a SandboxCountReconciler, applying the default
// resync interval for any non-positive value.
func NewSandboxCountReconciler(client kubernetes.Interface, grpcConn *grpc.ClientConn, resyncInterval time.Duration) *SandboxCountReconciler {
	if resyncInterval <= 0 {
		resyncInterval = defaultSandboxCountResyncInterval
	}
	r := &SandboxCountReconciler{
		client:         client,
		grpcConn:       grpcConn,
		resyncInterval: resyncInterval,
		baseCtx:        context.Background(),
		nsLocks:        make(map[string]*sync.Mutex),
	}
	r.adjust = r.grpcAdjust
	r.set = r.grpcSet
	r.namespaces = r.grpcGatewayNamespaces
	return r
}

// Run drives the sandbox-count reconciliation loop until the context is
// cancelled. It starts the sandbox pod informer, establishes the baseline count
// from the synced cache, and then self-heals on every resync tick while the
// informer's event handlers keep the count current between ticks.
func (r *SandboxCountReconciler) Run(ctx context.Context) error {
	log.Printf("INFO sandbox count reconciler started (resync=%s)", r.resyncInterval)
	r.baseCtx = ctx

	factory := informers.NewSharedInformerFactoryWithOptions(
		r.client,
		r.resyncInterval,
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.LabelSelector = gateway.SandboxPodSelector
		}),
	)
	podInformer := factory.Core().V1().Pods()
	informer := podInformer.Informer()

	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.onAdd,
		UpdateFunc: r.onUpdate,
		DeleteFunc: r.onDelete,
	}); err != nil {
		return fmt.Errorf("adding sandbox pod event handler: %w", err)
	}

	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		// Cache sync only fails when the context is cancelled during startup.
		return ctx.Err()
	}
	log.Printf("INFO sandbox count reconciler cache synced")

	// Enable incremental deltas only now that the initial LIST has populated the
	// cache, then immediately reconcile to the absolute truth so a restart
	// recovers the correct count without any intervening sandbox event.
	r.synced.Store(true)
	r.selfHeal(ctx, podInformer.Lister())

	ticker := time.NewTicker(r.resyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			r.selfHeal(ctx, podInformer.Lister())
		}
	}
}

// onAdd increments the count when a sandbox pod is observed active. Adds
// delivered during the informer's initial LIST are ignored; the baseline
// self-heal accounts for pre-existing pods.
func (r *SandboxCountReconciler) onAdd(obj interface{}) {
	if !r.synced.Load() {
		return
	}
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	if gateway.IsActiveSandboxPod(pod) {
		r.applyDelta(pod.Namespace, 1)
	}
}

// onUpdate adjusts the count only on an active-set transition: a pod entering the
// active set increments, a pod leaving it decrements. An active-to-active
// transition (e.g. Pending -> Running) and a resync of an unchanged pod are
// no-ops.
func (r *SandboxCountReconciler) onUpdate(oldObj, newObj interface{}) {
	if !r.synced.Load() {
		return
	}
	oldPod, ok := oldObj.(*corev1.Pod)
	if !ok {
		return
	}
	newPod, ok := newObj.(*corev1.Pod)
	if !ok {
		return
	}
	was := gateway.IsActiveSandboxPod(oldPod)
	now := gateway.IsActiveSandboxPod(newPod)
	switch {
	case !was && now:
		r.applyDelta(newPod.Namespace, 1)
	case was && !now:
		r.applyDelta(newPod.Namespace, -1)
	}
}

// onDelete decrements the count when an active sandbox pod is removed. A pod
// whose final state was missed arrives wrapped in a tombstone, which is
// unwrapped so a delete is never dropped.
func (r *SandboxCountReconciler) onDelete(obj interface{}) {
	if !r.synced.Load() {
		return
	}
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		tombstone, tok := obj.(cache.DeletedFinalStateUnknown)
		if !tok {
			return
		}
		pod, ok = tombstone.Obj.(*corev1.Pod)
		if !ok {
			return
		}
	}
	if gateway.IsActiveSandboxPod(pod) {
		r.applyDelta(pod.Namespace, -1)
	}
}

// applyDelta issues a single atomic adjust RPC, bounded by a timeout derived from
// the run context. It holds the per-namespace lock so a delta and a concurrent
// self-heal SET for the same gateway are serialized rather than racing at the API
// server. Adjusting an unknown namespace is a no-op at the API server, so a
// sandbox pod outside any gateway namespace costs only a cheap round trip.
func (r *SandboxCountReconciler) applyDelta(namespace string, delta int) {
	unlock := r.lockNamespace(namespace)
	defer unlock()
	ctx, cancel := context.WithTimeout(r.baseCtx, sandboxCountRPCTimeout)
	defer cancel()
	if err := r.adjust(ctx, namespace, delta); err != nil {
		log.Printf("WARN sandbox count: adjust %s by %+d: %v", namespace, delta, err)
	}
}

// lockNamespace locks and returns an unlock func for the given namespace's
// mutex, creating it on first use. It serializes the reconciler's own count
// mutations (event-driven deltas and self-heal SETs) for one gateway without
// blocking other gateways.
func (r *SandboxCountReconciler) lockNamespace(namespace string) func() {
	r.nsMu.Lock()
	mu := r.nsLocks[namespace]
	if mu == nil {
		mu = &sync.Mutex{}
		r.nsLocks[namespace] = mu
	}
	r.nsMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

// selfHeal reconciles every gateway's stored count to the absolute number of
// active sandbox pods held in the informer cache. It reads the cache (never the
// Kubernetes API) and sets a count for every gateway namespace - including those
// with zero cached sandboxes - so drift is corrected in both directions. Each
// namespace is read from the cache and SET immediately before the next, under the
// per-namespace lock, so the value written reflects the freshest cache snapshot
// for that gateway and no event-driven delta interleaves between the read and the
// write. Each set RPC is bounded by sandboxCountRPCTimeout so a single hung call
// cannot stall the rest of the pass.
func (r *SandboxCountReconciler) selfHeal(ctx context.Context, lister corelisters.PodLister) {
	ctx, endSpan := cpotel.StartReconcileSpan(ctx, "sandbox-count", "reconcile")
	var tickErr error
	defer func() { endSpan(tickErr) }()

	listCtx, cancel := context.WithTimeout(ctx, sandboxCountGatewayListTimeout)
	namespaces, err := r.namespaces(listCtx)
	cancel()
	if err != nil {
		log.Printf("WARN sandbox count: list gateway namespaces: %v", err)
		return
	}
	for _, ns := range namespaces {
		r.healNamespace(ctx, lister, ns)
	}
}

// healNamespace reconciles a single gateway's stored count to the absolute
// number of active sandbox pods currently in the informer cache for its
// namespace. It reads the cache and issues the SET while holding the namespace
// lock, so an event-driven delta for the same gateway cannot interleave between
// the read and the write, and the control plane never issues two overlapping
// count RPCs for one gateway.
func (r *SandboxCountReconciler) healNamespace(ctx context.Context, lister corelisters.PodLister, namespace string) {
	unlock := r.lockNamespace(namespace)
	defer unlock()

	count, err := countActiveSandboxes(lister, namespace)
	if err != nil {
		log.Printf("WARN sandbox count: read cache for %s: %v", namespace, err)
		return
	}
	rpcCtx, cancel := context.WithTimeout(ctx, sandboxCountRPCTimeout)
	defer cancel()
	if err := r.set(rpcCtx, namespace, count); err != nil {
		log.Printf("WARN sandbox count: set %s to %d: %v", namespace, count, err)
	}
}

// countActiveSandboxes tallies the active sandbox pods a namespace holds in the
// informer cache. The informer is label-selected to sandbox pods, so the cache
// holds only sandbox pods; the accounting reuses activeSandboxesByNamespace so
// the active-pod filter has a single source of truth.
func countActiveSandboxes(lister corelisters.PodLister, namespace string) (int, error) {
	pods, err := lister.Pods(namespace).List(labels.Everything())
	if err != nil {
		return 0, err
	}
	return activeSandboxesByNamespace(pods)[namespace], nil
}

// activeSandboxesByNamespace tallies active sandbox pods per namespace from a
// cache snapshot. It is pure so the self-heal accounting can be unit tested
// without an informer.
func activeSandboxesByNamespace(pods []*corev1.Pod) map[string]int {
	counts := make(map[string]int)
	for _, pod := range pods {
		if gateway.IsActiveSandboxPod(pod) {
			counts[pod.Namespace]++
		}
	}
	return counts
}

func (r *SandboxCountReconciler) grpcAdjust(ctx context.Context, namespace string, delta int) error {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err := client.AdjustActiveSandboxCount(ctx, &pb.AdjustActiveSandboxCountRequest{
		Namespace: namespace,
		Delta:     int32(delta),
	})
	return err
}

func (r *SandboxCountReconciler) grpcSet(ctx context.Context, namespace string, count int) error {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	_, err := client.SetActiveSandboxCount(ctx, &pb.SetActiveSandboxCountRequest{
		Namespace: namespace,
		Count:     int32(count),
	})
	return err
}

// grpcGatewayNamespaces returns the namespace of every gateway in the fleet, so
// the self-heal reconciles a count for each - including gateways whose count
// must converge back to zero and which therefore have no pods in the cache.
func (r *SandboxCountReconciler) grpcGatewayNamespaces(ctx context.Context) ([]string, error) {
	client := pb.NewGatewayServiceClient(r.grpcConn)
	gateways, err := listAllGateways(ctx, client)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(gateways))
	for _, gw := range gateways {
		if ns := gw.GetNamespace(); ns != "" {
			out = append(out, ns)
		}
	}
	return out, nil
}
