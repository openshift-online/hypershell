package gateway

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func pod(name, namespace string, phase corev1.PodPhase, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Status:     corev1.PodStatus{Phase: phase},
	}
}

func crashLoopPod(name string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.PodStatus{
			// A crash-looping pod's phase stays Running; the state is only visible
			// on the container waiting reason.
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
			},
		},
	}
}

func phasePod(name string, phase corev1.PodPhase) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status:     corev1.PodStatus{Phase: phase},
	}
}

func TestClassifyPod(t *testing.T) {
	tests := []struct {
		name string
		pod  corev1.Pod
		want string
	}{
		{"running", phasePod("p", corev1.PodRunning), PodStateRunning},
		{"succeeded is completed", phasePod("p", corev1.PodSucceeded), PodStateCompleted},
		{"pending", phasePod("p", corev1.PodPending), PodStatePending},
		{"failed", phasePod("p", corev1.PodFailed), PodStateFailed},
		{"unknown", phasePod("p", corev1.PodUnknown), PodStateUnknown},
		{"crashloop detected from waiting reason", crashLoopPod("p"), PodStateCrashLoopBackOff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyPod(&tt.pod); got != tt.want {
				t.Errorf("classifyPod() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummarizeNamespacePods(t *testing.T) {
	pods := []corev1.Pod{
		phasePod("a", corev1.PodRunning),
		phasePod("b", corev1.PodRunning),
		crashLoopPod("c"),
		phasePod("d", corev1.PodSucceeded),
	}

	summary := SummarizeNamespacePods(pods)

	if summary.Total != 4 {
		t.Fatalf("Total = %d, want 4", summary.Total)
	}
	if summary.States[PodStateRunning] != 2 {
		t.Errorf("Running = %d, want 2", summary.States[PodStateRunning])
	}
	if summary.States[PodStateCrashLoopBackOff] != 1 {
		t.Errorf("CrashLoopBackOff = %d, want 1", summary.States[PodStateCrashLoopBackOff])
	}
	if summary.States[PodStateCompleted] != 1 {
		t.Errorf("Completed = %d, want 1", summary.States[PodStateCompleted])
	}

	want := "4 pods: 2 Running, 1 CrashLoopBackOff, 1 Completed"
	if got := summary.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestSummarizeNamespacePodsEmpty(t *testing.T) {
	if got := SummarizeNamespacePods(nil).String(); got != "0 pods" {
		t.Errorf("String() = %q, want %q", got, "0 pods")
	}
}

func TestCountActiveSandboxes(t *testing.T) {
	const ns = "openshell-gw"
	client := fake.NewSimpleClientset(
		// active sandbox
		pod("sb-running", ns, corev1.PodRunning, map[string]string{sandboxPodLabelKey: "abc123"}),
		// active sandbox still starting
		pod("sb-pending", ns, corev1.PodPending, map[string]string{sandboxPodLabelKey: "def456"}),
		// terminated sandbox -> not active
		pod("sb-done", ns, corev1.PodSucceeded, map[string]string{sandboxPodLabelKey: "ghi789"}),
		// gateway workload -> excluded (no sandbox label)
		pod("openshell-gateway-1", ns, corev1.PodRunning, map[string]string{"app": "openshell-gateway"}),
		// gateway database -> excluded
		pod("openshell-db-0", ns, corev1.PodRunning, nil),
	)

	got, err := CountActiveSandboxes(context.Background(), client, ns)
	if err != nil {
		t.Fatalf("CountActiveSandboxes() error = %v", err)
	}
	if got != 2 {
		t.Errorf("CountActiveSandboxes() = %d, want 2", got)
	}
}

func TestIsActiveSandboxPod(t *testing.T) {
	sandbox := map[string]string{sandboxPodLabelKey: "abc123"}
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{"running sandbox is active", pod("p", "ns", corev1.PodRunning, sandbox), true},
		{"pending sandbox is active", pod("p", "ns", corev1.PodPending, sandbox), true},
		{"succeeded sandbox is not active", pod("p", "ns", corev1.PodSucceeded, sandbox), false},
		{"failed sandbox is not active", pod("p", "ns", corev1.PodFailed, sandbox), false},
		{"running non-sandbox is not counted", pod("p", "ns", corev1.PodRunning, map[string]string{"app": "openshell-gateway"}), false},
		{"unlabelled pod is not counted", pod("p", "ns", corev1.PodRunning, nil), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsActiveSandboxPod(tt.pod); got != tt.want {
				t.Errorf("IsActiveSandboxPod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCountActiveSandboxesEmpty(t *testing.T) {
	client := fake.NewSimpleClientset()
	got, err := CountActiveSandboxes(context.Background(), client, "openshell-gw")
	if err != nil {
		t.Fatalf("CountActiveSandboxes() error = %v", err)
	}
	if got != 0 {
		t.Errorf("CountActiveSandboxes() = %d, want 0", got)
	}
}
