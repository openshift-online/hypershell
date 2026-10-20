package reconciler

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	cpotel "github.com/openshift-online/hypershell/components/control-plane/internal/otel"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultSandboxEventsInterval = 2 * time.Minute
	sandboxEventsListTimeout     = 30 * time.Second
	sandboxEventsRPCTimeout      = 10 * time.Second
)

// SandboxEventsReconciler periodically reports the complete bounded Kubernetes
// event snapshot for each correlated sandbox in a live gateway namespace.
type SandboxEventsReconciler struct {
	client         kubernetes.Interface
	dynamicClient  dynamic.Interface
	grpcConn       *grpc.ClientConn
	interval       time.Duration
	clusterID      string
	limit          int
	listPods       func(context.Context) ([]*corev1.Pod, error)
	listSandboxes  func(context.Context) ([]*unstructured.Unstructured, error)
	listEvents     func(context.Context) ([]*corev1.Event, error)
	liveNamespaces func(context.Context) (map[string]struct{}, error)
	report         func(context.Context, string, string, []*pb.SandboxEvent) error
}

func NewSandboxEventsReconciler(client kubernetes.Interface, dynamicClient dynamic.Interface, grpcConn *grpc.ClientConn, interval time.Duration, clusterID string, limit int) *SandboxEventsReconciler {
	if interval <= 0 {
		interval = defaultSandboxEventsInterval
	}
	if limit < 5 || limit > 10 {
		limit = 10
	}
	r := &SandboxEventsReconciler{client: client, dynamicClient: dynamicClient, grpcConn: grpcConn, interval: interval, clusterID: clusterID, limit: limit}
	r.listPods, r.listSandboxes, r.listEvents = r.kubernetesPods, r.kubernetesSandboxes, r.kubernetesEvents
	r.liveNamespaces, r.report = r.grpcLiveNamespaces, r.grpcReport
	return r
}

func (r *SandboxEventsReconciler) Run(ctx context.Context) error {
	log.Printf("INFO sandbox events reconciler started (interval=%s)", r.interval)
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

func (r *SandboxEventsReconciler) reconcileOnce(ctx context.Context) {
	ctx, end := cpotel.StartReconcileSpan(ctx, "sandbox-events", "reconcile", "")
	var tickErr error
	defer func() { outcome, reason := cpotel.ClassifyReconcileOutcome(tickErr); end(outcome, reason, tickErr) }()
	listCtx, cancel := context.WithTimeout(ctx, sandboxEventsListTimeout)
	defer cancel()
	live, err := r.liveNamespaces(listCtx)
	if err != nil {
		tickErr = err
		log.Printf("WARN sandbox events: list live gateway namespaces: %v", err)
		return
	}
	pods, err := r.listPods(listCtx)
	if err != nil {
		tickErr = err
		log.Printf("WARN sandbox events: list sandbox pods: %v", err)
		return
	}
	sandboxes, err := r.listSandboxes(listCtx)
	if err != nil {
		tickErr = err
		log.Printf("WARN sandbox events: list Sandbox CRs: %v", err)
		return
	}
	events, err := r.listEvents(listCtx)
	if err != nil {
		tickErr = err
		log.Printf("WARN sandbox events: list Kubernetes Events: %v", err)
		return
	}
	groups := selectSandboxEvents(pods, gateway.IndexSandboxes(sandboxes), events, live, r.limit)
	for _, group := range groups {
		if err := r.report(ctx, group.namespace, group.sandbox, group.events); err != nil {
			tickErr = err
			log.Printf("WARN sandbox events: report %s/%s: %v", group.namespace, group.sandbox, err)
		}
	}
}

func (r *SandboxEventsReconciler) kubernetesPods(ctx context.Context) ([]*corev1.Pod, error) {
	list, err := r.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{LabelSelector: gateway.SandboxPodSelector})
	if err != nil {
		return nil, fmt.Errorf("list sandbox pods: %w", err)
	}
	out := make([]*corev1.Pod, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, &list.Items[i])
	}
	return out, nil
}
func (r *SandboxEventsReconciler) kubernetesSandboxes(ctx context.Context) ([]*unstructured.Unstructured, error) {
	list, err := r.dynamicClient.Resource(gateway.SandboxGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list Sandbox CRs: %w", err)
	}
	out := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, &list.Items[i])
	}
	return out, nil
}
func (r *SandboxEventsReconciler) kubernetesEvents(ctx context.Context) ([]*corev1.Event, error) {
	list, err := r.client.CoreV1().Events("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list Kubernetes Events: %w", err)
	}
	out := make([]*corev1.Event, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, &list.Items[i])
	}
	return out, nil
}
func (r *SandboxEventsReconciler) grpcLiveNamespaces(ctx context.Context) (map[string]struct{}, error) {
	callCtx, cancel := context.WithTimeout(ctx, sandboxEventsRPCTimeout)
	defer cancel()
	gateways, err := listAllGateways(callCtx, pb.NewGatewayServiceClient(r.grpcConn), r.clusterID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(gateways))
	for _, gw := range gateways {
		if namespace := gw.GetNamespace(); namespace != "" {
			out[namespace] = struct{}{}
		}
	}
	return out, nil
}
func (r *SandboxEventsReconciler) grpcReport(ctx context.Context, namespace, sandbox string, events []*pb.SandboxEvent) error {
	callCtx, cancel := context.WithTimeout(ctx, sandboxEventsRPCTimeout)
	defer cancel()
	_, err := pb.NewGatewayServiceClient(r.grpcConn).ReportSandboxEvents(callCtx, &pb.ReportSandboxEventsRequest{Namespace: namespace, SandboxName: sandbox, Events: events})
	return err
}

