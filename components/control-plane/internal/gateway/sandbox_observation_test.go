package gateway

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestObservationFromPodUsesSandboxFields(t *testing.T) {
	created := metav1.NewTime(time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC))
	shutdown := time.Date(2026, 9, 21, 18, 0, 0, 0, time.UTC)
	activity := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "sb-1-pod",
			Namespace:         "openshell-live",
			CreationTimestamp: metav1.NewTime(created.Add(-time.Hour)),
			Labels:            map[string]string{sandboxPodLabelKey: "hash"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "agents.x-k8s.io/v1beta1",
				Kind:       "Sandbox",
				Name:       "sb-1",
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	sandbox := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "agents.x-k8s.io/v1beta1",
		"kind":       "Sandbox",
		"metadata": map[string]interface{}{
			"name":              "sb-1",
			"namespace":         "openshell-live",
			"creationTimestamp": created.UTC().Format(time.RFC3339),
		},
		"spec": map[string]interface{}{
			"lifecycle": map[string]interface{}{
				"shutdownTime": shutdown.Format(time.RFC3339),
			},
		},
		"status": map[string]interface{}{
			"lastActivityTime": activity.Format(time.RFC3339),
		},
	}}

	obs := ObservationFromPod(pod, sandbox)
	if !obs.Active {
		t.Fatal("expected active observation")
	}
	if obs.Name != "sb-1" {
		t.Fatalf("Name = %q, want sb-1", obs.Name)
	}
	if obs.CreationTime.UTC() != created.UTC() {
		t.Fatalf("CreationTime = %v, want %v", obs.CreationTime, created.Time)
	}
	if obs.ShutdownTime == nil || !obs.ShutdownTime.Equal(shutdown) {
		t.Fatalf("ShutdownTime = %v, want %v", obs.ShutdownTime, shutdown)
	}
	if obs.LastActivityTime == nil || !obs.LastActivityTime.Equal(activity) {
		t.Fatalf("LastActivityTime = %v, want %v", obs.LastActivityTime, activity)
	}
}

func TestObservationFromPodUsesOwnerReferenceWhenSandboxIsMissing(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sb-1-pod",
			Namespace: "openshell-live",
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "Sandbox",
				Name: "sb-1",
			}},
		},
	}

	obs := ObservationFromPod(pod, nil)

	if obs.Name != "sb-1" {
		t.Fatalf("Name = %q, want sb-1", obs.Name)
	}
}

func TestFindSandboxForPodByOwnerAndName(t *testing.T) {
	sb := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "sb-a", "namespace": "ns"},
	}}
	lookup := IndexSandboxes([]*unstructured.Unstructured{sb})

	owned := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "pod-other",
		Namespace: "ns",
		OwnerReferences: []metav1.OwnerReference{{
			Kind: "Sandbox",
			Name: "sb-a",
		}},
	}}
	if got := FindSandboxForPod(owned, lookup); got != sb {
		t.Fatalf("owner lookup failed: got %#v", got)
	}

	named := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "sb-a", Namespace: "ns"}}
	if got := FindSandboxForPod(named, lookup); got != sb {
		t.Fatalf("name lookup failed: got %#v", got)
	}
}

func TestBuildAttentionObservationsFiltersNamespaces(t *testing.T) {
	pods := []*corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "a",
				Namespace: "openshell-eligible",
				Labels:    map[string]string{sandboxPodLabelKey: "h"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "b",
				Namespace: "other",
				Labels:    map[string]string{sandboxPodLabelKey: "h"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}
	eligible := map[string]struct{}{"openshell-eligible": {}}
	got := BuildAttentionObservations(pods, nil, eligible)
	if len(got) != 1 || got[0].Namespace != "openshell-eligible" {
		t.Fatalf("got %#v", got)
	}
}

func TestProvisionedSandboxCounts(t *testing.T) {
	sandboxes := []*unstructured.Unstructured{
		{Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "a", "namespace": "openshell-a"},
		}},
		{Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "b", "namespace": "openshell-a"},
		}},
		{Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "c", "namespace": "other"},
		}},
		nil,
	}
	eligible := map[string]struct{}{"openshell-a": {}, "openshell-empty": {}}

	got := ProvisionedSandboxCounts(sandboxes, eligible)
	want := map[string]int{"openshell-a": 2, "openshell-empty": 0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ProvisionedSandboxCounts() = %#v, want %#v", got, want)
	}
}

func TestReconcileActiveSandboxes(t *testing.T) {
	created := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	observations := []SandboxObservation{
		{Namespace: "openshell-a", Name: "keep", Active: true, CreationTime: created},
		{Namespace: "openshell-a", Name: "leave", Active: false, CreationTime: created},
		{Namespace: "openshell-b", Active: true, CreationTime: created},
	}

	current, departed := ReconcileActiveSandboxes(nil, observations)
	if len(current) != 1 || current["openshell-a/keep"].Name != "keep" {
		t.Fatalf("initial current = %#v, want only openshell-a/keep", current)
	}
	if len(departed) != 0 {
		t.Fatalf("initial departed = %#v, want none", departed)
	}

	previous := map[string]SandboxObservation{
		"openshell-a/keep":  observations[0],
		"openshell-a/leave": {Namespace: "openshell-a", Name: "leave", Active: true, CreationTime: created},
	}
	current, departed = ReconcileActiveSandboxes(previous, observations)
	if len(current) != 1 || current["openshell-a/keep"].Name != "keep" {
		t.Fatalf("current = %#v, want only openshell-a/keep", current)
	}
	if len(departed) != 1 || departed[0].Name != "leave" {
		t.Fatalf("departed = %#v, want openshell-a/leave", departed)
	}
}

func TestSandboxLifetime(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	created := now.Add(-3 * time.Hour)

	if lifetime, ok := SandboxLifetime(SandboxObservation{CreationTime: created}, now); !ok || lifetime != 3*time.Hour {
		t.Fatalf("SandboxLifetime() = %v, %v; want 3h, true", lifetime, ok)
	}
	if _, ok := SandboxLifetime(SandboxObservation{}, now); ok {
		t.Fatal("SandboxLifetime() succeeded without a creation time")
	}
	if _, ok := SandboxLifetime(SandboxObservation{CreationTime: now.Add(time.Minute)}, now); ok {
		t.Fatal("SandboxLifetime() succeeded with a future creation time")
	}
}
