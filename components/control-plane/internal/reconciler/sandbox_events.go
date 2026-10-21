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
	listEvents     func(context.Context, []string) ([]*corev1.Event, error)
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
	events, err := r.listEvents(listCtx, sortedNamespaces(live))
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

// kubernetesEvents lists Kubernetes Events per live gateway namespace, never
// cluster-wide. A shared cluster hosts other tenants' environments whose event
// volume is unbounded from this controller's perspective; a cluster-wide list
// has already OOM-killed the memory-limited controller in the PR e2e
// environment (HYPERSHELL-422). Namespace-scoped lists bound the decode cost
// to the namespaces this cluster actually reconciles.
func (r *SandboxEventsReconciler) kubernetesEvents(ctx context.Context, namespaces []string) ([]*corev1.Event, error) {
	var out []*corev1.Event
	for _, namespace := range namespaces {
		list, err := r.client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list Kubernetes Events in %s: %w", namespace, err)
		}
		for i := range list.Items {
			out = append(out, &list.Items[i])
		}
	}
	return out, nil
}

// sortedNamespaces returns the live namespace set as a stable sorted slice so
// list calls and logs are deterministic across ticks.
func sortedNamespaces(live map[string]struct{}) []string {
	out := make([]string, 0, len(live))
	for namespace := range live {
		out = append(out, namespace)
	}
	sort.Strings(out)
	return out
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
	sourceEvents       []*corev1.Event
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
				group.sourceEvents = append(group.sourceEvents, event)
			}
		}
	}
	out := make([]sandboxEventGroup, 0, len(groups))
	for _, group := range groups {
		// Kubernetes Events retain ordering information that cannot be recovered
		// after conversion to the bounded protobuf representation. Sort the source
		// objects before projecting them so EventTime, series observations, and
		// creation time retain their distinct precedence (OSG-SEVT-01).
		sort.SliceStable(group.sourceEvents, func(i, j int) bool { return sandboxEventNewer(group.sourceEvents[i], group.sourceEvents[j]) })
		if len(group.sourceEvents) > limit {
			group.sourceEvents = group.sourceEvents[:limit]
		}
		group.events = make([]*pb.SandboxEvent, 0, len(group.sourceEvents))
		for _, event := range group.sourceEvents {
			group.events = append(group.events, protoEvent(event))
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
	if first.IsZero() && !event.EventTime.IsZero() {
		first = event.EventTime.Time
	}
	return &pb.SandboxEvent{Type: event.Type, Reason: event.Reason, Message: event.Message, FirstObservedAt: timestampOrNil(first), LastObservedAt: timestampOrNil(sandboxEventLastObservedTime(event)), Count: event.Count, InvolvedPodName: event.InvolvedObject.Name, InvolvedPodUid: string(event.InvolvedObject.UID)}
}
func timestampOrNil(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}
	return timestamppb.New(value.UTC())
}

// sandboxEventLastObservedTime returns the Kubernetes last-observed value.
// EventTime is deliberately not a fallback: it has a distinct meaning and is
// represented by first_observed_at when FirstTimestamp is unavailable.
func sandboxEventLastObservedTime(event *corev1.Event) time.Time {
	if event.Series != nil && !event.Series.LastObservedTime.IsZero() {
		return event.Series.LastObservedTime.Time
	}
	return event.LastTimestamp.Time
}

// sandboxEventNewer implements the OSG-SEVT-01 source-object ordering. Do not sort
// protobuf events here: projection loses EventTime and creation time, and would
// collapse independently meaningful Kubernetes timestamps.
func sandboxEventNewer(left, right *corev1.Event) bool {
	for _, timestamps := range [][2]time.Time{
		{left.EventTime.Time, right.EventTime.Time},
		{sandboxEventLastObservedTime(left), sandboxEventLastObservedTime(right)},
		{left.CreationTimestamp.Time, right.CreationTimestamp.Time},
	} {
		if !timestamps[0].Equal(timestamps[1]) {
			return timestamps[0].After(timestamps[1])
		}
	}
	for _, values := range [][2]string{
		{string(left.InvolvedObject.UID), string(right.InvolvedObject.UID)},
		{left.InvolvedObject.Namespace, right.InvolvedObject.Namespace},
		{left.InvolvedObject.Name, right.InvolvedObject.Name},
		{left.Reason, right.Reason},
		{left.Message, right.Message},
		{left.Type, right.Type},
		{left.Name, right.Name},
		{left.ResourceVersion, right.ResourceVersion},
	} {
		if values[0] != values[1] {
			return values[0] < values[1]
		}
	}
	return false
}
