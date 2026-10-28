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
	oidcInspector oidcClientInspector

	publish                        func(ctx context.Context, counts gateway.AttentionCounts)
	liveNamespaces                 func(ctx context.Context) (map[string]struct{}, error)
	listPods                       func(ctx context.Context) ([]*corev1.Pod, error)
	listSandboxes                  func(ctx context.Context) ([]*unstructured.Unstructured, error)
	listNamespaces                 func(ctx context.Context) ([]*corev1.Namespace, error)
	activeSandboxes                map[string]gateway.SandboxObservation
	publishUsage                   func(ctx context.Context, counts map[string]int)
	recordLifetime                 func(ctx context.Context, namespace string, lifetime time.Duration)
	publishReadiness               func(ctx context.Context, ready int)
	publishReadinessUnavailable    func(ctx context.Context)
	publishCertificates            func(ctx context.Context, health certificateHealth)
	publishCertificatesUnavailable func(ctx context.Context)
	publishOIDC                    func(ctx context.Context, synced, expected int, connected bool)
	listCertificates               func(ctx context.Context, eligible map[string]struct{}) ([][]byte, error)
	listGateways                   func(ctx context.Context) ([]*pb.Gateway, error)
}

// NewSandboxAttentionReconciler builds a SandboxAttentionReconciler.
func NewSandboxAttentionReconciler(
	client kubernetes.Interface,
	dynamicClient dynamic.Interface,
	grpcConn *grpc.ClientConn,
	interval time.Duration,
	clusterID, instance string,
	oidcInspector oidcClientInspector,
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
		oidcInspector: oidcInspector,
	}
	r.publish = r.exportCounts
	r.liveNamespaces = r.grpcLiveNamespaces
	r.listPods = r.listSandboxPods
	r.listSandboxes = r.listSandboxCRs
	r.listNamespaces = r.listManagedNamespaces
	r.publishUsage = r.exportUsage
	r.recordLifetime = r.exportLifetime
	r.publishReadiness = r.exportReadiness
	r.publishReadinessUnavailable = r.exportReadinessUnavailable
	r.publishCertificates = r.exportCertificates
	r.publishCertificatesUnavailable = r.exportCertificatesUnavailable
	r.publishOIDC = r.exportOIDC
	r.listCertificates = r.listCertificateMaterials
	r.listGateways = r.grpcGateways
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
	ctx, endSpan := cpotel.StartReconcileSpan(ctx, "Gateway", "reconcile", "")
	var tickErr error
	defer func() {
		outcome, reason := cpotel.ClassifyReconcileOutcome(tickErr)
		endSpan(outcome, reason, tickErr)
	}()

	listCtx, cancel := context.WithTimeout(ctx, sandboxAttentionListTimeout)
	defer cancel()

	namespaces, err := r.listNamespaces(listCtx)
	if err != nil {
		tickErr = err
		log.Printf("WARN sandbox attention: list managed namespaces: %v", err)
		r.publishDashboardHealthUnavailable(ctx)
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
		r.publishDashboardHealthUnavailable(ctx)
		r.publishZero(ctx)
		return
	}

	// Sandbox CR list failure must not block orphaned publication (OBS-SBXA-01 /
	// SC-003): orphaned counts need only pods + live gateway namespaces.
	// OBS-SBXA-05: on CR failure, treat expiring-soon and idle as zero for that tick
	// (do not apply the never-used idle heuristic from pod CreationTime alone).
	sandboxes, err := r.listSandboxes(listCtx)
	crListFailed := err != nil
	if crListFailed {
		log.Printf("WARN sandbox attention: list sandbox CRs (degrading to pod-only orphaned counts): %v", err)
		sandboxes = nil
	}

	observations := gateway.BuildAttentionObservations(pods, gateway.IndexSandboxes(sandboxes), eligible)
	r.recordUsage(ctx, eligible, sandboxes, observations, crListFailed)
	if !crListFailed && r.publishReadiness != nil {
		r.publishReadiness(ctx, readySandboxCount(pods, sandboxes, eligible))
	} else if crListFailed && r.publishReadinessUnavailable != nil {
		r.publishReadinessUnavailable(ctx)
	}
	if r.listCertificates != nil && r.publishCertificates != nil {
		materials, certErr := r.listCertificates(listCtx, eligible)
		if certErr != nil {
			log.Printf("WARN dashboard health: evaluate certificates: %v", certErr)
			if r.publishCertificatesUnavailable != nil {
				r.publishCertificatesUnavailable(ctx)
			}
		} else {
			r.publishCertificates(ctx, evaluateCertificateHealth(materials, r.now().UTC()))
		}
	}
	r.observeOIDCHealth(listCtx)

	live, err := r.liveNamespaces(listCtx)
	if err != nil {
		tickErr = err
		log.Printf("WARN sandbox attention: list live gateway namespaces: %v", err)
		r.publishZero(ctx)
		return
	}

	counts := gateway.ClassifyAttentionCounts(r.now().UTC(), live, observations)
	if crListFailed {
		counts.Expiring = 0
		counts.Idle = 0
	}

	r.publish(ctx, counts)
	log.Printf("INFO sandbox attention: exported %s (eligible_ns=%d observations=%d)",
		gateway.FormatAttentionCounts(counts), len(eligible), len(observations))
}

