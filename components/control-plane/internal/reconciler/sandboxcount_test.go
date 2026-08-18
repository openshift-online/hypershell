package reconciler

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// scPod builds a pod for the sandbox-count tests. A sandbox pod carries the
// sandbox-name-hash label the informer selects on; a non-sandbox pod carries a
// gateway-workload label instead, so it must never be counted.
func scPod(name, namespace string, phase corev1.PodPhase, sandbox bool) *corev1.Pod {
	labels := map[string]string{}
	if sandbox {
		labels[gateway.SandboxPodSelector] = "hash-" + name
	} else {
		labels["app"] = "openshell-gateway"
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Status:     corev1.PodStatus{Phase: phase},
	}
}

// podLister builds a PodLister backed by a plain indexer so selfHeal can be
// exercised synchronously without starting an informer.
func podLister(pods ...*corev1.Pod) corelisters.PodLister {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	for _, p := range pods {
		_ = indexer.Add(p)
	}
	return corelisters.NewPodLister(indexer)
}

// seamRecorder captures the RPC seams the reconciler would otherwise drive over
// gRPC, so the reconciliation logic is testable without a live API server.
type seamRecorder struct {
	mu      sync.Mutex
	adjusts map[string]int // namespace -> summed delta
	sets    map[string]int // namespace -> last absolute count set
	nsList  []string
	nsErr   error
	adjErr  error
	setErr  error
}

func (rec *seamRecorder) adjust(ns string, delta int) int {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return rec.adjusts[ns]
}

// newTestSandboxCount builds a SandboxCountReconciler with recording seams and
// no gRPC connection, following the namespace_test.go convention of exercising
// the pure sub-methods directly.
func newTestSandboxCount(rec *seamRecorder) *SandboxCountReconciler {
	r := NewSandboxCountReconciler(fake.NewSimpleClientset(), nil, time.Minute)
	r.adjust = func(ctx context.Context, ns string, delta int) error {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		if rec.adjusts == nil {
			rec.adjusts = map[string]int{}
		}
		rec.adjusts[ns] += delta
		return rec.adjErr
	}
	r.set = func(ctx context.Context, ns string, count int) error {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		if rec.sets == nil {
			rec.sets = map[string]int{}
		}
		rec.sets[ns] = count
		return rec.setErr
	}
	r.namespaces = func(ctx context.Context) ([]string, error) {
		return rec.nsList, rec.nsErr
	}
	return r
}

