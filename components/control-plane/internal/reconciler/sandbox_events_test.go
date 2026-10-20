package reconciler

import (
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

func TestSelectSandboxEventsCorrelatesAndSelectsNewestAcrossTerminatedPods(t *testing.T) {
	now := time.Now().UTC()
	sandbox := &unstructured.Unstructured{Object: map[string]interface{}{"metadata": map[string]interface{}{"name": "writer", "namespace": "live"}}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "live", UID: types.UID("pod-uid"), Labels: map[string]string{gateway.SandboxPodSelector: "h"}, OwnerReferences: []metav1.OwnerReference{{Kind: "Sandbox", Name: "writer"}}}, Status: corev1.PodStatus{Phase: corev1.PodFailed}}
	events := []*corev1.Event{{ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: "live", CreationTimestamp: metav1.NewTime(now.Add(-time.Hour))}, InvolvedObject: corev1.ObjectReference{Namespace: "live", Name: "pod", UID: pod.UID}, Reason: "old"}, {ObjectMeta: metav1.ObjectMeta{Name: "new", Namespace: "live", CreationTimestamp: metav1.NewTime(now)}, InvolvedObject: corev1.ObjectReference{Namespace: "live", Name: "pod", UID: pod.UID}, Reason: "new"}, {InvolvedObject: corev1.ObjectReference{Namespace: "live", Name: "other"}, Reason: "ignored"}}
	groups := selectSandboxEvents([]*corev1.Pod{pod}, gateway.IndexSandboxes([]*unstructured.Unstructured{sandbox}), events, map[string]struct{}{"live": {}}, 5)
	if len(groups) != 1 || len(groups[0].events) != 2 || groups[0].events[0].GetReason() != "new" {
		t.Fatalf("groups = %#v", groups)
	}
}
func TestSelectSandboxEventsFallsBackOnlyWithoutOwner(t *testing.T) {
	sandbox := &unstructured.Unstructured{Object: map[string]interface{}{"metadata": map[string]interface{}{"name": "pod", "namespace": "live"}}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "live", Labels: map[string]string{gateway.SandboxPodSelector: "h"}, OwnerReferences: []metav1.OwnerReference{{Kind: "Sandbox", Name: "missing"}}}}
	if groups := selectSandboxEvents([]*corev1.Pod{pod}, gateway.IndexSandboxes([]*unstructured.Unstructured{sandbox}), nil, map[string]struct{}{"live": {}}, 5); len(groups) != 0 {
		t.Fatalf("owner ref must not fallback: %#v", groups)
	}
}