func (r *SandboxAttentionReconciler) recordUsage(
	ctx context.Context,
	eligible map[string]struct{},
	sandboxes []*unstructured.Unstructured,
	observations []gateway.SandboxObservation,
	crListFailed bool,
) {
	if !crListFailed && r.publishUsage != nil {
		r.publishUsage(ctx, gateway.ProvisionedSandboxCounts(sandboxes, eligible))
	}

	current, departed := gateway.ReconcileActiveSandboxes(r.activeSandboxes, observations)
	r.activeSandboxes = current
	if r.recordLifetime == nil {
		return
	}
	now := r.now().UTC()
	for _, obs := range departed {
		lifetime, ok := gateway.SandboxLifetime(obs, now)
		if !ok {
			continue
		}
		r.recordLifetime(ctx, obs.Namespace, lifetime)
	}
}

func (r *SandboxAttentionReconciler) exportUsage(ctx context.Context, counts map[string]int) {
	for namespace, count := range counts {
		cpotel.RecordSandboxTotal(ctx, r.clusterID, namespace, count)
	}
}

func (r *SandboxAttentionReconciler) exportLifetime(ctx context.Context, namespace string, lifetime time.Duration) {
	cpotel.RecordSandboxLifetime(ctx, r.clusterID, namespace, lifetime)
}

func (r *SandboxAttentionReconciler) exportReadiness(ctx context.Context, ready int) {
	cpotel.RecordSandboxReady(ctx, r.clusterID, ready)
}

func (r *SandboxAttentionReconciler) exportReadinessUnavailable(ctx context.Context) {
	cpotel.RecordSandboxReadyUnavailable(ctx, r.clusterID)
}

func (r *SandboxAttentionReconciler) publishDashboardHealthUnavailable(ctx context.Context) {
	if r.publishReadinessUnavailable != nil {
		r.publishReadinessUnavailable(ctx)
	}
	if r.publishCertificatesUnavailable != nil {
		r.publishCertificatesUnavailable(ctx)
	}
}

func (r *SandboxAttentionReconciler) exportCertificates(ctx context.Context, health certificateHealth) {
	cpotel.RecordCertificateHealth(ctx, r.clusterID, health.Valid, health.Expired, health.Unavailable, health.DaysToExpiryMin)
}

func (r *SandboxAttentionReconciler) exportCertificatesUnavailable(ctx context.Context) {
	cpotel.RecordCertificateHealthUnavailable(ctx, r.clusterID)
}

func (r *SandboxAttentionReconciler) exportOIDC(ctx context.Context, synced, expected int, connected bool) {
	cpotel.RecordOIDCClientHealth(ctx, r.clusterID, synced, expected, connected)
}

func (r *SandboxAttentionReconciler) observeOIDCHealth(ctx context.Context) {
	if r.publishOIDC == nil || r.listGateways == nil {
		return
	}
	gateways, err := r.listGateways(ctx)
	if err != nil {
		log.Printf("WARN dashboard health: list desired OIDC clients: %v", err)
		r.publishOIDC(ctx, 0, 0, false)
		return
	}
	clientIDs := desiredOIDCClientIDs(gateways)
	if r.oidcInspector == nil {
		r.publishOIDC(ctx, 0, len(clientIDs), false)
		return
	}
	synced := 0
	for _, clientID := range clientIDs {
		inSync, inspectErr := r.oidcInspector.GatewayClientSynced(ctx, clientID)
		if inspectErr != nil {
			log.Printf("WARN dashboard health: compare desired OIDC clients: %v", inspectErr)
			r.publishOIDC(ctx, synced, len(clientIDs), false)
			return
		}
		if inSync {
			synced++
		}
	}
	r.publishOIDC(ctx, synced, len(clientIDs), true)
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

func (r *SandboxAttentionReconciler) grpcGateways(ctx context.Context) ([]*pb.Gateway, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, sandboxAttentionRPCTimeout)
	defer cancel()
	return listAllGateways(rpcCtx, pb.NewGatewayServiceClient(r.grpcConn), r.clusterID)
}

func (r *SandboxAttentionReconciler) exportCounts(ctx context.Context, counts gateway.AttentionCounts) {
	cpotel.RecordSandboxAttentionCounts(ctx, r.clusterID, counts.Orphaned, counts.Expiring, counts.Idle)
}

// publishZero clears attention gauges so a failed tick cannot leave stale
// non-zero values exporting until the process stops (OBS-SBXA-06 freshness).
func (r *SandboxAttentionReconciler) publishZero(ctx context.Context) {
	r.publish(ctx, gateway.AttentionCounts{})
}
