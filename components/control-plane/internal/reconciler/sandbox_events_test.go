package reconciler

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSelectSandboxEventsIncludesActiveAndTerminatedPodsAndCapsDefaultLimit(t *testing.T) {
	sandbox := sandbox("live", "writer")
	pods := []*corev1.Pod{
		pod("live", "active", "active-uid", "writer", corev1.PodRunning),
		pod("live", "failed", "failed-uid", "writer", corev1.PodFailed),
	}
	events := make([]*corev1.Event, 0, 12)
	for i := 0; i < 12; i++ {
		owner := pods[i%len(pods)]
		events = append(events, event(owner, "event-"+string(rune('a'+i)), time.Time{}, time.Time{}, time.Time{}, "reason-"+string(rune('a'+i))))
	}

	groups := selectSandboxEvents(pods, gateway.IndexSandboxes([]*unstructured.Unstructured{sandbox}), events, map[string]struct{}{"live": {}}, 0)
	if len(groups) != 1 {
		t.Fatalf("groups = %#v, want one group", groups)
	}
	if len(groups[0].events) != 10 {
		t.Fatalf("event count = %d, want default cap 10", len(groups[0].events))
	}
	seen := map[string]bool{}
	for _, got := range groups[0].events {
		seen[got.GetInvolvedPodName()] = true
	}
	if !seen["active"] || !seen["failed"] {
		t.Fatalf("selected pod events = %#v, want active and terminated pods", groups[0].events)
	}
}

func TestSelectSandboxEventsOrdersSourceTimestampsBeforeProjection(t *testing.T) {
	base := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	firstPod := pod("live", "a-pod", "a-uid", "writer", corev1.PodRunning)
	secondPod := pod("live", "b-pod", "b-uid", "writer", corev1.PodSucceeded)

	// EventTime takes precedence over a later series observation. With equal
	// EventTime, the series observation then creation time order the Events.
	events := []*corev1.Event{
		event(firstPod, "event-time-wins", base.Add(5*time.Minute), base.Add(time.Hour), base, "event-time-wins"),
		event(firstPod, "series-newer", base, base.Add(4*time.Minute), base, "series-newer"),
		event(firstPod, "creation-newer", base, base.Add(3*time.Minute), base.Add(2*time.Minute), "creation-newer"),
		event(firstPod, "creation-older", base, base.Add(3*time.Minute), base, "creation-older"),
		event(secondPod, "tie-b", base, base.Add(3*time.Minute), base, "tie-b"),
		event(firstPod, "tie-a", base, base.Add(3*time.Minute), base, "tie-a"),
	}
	groups := selectSandboxEvents([]*corev1.Pod{firstPod, secondPod}, gateway.IndexSandboxes([]*unstructured.Unstructured{sandbox("live", "writer")}), events, map[string]struct{}{"live": {}}, 10)
	if len(groups) != 1 {
		t.Fatalf("groups = %#v", groups)
	}
	want := []string{"event-time-wins", "series-newer", "creation-newer", "creation-older", "tie-a", "tie-b"}
	if len(groups[0].events) != len(want) {
		t.Fatalf("events = %#v", groups[0].events)
	}
	for i, reason := range want {
		if groups[0].events[i].GetReason() != reason {
			t.Fatalf("event %d reason = %q, want %q", i, groups[0].events[i].GetReason(), reason)
		}
	}
}

func TestSelectSandboxEventsAssociatesUIDLessEventsByPodNameAndNamespace(t *testing.T) {
	p := pod("live", "pod", "pod-uid", "writer", corev1.PodPending)
	uidLess := event(p, "uid-less", time.Time{}, time.Time{}, time.Now(), "uid-less")
	uidLess.InvolvedObject.UID = ""
	groups := selectSandboxEvents([]*corev1.Pod{p}, gateway.IndexSandboxes([]*unstructured.Unstructured{sandbox("live", "writer")}), []*corev1.Event{uidLess}, map[string]struct{}{"live": {}}, 5)
	if len(groups) != 1 || len(groups[0].events) != 1 || groups[0].events[0].GetReason() != "uid-less" {
		t.Fatalf("groups = %#v", groups)
	}
}