func TestOnAdd(t *testing.T) {
	const ns = "openshell-a"
	tests := []struct {
		name   string
		synced bool
		pod    *corev1.Pod
		want   int
	}{
		{"active sandbox increments once synced", true, scPod("s", ns, corev1.PodRunning, true), 1},
		{"pending sandbox increments", true, scPod("s", ns, corev1.PodPending, true), 1},
		{"initial-list add ignored before sync", false, scPod("s", ns, corev1.PodRunning, true), 0},
		{"terminated sandbox ignored", true, scPod("s", ns, corev1.PodSucceeded, true), 0},
		{"non-sandbox pod ignored", true, scPod("s", ns, corev1.PodRunning, false), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &seamRecorder{}
			r := newTestSandboxCount(rec)
			r.synced.Store(tt.synced)
			r.onAdd(tt.pod)
			if got := rec.adjust(ns, 0); got != tt.want {
				t.Errorf("adjust total = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestOnUpdate(t *testing.T) {
	const ns = "openshell-a"
	tests := []struct {
		name     string
		old, new *corev1.Pod
		want     int
	}{
		{"pending to running is active-to-active no-op", scPod("s", ns, corev1.PodPending, true), scPod("s", ns, corev1.PodRunning, true), 0},
		{"running to succeeded decrements", scPod("s", ns, corev1.PodRunning, true), scPod("s", ns, corev1.PodSucceeded, true), -1},
		{"unknown to running increments", scPod("s", ns, corev1.PodUnknown, true), scPod("s", ns, corev1.PodRunning, true), 1},
		{"running to unknown decrements", scPod("s", ns, corev1.PodRunning, true), scPod("s", ns, corev1.PodUnknown, true), -1},
		{"resync of unchanged active is no-op", scPod("s", ns, corev1.PodRunning, true), scPod("s", ns, corev1.PodRunning, true), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &seamRecorder{}
			r := newTestSandboxCount(rec)
			r.synced.Store(true)
			r.onUpdate(tt.old, tt.new)
			if got := rec.adjust(ns, 0); got != tt.want {
				t.Errorf("adjust total = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestOnDelete(t *testing.T) {
	const ns = "openshell-a"
	tests := []struct {
		name string
		obj  interface{}
		want int
	}{
		{"active sandbox decrements", scPod("s", ns, corev1.PodRunning, true), -1},
		{"terminated sandbox no-op", scPod("s", ns, corev1.PodSucceeded, true), 0},
		{"non-sandbox pod no-op", scPod("s", ns, corev1.PodRunning, false), 0},
		{"tombstone-wrapped active decrements", cache.DeletedFinalStateUnknown{Key: ns + "/s", Obj: scPod("s", ns, corev1.PodRunning, true)}, -1},
		{"tombstone-wrapped non-pod ignored", cache.DeletedFinalStateUnknown{Key: "x", Obj: "not-a-pod"}, 0},
		{"unexpected type ignored", "not-a-pod", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &seamRecorder{}
			r := newTestSandboxCount(rec)
			r.synced.Store(true)
			r.onDelete(tt.obj)
			if got := rec.adjust(ns, 0); got != tt.want {
				t.Errorf("adjust total = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestActiveSandboxesByNamespace(t *testing.T) {
	pods := []*corev1.Pod{
		scPod("a1", "openshell-a", corev1.PodRunning, true),
		scPod("a2", "openshell-a", corev1.PodPending, true),
		scPod("a3", "openshell-a", corev1.PodSucceeded, true), // terminated
		scPod("agw", "openshell-a", corev1.PodRunning, false), // non-sandbox
		scPod("b1", "openshell-b", corev1.PodRunning, true),
	}
	got := activeSandboxesByNamespace(pods)
	want := map[string]int{"openshell-a": 2, "openshell-b": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("activeSandboxesByNamespace() = %v, want %v", got, want)
	}
}

// TestSelfHeal_SetsAbsoluteForEveryGatewayNamespace verifies the self-heal sets
// the cache-observed absolute count for every gateway namespace - including
// those with no active sandboxes, so a drifted or stale count converges back to
// zero. With no incremental events between cache sync and self-heal, this is
// exactly the post-restart recovery path.
func TestSelfHeal_SetsAbsoluteForEveryGatewayNamespace(t *testing.T) {
	rec := &seamRecorder{nsList: []string{"openshell-a", "openshell-b", "openshell-c"}}
	r := newTestSandboxCount(rec)
	lister := podLister(
		scPod("a1", "openshell-a", corev1.PodRunning, true),
		scPod("a2", "openshell-a", corev1.PodPending, true),
		scPod("a3", "openshell-a", corev1.PodSucceeded, true), // terminated, not counted
		scPod("agw", "openshell-a", corev1.PodRunning, false), // non-sandbox, not counted
		scPod("b1", "openshell-b", corev1.PodFailed, true),    // terminated, not counted
		// openshell-c has no pods in cache and must be set to zero.
	)

	r.selfHeal(context.Background(), lister)

	want := map[string]int{"openshell-a": 2, "openshell-b": 0, "openshell-c": 0}
	if !reflect.DeepEqual(rec.sets, want) {
		t.Errorf("sets = %v, want %v", rec.sets, want)
	}
}

func TestSelfHeal_NamespaceListErrorSkipsSet(t *testing.T) {
	rec := &seamRecorder{nsErr: errors.New("boom")}
	r := newTestSandboxCount(rec)
	lister := podLister(scPod("a1", "openshell-a", corev1.PodRunning, true))

	r.selfHeal(context.Background(), lister)

	if len(rec.sets) != 0 {
		t.Errorf("sets = %v, want no writes when gateway namespace listing fails", rec.sets)
	}
}

// TestConcurrentAddsAndDeletesNetToZero fires interleaved add/delete events for
// the same pods concurrently; the net delta must be zero with no lost updates.
// Run under -race, this also guards the handler path against data races.
func TestConcurrentAddsAndDeletesNetToZero(t *testing.T) {
	const ns = "openshell-a"
	rec := &seamRecorder{}
	r := newTestSandboxCount(rec)
	r.synced.Store(true)

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		p := scPod(fmt.Sprintf("sb-%d", i), ns, corev1.PodRunning, true)
		wg.Add(2)
		go func() { defer wg.Done(); r.onAdd(p) }()
		go func() { defer wg.Done(); r.onDelete(p) }()
	}
	wg.Wait()

	if got := rec.adjust(ns, 0); got != 0 {
		t.Errorf("net delta = %d, want 0", got)
	}
}