type sandboxEventGroup struct {
	namespace, sandbox string
	events             []*pb.SandboxEvent
}

func selectSandboxEvents(pods []*corev1.Pod, sandboxes gateway.SandboxLookup, events []*corev1.Event, live map[string]struct{}, limit int) []sandboxEventGroup {
	if limit < 5 || limit > 10 {
		limit = 10
	}
	groups := map[string]*sandboxEventGroup{}
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		if _, ok := live[pod.Namespace]; !ok {
			continue
		}
		sandbox := gateway.FindSandboxForPod(pod, sandboxes)
		if sandbox == nil || sandbox.GetName() == "" {
			continue
		}
		key := pod.Namespace + "\x00" + sandbox.GetName()
		group := groups[key]
		if group == nil {
			group = &sandboxEventGroup{namespace: pod.Namespace, sandbox: sandbox.GetName()}
			groups[key] = group
		}
		for _, event := range events {
			if eventMatchesPod(event, pod) {
				group.events = append(group.events, protoEvent(event))
			}
		}
	}
	out := make([]sandboxEventGroup, 0, len(groups))
	for _, group := range groups {
		sort.SliceStable(group.events, func(i, j int) bool { return sandboxEventNewer(group.events[i], group.events[j]) })
		if len(group.events) > limit {
			group.events = group.events[:limit]
		}
		out = append(out, *group)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].namespace != out[j].namespace {
			return out[i].namespace < out[j].namespace
		}
		return out[i].sandbox < out[j].sandbox
	})
	return out
}
func eventMatchesPod(event *corev1.Event, pod *corev1.Pod) bool {
	if event == nil {
		return false
	}
	involved := event.InvolvedObject
	if involved.Namespace != pod.Namespace {
		return false
	}
	if involved.UID != "" {
		return involved.UID == pod.UID
	}
	return involved.Name == pod.Name
}
func protoEvent(event *corev1.Event) *pb.SandboxEvent {
	first := event.FirstTimestamp.Time
	last := sandboxEventTimestamp(event)
	return &pb.SandboxEvent{Type: event.Type, Reason: event.Reason, Message: event.Message, FirstObservedAt: timestampOrNil(first), LastObservedAt: timestampOrNil(last), Count: event.Count, InvolvedPodName: event.InvolvedObject.Name, InvolvedPodUid: string(event.InvolvedObject.UID)}
}
func timestampOrNil(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}
	return timestamppb.New(value.UTC())
}
func sandboxEventTimestamp(event *corev1.Event) time.Time {
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	if event.Series != nil && !event.Series.LastObservedTime.IsZero() {
		return event.Series.LastObservedTime.Time
	}
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	return event.CreationTimestamp.Time
}
func sandboxEventNewer(left, right *pb.SandboxEvent) bool {
	leftTime, rightTime := protoEventTime(left), protoEventTime(right)
	if !leftTime.Equal(rightTime) {
		return leftTime.After(rightTime)
	}
	if left.GetInvolvedPodUid() != right.GetInvolvedPodUid() {
		return left.GetInvolvedPodUid() < right.GetInvolvedPodUid()
	}
	if left.GetInvolvedPodName() != right.GetInvolvedPodName() {
		return left.GetInvolvedPodName() < right.GetInvolvedPodName()
	}
	if left.GetReason() != right.GetReason() {
		return left.GetReason() < right.GetReason()
	}
	return left.GetMessage() < right.GetMessage()
}
func protoEventTime(event *pb.SandboxEvent) time.Time {
	if event.GetLastObservedAt() != nil {
		return event.GetLastObservedAt().AsTime()
	}
	if event.GetFirstObservedAt() != nil {
		return event.GetFirstObservedAt().AsTime()
	}
	return time.Time{}
}