func TestProtoEventPreservesIndependentObservationTimes(t *testing.T) {
	base := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	value := &corev1.Event{
		EventTime:      metav1.MicroTime{Time: base.Add(time.Minute)},
		FirstTimestamp: metav1.NewTime(base),
		Series:         &corev1.EventSeries{LastObservedTime: metav1.NewMicroTime(base.Add(2 * time.Minute))},
	}
	got := protoEvent(value)
	if got.GetFirstObservedAt().AsTime() != base || got.GetLastObservedAt().AsTime() != base.Add(2*time.Minute) {
		t.Fatalf("observation times = first %v, last %v", got.GetFirstObservedAt(), got.GetLastObservedAt())
	}

	value.FirstTimestamp = metav1.Time{}
	value.Series = nil
	value.LastTimestamp = metav1.NewTime(base.Add(3 * time.Minute))
	got = protoEvent(value)
	if got.GetFirstObservedAt().AsTime() != base.Add(time.Minute) || got.GetLastObservedAt().AsTime() != base.Add(3*time.Minute) {
		t.Fatalf("EventTime and deprecated last-observed fallback = first %v, last %v", got.GetFirstObservedAt(), got.GetLastObservedAt())
	}

	value.LastTimestamp = metav1.Time{}
	got = protoEvent(value)
	if got.GetFirstObservedAt().AsTime() != base.Add(time.Minute) || got.GetLastObservedAt() != nil {
		t.Fatalf("EventTime-only fallback = first %v, last %v", got.GetFirstObservedAt(), got.GetLastObservedAt())
	}
}

func TestSelectSandboxEventsFallsBackOnlyWithoutOwner(t *testing.T) {
	sandbox := sandbox("live", "pod")
	p := pod("live", "pod", "uid", "missing", corev1.PodRunning)
	if groups := selectSandboxEvents([]*corev1.Pod{p}, gateway.IndexSandboxes([]*unstructured.Unstructured{sandbox}), nil, map[string]struct{}{"live": {}}, 5); len(groups) != 0 {
		t.Fatalf("owner ref must not fallback: %#v", groups)
	}
}

func TestReconcileOnceReplacesSnapshotWithEmptyReport(t *testing.T) {
	p := pod("live", "pod", "uid", "writer", corev1.PodRunning)
	events := []*corev1.Event{event(p, "old", time.Time{}, time.Time{}, time.Now(), "old")}
	snapshots := map[string][]*pb.SandboxEvent{}
	r := newFakeSandboxEventsReconciler([]*corev1.Pod{p}, []*unstructured.Unstructured{sandbox("live", "writer")}, &events, func(_ context.Context, namespace, name string, values []*pb.SandboxEvent) error {
		snapshots[namespace+"/"+name] = append([]*pb.SandboxEvent(nil), values...)
		return nil
	})
	r.reconcileOnce(context.Background())
	if len(snapshots["live/writer"]) != 1 {
		t.Fatalf("initial snapshot = %#v", snapshots)
	}

	events = nil
	r.reconcileOnce(context.Background())
	if got := snapshots["live/writer"]; len(got) != 0 {
		t.Fatalf("empty full snapshot did not clear prior values: %#v", got)
	}
}

func TestSandboxEventsReconcilerContinuesAfterFailedReportAndNextTick(t *testing.T) {
	pods := []*corev1.Pod{pod("live", "a-pod", "a-uid", "a", corev1.PodRunning), pod("live", "b-pod", "b-uid", "b", corev1.PodFailed)}
	var events []*corev1.Event
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	calls := map[string]int{}
	r := newFakeSandboxEventsReconciler(pods, []*unstructured.Unstructured{sandbox("live", "a"), sandbox("live", "b")}, &events, func(_ context.Context, _, name string, _ []*pb.SandboxEvent) error {
		mu.Lock()
		defer mu.Unlock()
		calls[name]++
		if calls["a"] >= 2 && calls["b"] >= 2 {
			cancel()
		}
		if name == "a" {
			return errors.New("report failed")
		}
		return nil
	})
	r.interval = time.Millisecond
	if err := r.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context cancellation", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls["a"] < 2 || calls["b"] < 2 {
		t.Fatalf("reports = %#v, want failed report not to stop siblings or next tick", calls)
	}
}

type deadlineGatewayServer struct {
	pb.UnimplementedGatewayServiceServer
	deadline chan time.Duration
}

func (s *deadlineGatewayServer) ReportSandboxEvents(ctx context.Context, _ *pb.ReportSandboxEventsRequest) (*pb.ReportSandboxEventsResponse, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		s.deadline <- 0
		return nil, errors.New("missing RPC deadline")
	}
	s.deadline <- time.Until(deadline)
	return &pb.ReportSandboxEventsResponse{}, nil
}

