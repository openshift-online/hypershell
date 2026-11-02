package gateway

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestEnsureControlPlaneNamespaceLabel(t *testing.T) {
	t.Run("labels an unlabeled namespace", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "hyp2"}})
		if err := EnsureControlPlaneNamespaceLabel(context.Background(), cs, "hyp2"); err != nil {
			t.Fatalf("EnsureControlPlaneNamespaceLabel: %v", err)
		}
		ns, _ := cs.CoreV1().Namespaces().Get(context.Background(), "hyp2", metav1.GetOptions{})
		if ns.Labels[ControlPlaneLabel] != ControlPlaneLabelValue {
			t.Fatalf("label = %q; want %q", ns.Labels[ControlPlaneLabel], ControlPlaneLabelValue)
		}
	})

	t.Run("idempotent when already labeled", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   "hyp2",
			Labels: map[string]string{ControlPlaneLabel: ControlPlaneLabelValue},
		}})
		if err := EnsureControlPlaneNamespaceLabel(context.Background(), cs, "hyp2"); err != nil {
			t.Fatalf("idempotent call: %v", err)
		}
	})

	t.Run("preserves existing labels", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   "hyp2",
			Labels: map[string]string{"observability.hypershell.io/stack": "hypershell"},
		}})
		if err := EnsureControlPlaneNamespaceLabel(context.Background(), cs, "hyp2"); err != nil {
			t.Fatalf("EnsureControlPlaneNamespaceLabel: %v", err)
		}
		ns, _ := cs.CoreV1().Namespaces().Get(context.Background(), "hyp2", metav1.GetOptions{})
		if ns.Labels["observability.hypershell.io/stack"] != "hypershell" {
			t.Errorf("existing label dropped: %v", ns.Labels)
		}
		if ns.Labels[ControlPlaneLabel] != ControlPlaneLabelValue {
			t.Errorf("control-plane label not applied: %v", ns.Labels)
		}
	})

	t.Run("empty namespace is an error", func(t *testing.T) {
		cs := k8sfake.NewSimpleClientset()
		if err := EnsureControlPlaneNamespaceLabel(context.Background(), cs, ""); err == nil {
			t.Fatal("expected error for empty namespace")
		}
	})
}
