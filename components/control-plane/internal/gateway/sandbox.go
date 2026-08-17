package gateway

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Sandbox pods in a gateway namespace are created by the upstream OpenShell
// gateway via the agent-sandbox controller (agents.x-k8s.io), not by this
// control plane. Every sandbox pod carries the sandbox-name-hash label the
// controller stamps on it; none of the gateway's own workloads (deployment,
// database, certgen job) do, so this label reliably distinguishes an occupied
// user session from gateway infrastructure. Older gateway builds instead
// labelled sandbox pods with the legacy openshell.ai/managed-by=openshell
// marker, so both are matched for robustness.
const (
	sandboxPodLabelKey      = "agents.x-k8s.io/sandbox-name-hash"
	sandboxLegacyLabelKey   = "openshell.ai/managed-by"
	sandboxLegacyLabelValue = "openshell"
)

// Pod state classifications used when summarizing a namespace's workloads for
// garbage-collection events and logs. These mirror the states an operator sees
// in `kubectl get pods` so the recorded reason is immediately recognizable.
const (
	PodStateRunning          = "Running"
	PodStateCrashLoopBackOff = "CrashLoopBackOff"
	PodStateCompleted        = "Completed"
	PodStatePending          = "Pending"
	PodStateFailed           = "Failed"
	PodStateUnknown          = "Unknown"
)

// hasSandboxLabel reports whether a pod is an agent sandbox (current or legacy
// labelling). Checking the label client-side keeps the count correct even
// against a fake clientset that does not honor server-side label selectors, and
// guards against a future gateway workload accidentally matching a broad
// selector.
func hasSandboxLabel(pod *corev1.Pod) bool {
	if _, ok := pod.Labels[sandboxPodLabelKey]; ok {
		return true
	}
	return pod.Labels[sandboxLegacyLabelKey] == sandboxLegacyLabelValue
}

// isActivePod reports whether a pod represents an occupied session. Running and
// Pending pods are active; terminated pods (Succeeded/Failed) are not.
func isActivePod(pod *corev1.Pod) bool {
	switch pod.Status.Phase {
	case corev1.PodRunning, corev1.PodPending:
		return true
	default:
		return false
	}
}

// CountActiveSandboxes returns the number of active sandbox pods in a gateway
// namespace. It is an observability signal used to surface the number of running
// sandboxes before a gateway is deleted; it is deliberately NOT a deletion gate,
// since deletion is best-effort and idempotent.
func CountActiveSandboxes(ctx context.Context, client kubernetes.Interface, namespace string) (int, error) {
	list, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("list pods in %s: %w", namespace, err)
	}
	count := 0
	for i := range list.Items {
		pod := &list.Items[i]
		if hasSandboxLabel(pod) && isActivePod(pod) {
			count++
		}
	}
	return count, nil
}

// classifyPod reduces a pod's phase and container statuses to a single
// human-readable state. CrashLoopBackOff is detected from container waiting
// reasons because a crash-looping pod's phase remains Running. Succeeded maps to
// Completed (e.g. the certgen job).
func classifyPod(pod *corev1.Pod) string {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason == PodStateCrashLoopBackOff {
			return PodStateCrashLoopBackOff
		}
	}
	switch pod.Status.Phase {
	case corev1.PodRunning:
		return PodStateRunning
	case corev1.PodSucceeded:
		return PodStateCompleted
	case corev1.PodPending:
		return PodStatePending
	case corev1.PodFailed:
		return PodStateFailed
	default:
		return PodStateUnknown
	}
}

// NamespacePodSummary is a per-state tally of the pods in a namespace, used to
// record why a namespace is being garbage collected.
type NamespacePodSummary struct {
	Total  int
	States map[string]int
}

// String renders the summary as a compact, stable descriptor such as
// "3 pods: 1 Running, 1 CrashLoopBackOff, 1 Completed" for events and logs.
func (s NamespacePodSummary) String() string {
	if s.Total == 0 {
		return "0 pods"
	}
	order := []string{
		PodStateRunning,
		PodStateCrashLoopBackOff,
		PodStatePending,
		PodStateCompleted,
		PodStateFailed,
		PodStateUnknown,
	}
	parts := make([]string, 0, len(order))
	for _, state := range order {
		if n := s.States[state]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, state))
		}
	}
	return fmt.Sprintf("%d pods: %s", s.Total, strings.Join(parts, ", "))
}

// SummarizeNamespacePods classifies every pod into a per-state tally. It is pure
// (no API calls) so the classification logic can be unit tested directly.
func SummarizeNamespacePods(pods []corev1.Pod) NamespacePodSummary {
	summary := NamespacePodSummary{States: make(map[string]int)}
	for i := range pods {
		summary.Total++
		summary.States[classifyPod(&pods[i])]++
	}
	return summary
}

// SummarizeNamespace lists all pods in a namespace and returns their state
// tally for garbage-collection observability.
func SummarizeNamespace(ctx context.Context, client kubernetes.Interface, namespace string) (NamespacePodSummary, error) {
	list, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return NamespacePodSummary{}, fmt.Errorf("list pods in %s: %w", namespace, err)
	}
	return SummarizeNamespacePods(list.Items), nil
}