func TestGRPCReportUsesTenSecondTimeout(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	handler := &deadlineGatewayServer{deadline: make(chan time.Duration, 1)}
	pb.RegisterGatewayServiceServer(server, handler)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("passthrough:///sandbox-events", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("new gRPC client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	r := &SandboxEventsReconciler{grpcConn: conn}
	if err := r.grpcReport(context.Background(), "live", "writer", nil); err != nil {
		t.Fatalf("grpc report: %v", err)
	}
	remaining := <-handler.deadline
	if remaining < 9*time.Second || remaining > sandboxEventsRPCTimeout {
		t.Fatalf("RPC deadline remaining = %s, want approximately %s", remaining, sandboxEventsRPCTimeout)
	}
}

func newFakeSandboxEventsReconciler(pods []*corev1.Pod, sandboxes []*unstructured.Unstructured, events *[]*corev1.Event, report func(context.Context, string, string, []*pb.SandboxEvent) error) *SandboxEventsReconciler {
	return &SandboxEventsReconciler{
		limit:    5,
		listPods: func(context.Context) ([]*corev1.Pod, error) { return pods, nil },
		listSandboxes: func(context.Context) ([]*unstructured.Unstructured, error) {
			return sandboxes, nil
		},
		listEvents:     func(_ context.Context, namespaces []string) ([]*corev1.Event, error) { return *events, nil },
		liveNamespaces: func(context.Context) (map[string]struct{}, error) { return map[string]struct{}{"live": {}}, nil },
		report:         report,
	}
}

func sandbox(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{"metadata": map[string]interface{}{"name": name, "namespace": namespace}}}
}

func pod(namespace, name, uid, sandboxName string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: types.UID(uid), Labels: map[string]string{gateway.SandboxPodSelector: "h"}, OwnerReferences: []metav1.OwnerReference{{Kind: "Sandbox", Name: sandboxName}}}, Status: corev1.PodStatus{Phase: phase}}
}

func event(pod *corev1.Pod, name string, eventTime, lastObserved, created time.Time, reason string) *corev1.Event {
	value := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: pod.Namespace, CreationTimestamp: metav1.NewTime(created)}, InvolvedObject: corev1.ObjectReference{Namespace: pod.Namespace, Name: pod.Name, UID: pod.UID}, Reason: reason}
	if !eventTime.IsZero() {
		value.EventTime = metav1.NewMicroTime(eventTime)
	}
	if !lastObserved.IsZero() {
		value.Series = &corev1.EventSeries{LastObservedTime: metav1.NewMicroTime(lastObserved)}
	}
	return value
}

// TestKubernetesEventsListsOnlyLiveNamespaces guards the HYPERSHELL-422 e2e
// regression: a cluster-wide Event list OOM-killed the memory-limited
// controller on the shared PR environment cluster. The list seam MUST receive
// exactly the live gateway namespaces and kubernetesEvents MUST issue one
// namespace-scoped list per entry, never a cluster-wide ("") list.
func TestKubernetesEventsListsOnlyLiveNamespaces(t *testing.T) {
	requested := make([]string, 0, 2)
	r := &SandboxEventsReconciler{
		limit:         5,
		listPods:      func(context.Context) ([]*corev1.Pod, error) { return nil, nil },
		listSandboxes: func(context.Context) ([]*unstructured.Unstructured, error) { return nil, nil },
		liveNamespaces: func(context.Context) (map[string]struct{}, error) {
			return map[string]struct{}{"openshell-b": {}, "openshell-a": {}}, nil
		},
		report: func(context.Context, string, string, []*pb.SandboxEvent) error { return nil },
	}
	r.listEvents = func(_ context.Context, namespaces []string) ([]*corev1.Event, error) {
		requested = append(requested, namespaces...)
		return nil, nil
	}
	r.reconcileOnce(context.Background())
	if len(requested) != 2 || requested[0] != "openshell-a" || requested[1] != "openshell-b" {
		t.Fatalf("event list namespaces = %v, want sorted live namespaces [openshell-a openshell-b]", requested)
	}

	client := fake.NewSimpleClientset(
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "e1", Namespace: "openshell-a"}},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "e2", Namespace: "other-tenant"}},
	)
	scoped := &SandboxEventsReconciler{client: client}
	got, err := scoped.kubernetesEvents(context.Background(), []string{"openshell-a"})
	if err != nil {
		t.Fatalf("kubernetes events: %v", err)
	}
	if len(got) != 1 || got[0].Name != "e1" {
		t.Fatalf("scoped events = %v, want only e1 from openshell-a", got)
	}
	for _, action := range client.Actions() {
		if action.GetVerb() != "list" {
			continue
		}
		if action.GetNamespace() == "" {
			t.Fatalf("cluster-wide event list issued; want namespace-scoped lists only")
		}
	}
}
