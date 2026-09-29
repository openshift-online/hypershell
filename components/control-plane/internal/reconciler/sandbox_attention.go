package reconciler

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultSandboxAttentionInterval = 2 * time.Minute
	sandboxAttentionListTimeout     = 30 * time.Second
	sandboxAttentionRPCTimeout      = 10 * time.Second
)

// SandboxAttentionReconciler periodically derives cluster-local attention counts
// (orphaned, expiring-soon, idle) by observing active sandbox pods in managed
// gateway namespaces together with Sandbox CR lifecycle fields, then exports
// those counts as OTLP gauges for the platform metrics pipeline.
//
// See gateway-sandbox-attention-status.spec.md.
type SandboxAttentionReconciler struct {
	client        kubernetes.Interface
	dynamicClient dynamic.Interface
	grpcConn      *grpc.ClientConn
	interval      time.Duration
	clusterID     string
	instance      string
	now           func() time.Time

	publish        func(ctx context.Context, counts gateway.AttentionCounts)
	liveNamespaces func(ctx context.Context) (map[string]struct{}, error)
	listPods       func(ctx context.Context) ([]*corev1.Pod, error)
	listSandboxes  func(ctx context.Context) ([]*unstructured.Unstructured, error)
	listNamespaces func(ctx context.Context) ([]*corev1.Namespace, error)
}

// NewSandboxAttentionReconciler builds a SandboxAttentionReconciler.
func NewSandboxAttentionReconciler(
	client kubernetes.Interface,
	dynamicClient dynamic.Interface,
	grpcConn *grpc.ClientConn,
	interval time.Duration,
	clusterID, instance string,
) *SandboxAttentionReconciler {
	if interval <= 0 {
		interval = defaultSandboxAttentionInterval
	}
	r := &SandboxAttentionReconciler{
		client:        client,
		dynamicClient: dynamicClient,
		grpcConn:      grpcConn,
		interval:      interval,
		clusterID:     clusterID,
		instance:      instance,
		now:           time.Now,
	}
	r.publish = r.exportCounts
	r.liveNamespaces = r.grpcLiveNamespaces
	r.listPods = r.listSandboxPods
	r.listSandboxes = r.listSandboxCRs
	r.listNamespaces = r.listManagedNamespaces
	return r
}

// Run drives the attention reconciliation loop until the context is cancelled.
func (r *SandboxAttentionReconciler) Run(ctx context.Context) error {
	log.Printf("INFO sandbox attention reconciler started (interval=%s cluster_id=%q instance=%s)",
		r.interval, r.clusterID, r.instance)
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

func (r *SandboxAttentionReconciler) reconcileOnce(ctx context.Context) {
	ctx, endSpan := cpotel.StartReconcileSpan(ctx, "sandbox-attention", "reconcile", "")
	var tickErr error
	defer func() { endSpan(tickErr) }()

	listCtx, cancel := context.WithTimeout(ctx, sandboxAttentionListTimeout)
	defer cancel()

	live, err := r.liveNamespaces(listCtx)
	if err != nil {
		tickErr = err
		log.Printf("WARN sandbox attention: list live gateway namespaces: %v", err)
		r.publishZero(ctx)
		return
	}

	namespaces, err := r.listNamespaces(listCtx)
	if err != nil {
		tickErr = err
		log.Printf("WARN sandbox attention: list managed namespaces: %v", err)
		r.publishZero(ctx)
		return
	}
	eligible := make(map[string]struct{}, len(namespaces))
	for _, ns := range namespaces {
		if gateway.IsGatewayNamespaceForGC(ns, r.instance) {
			eligible[ns.Name] = struct{}{}
		}
	}

	pods, err := r.listPods(listCtx)
	if err != nil {
		tickErr = err
		log.Printf("WARN sandbox attention: list sandbox pods: %v", err)
		r.publishZero(ctx)
		return
	}

	// Sandbox CR list failure must not block orphaned publication (SSA-01 /
	// SC-003): orphaned counts need only pods + live gateway namespaces.
	// SSA-05: on CR failure, treat expiring-soon and idle as zero for that tick
	// (do not apply the never-used idle heuristic from pod CreationTime alone).
	sandboxes, err := r.listSandboxes(listCtx)
	crListFailed := err != nil
	if crListFailed {
		log.Printf("WARN sandbox attention: list sandbox CRs (degrading to pod-only orphaned counts): %v", err)
		sandboxes = nil
	}

	observations := gateway.BuildAttentionObservations(pods, gateway.IndexSandboxes(sandboxes), eligible)
	counts := gateway.ClassifyAttentionCounts(r.now().UTC(), live, observations)
	if crListFailed {
		counts.Expiring = 0
		counts.Idle = 0
	}

	r.publish(ctx, counts)
	log.Printf("INFO sandbox attention: exported %s (eligible_ns=%d observations=%d)",
		gateway.FormatAttentionCounts(counts), len(eligible), len(observations))
}

func (r *SandboxAttentionReconciler) listSandboxPods(ctx context.Context) ([]*corev1.Pod, error) {
	list, err := r.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		LabelSelector: gateway.SandboxPodSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("list sandbox pods: %w", err)
	}
	out := make([]*corev1.Pod, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, &list.Items[i])
	}
	return out, nil
}

func (r *SandboxAttentionReconciler) listSandboxCRs(ctx context.Context) ([]*unstructured.Unstructured, error) {
	list, err := r.dynamicClient.Resource(gateway.SandboxGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list sandboxes: %w", err)
	}
	out := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, &list.Items[i])
	}
	return out, nil
}

func (r *SandboxAttentionReconciler) listManagedNamespaces(ctx context.Context) ([]*corev1.Namespace, error) {
	selector, err := gateway.ManagedNamespaceSelector(r.instance)
	if err != nil {
		return nil, err
	}
	list, err := r.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list managed namespaces: %w", err)
	}
	out := make([]*corev1.Namespace, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, &list.Items[i])
	}
	return out, nil
}

func (r *SandboxAttentionReconciler) grpcLiveNamespaces(ctx context.Context) (map[string]struct{}, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, sandboxAttentionRPCTimeout)
	defer cancel()
	client := pb.NewGatewayServiceClient(r.grpcConn)
	gateways, err := listAllGateways(rpcCtx, client, r.clusterID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(gateways))
	for _, gw := range gateways {
		if ns := gw.GetNamespace(); ns != "" {
			out[ns] = struct{}{}
		}
	}
	return out, nil
}

func (r *SandboxAttentionReconciler) exportCounts(ctx context.Context, counts gateway.AttentionCounts) {
	cpotel.RecordSandboxAttentionCounts(ctx, r.clusterID, counts.Orphaned, counts.Expiring, counts.Idle)
}

// publishZero clears attention gauges so a failed tick cannot leave stale
// non-zero values exporting until the process stops (SSA-06 freshness).
func (r *SandboxAttentionReconciler) publishZero(ctx context.Context) {
	r.publish(ctx, gateway.AttentionCounts{})
}
