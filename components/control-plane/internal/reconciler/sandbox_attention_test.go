package reconciler

import (
	"context"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSandboxAttentionReconcileOnce(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	instance := "hypershell-system"

	managedNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openshell-orphan",
			Labels: map[string]string{
				gateway.ManagedLabel:   gateway.ManagedLabelValue,
				gateway.ManagedByLabel: gateway.ManagedByValue,
				gateway.InstanceLabel:  instance,
			},
		},
	}
	liveNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openshell-live",
			Labels: map[string]string{
				gateway.ManagedLabel:   gateway.ManagedLabelValue,
				gateway.ManagedByLabel: gateway.ManagedByValue,
				gateway.InstanceLabel:  instance,
			},
		},
	}

	orphanPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "orphan-sb",
			Namespace:         "openshell-orphan",
			CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Hour)),
			Labels:            map[string]string{gateway.SandboxPodSelector: "h1"},
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "Sandbox",
				Name: "orphan-sb",
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	idlePod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "idle-sb",
			Namespace:         "openshell-live",
			CreationTimestamp: metav1.NewTime(now.Add(-30 * time.Hour)),
			Labels:            map[string]string{gateway.SandboxPodSelector: "h2"},
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "Sandbox",
				Name: "idle-sb",
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	succeededPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "done-sb",
			Namespace: "openshell-orphan",
			Labels:    map[string]string{gateway.SandboxPodSelector: "h3"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
	}

	orphanSandbox := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":              "orphan-sb",
			"namespace":         "openshell-orphan",
			"creationTimestamp": now.Add(-2 * time.Hour).Format(time.RFC3339),
		},
		"spec": map[string]interface{}{
			"lifecycle": map[string]interface{}{
				"shutdownTime": now.Add(6 * time.Hour).Format(time.RFC3339),
			},
		},
	}}
	idleSandbox := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":              "idle-sb",
			"namespace":         "openshell-live",
			"creationTimestamp": now.Add(-30 * time.Hour).Format(time.RFC3339),
		},
	}}

	var gotCounts gateway.AttentionCounts
	r := &SandboxAttentionReconciler{
		interval:  time.Minute,
		clusterID: "cluster-a",
		instance:  instance,
		now:       func() time.Time { return now },
		liveNamespaces: func(ctx context.Context) (map[string]struct{}, error) {
			return map[string]struct{}{"openshell-live": {}}, nil
		},
		listNamespaces: func(ctx context.Context) ([]*corev1.Namespace, error) {
			return []*corev1.Namespace{managedNS, liveNS}, nil
		},
		listPods: func(ctx context.Context) ([]*corev1.Pod, error) {
			return []*corev1.Pod{orphanPod, idlePod, succeededPod}, nil
		},
		listSandboxes: func(ctx context.Context) ([]*unstructured.Unstructured, error) {
			return []*unstructured.Unstructured{orphanSandbox, idleSandbox}, nil
		},
		publish: func(ctx context.Context, counts gateway.AttentionCounts) {
			gotCounts = counts
		},
	}

	r.reconcileOnce(context.Background())

	want := gateway.AttentionCounts{Orphaned: 1, Expiring: 1, Idle: 1}
	if gotCounts != want {
		t.Fatalf("counts = %+v, want %+v", gotCounts, want)
	}
}

func TestSandboxAttentionReconcileOnceDegradesWhenSandboxCRsFail(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	instance := "hypershell-system"

	managedNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openshell-orphan",
			Labels: map[string]string{
				gateway.ManagedLabel:   gateway.ManagedLabelValue,
				gateway.ManagedByLabel: gateway.ManagedByValue,
				gateway.InstanceLabel:  instance,
			},
		},
	}
	liveNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openshell-live",
			Labels: map[string]string{
				gateway.ManagedLabel:   gateway.ManagedLabelValue,
				gateway.ManagedByLabel: gateway.ManagedByValue,
				gateway.InstanceLabel:  instance,
			},
		},
	}
	orphanPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "orphan-sb",
			Namespace:         "openshell-orphan",
			CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Hour)),
			Labels:            map[string]string{gateway.SandboxPodSelector: "h1"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	// Live-namespace pod older than IdleNeverUsedAge: without SSA-05 zeroing,
	// ClassifyAttentionCounts would count this as idle from pod CreationTime.
	wouldBeIdlePod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "would-be-idle-sb",
			Namespace:         "openshell-live",
			CreationTimestamp: metav1.NewTime(now.Add(-30 * time.Hour)),
			Labels:            map[string]string{gateway.SandboxPodSelector: "h2"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	var gotCounts gateway.AttentionCounts
	published := false
	r := &SandboxAttentionReconciler{
		interval:  time.Minute,
		clusterID: "cluster-a",
		instance:  instance,
		now:       func() time.Time { return now },
		liveNamespaces: func(ctx context.Context) (map[string]struct{}, error) {
			return map[string]struct{}{"openshell-live": {}}, nil
		},
		listNamespaces: func(ctx context.Context) ([]*corev1.Namespace, error) {
			return []*corev1.Namespace{managedNS, liveNS}, nil
		},
		listPods: func(ctx context.Context) ([]*corev1.Pod, error) {
			return []*corev1.Pod{orphanPod, wouldBeIdlePod}, nil
		},
		listSandboxes: func(ctx context.Context) ([]*unstructured.Unstructured, error) {
			return nil, context.DeadlineExceeded
		},
		publish: func(ctx context.Context, counts gateway.AttentionCounts) {
			published = true
			gotCounts = counts
		},
	}

	r.reconcileOnce(context.Background())

	if !published {
		t.Fatal("expected publish despite Sandbox CR list failure")
	}
	want := gateway.AttentionCounts{Orphaned: 1, Expiring: 0, Idle: 0}
	if gotCounts != want {
		t.Fatalf("counts = %+v, want %+v (expiring/idle must stay zero without CRs)", gotCounts, want)
	}
}

func TestSandboxAttentionReconcileOnceZerosOnHardFailure(t *testing.T) {
	var gotCounts gateway.AttentionCounts
	published := false
	r := &SandboxAttentionReconciler{
		interval:  time.Minute,
		clusterID: "cluster-a",
		instance:  "hypershell-system",
		now:       time.Now,
		liveNamespaces: func(ctx context.Context) (map[string]struct{}, error) {
			return nil, context.DeadlineExceeded
		},
		publish: func(ctx context.Context, counts gateway.AttentionCounts) {
			published = true
			gotCounts = counts
		},
	}

	r.reconcileOnce(context.Background())

	if !published {
		t.Fatal("expected zero publish on hard failure so gauges are not sticky")
	}
	if gotCounts != (gateway.AttentionCounts{}) {
		t.Fatalf("counts = %+v, want zeros", gotCounts)
	}
}
