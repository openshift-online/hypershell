package gateway

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SandboxGVR is the upstream agent-sandbox Sandbox custom resource.
var SandboxGVR = schema.GroupVersionResource{
	Group:    "agents.x-k8s.io",
	Version:  "v1beta1",
	Resource: "sandboxes",
}

// SandboxLookup indexes Sandbox CRs by namespace/name for correlating pods.
type SandboxLookup map[string]*unstructured.Unstructured

func sandboxKey(namespace, name string) string {
	return namespace + "/" + name
}

// IndexSandboxes builds a namespace/name lookup from unstructured Sandbox objects.
func IndexSandboxes(sandboxes []*unstructured.Unstructured) SandboxLookup {
	out := make(SandboxLookup, len(sandboxes))
	for _, sb := range sandboxes {
		if sb == nil {
			continue
		}
		out[sandboxKey(sb.GetNamespace(), sb.GetName())] = sb
	}
	return out
}

// FindSandboxForPod locates the Sandbox CR for a sandbox pod via OwnerReference
// (preferred) or a same-name fallback when the owner ref is absent.
func FindSandboxForPod(pod *corev1.Pod, sandboxes SandboxLookup) *unstructured.Unstructured {
	if pod == nil {
		return nil
	}
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "Sandbox" && ref.Name != "" {
			if sb := sandboxes[sandboxKey(pod.Namespace, ref.Name)]; sb != nil {
				return sb
			}
		}
	}
	return sandboxes[sandboxKey(pod.Namespace, pod.Name)]
}

// ObservationFromPod builds a SandboxObservation from an active-candidate pod and
// an optional matching Sandbox CR. Non-sandbox or inactive pods still produce an
// observation with Active=false so callers can filter uniformly.
func ObservationFromPod(pod *corev1.Pod, sandbox *unstructured.Unstructured) SandboxObservation {
	obs := SandboxObservation{
		Namespace:    pod.Namespace,
		Active:       IsActiveSandboxPod(pod),
		CreationTime: pod.CreationTimestamp.Time,
	}
	if sandbox != nil {
		if created := sandbox.GetCreationTimestamp(); !created.IsZero() {
			obs.CreationTime = created.Time
		}
		obs.ShutdownTime = SandboxShutdownTime(sandbox)
		obs.LastActivityTime = SandboxLastActivityTime(sandbox)
	}
	return obs
}

// SandboxShutdownTime reads spec.lifecycle.shutdownTime from a Sandbox object.
func SandboxShutdownTime(sandbox *unstructured.Unstructured) *time.Time {
	return nestedTime(sandbox, "spec", "lifecycle", "shutdownTime")
}

// SandboxLastActivityTime reads status.lastActivityTime from a Sandbox object.
func SandboxLastActivityTime(sandbox *unstructured.Unstructured) *time.Time {
	return nestedTime(sandbox, "status", "lastActivityTime")
}

func nestedTime(obj *unstructured.Unstructured, fields ...string) *time.Time {
	if obj == nil {
		return nil
	}
	raw, found, err := unstructured.NestedString(obj.Object, fields...)
	if err != nil || !found || raw == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		// Kubernetes also emits RFC3339Nano for metav1.Time.
		parsed, err = time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return nil
		}
	}
	return &parsed
}

// BuildAttentionObservations correlates sandbox pods with Sandbox CRs. Only pods
// in eligibleNamespaces are included. Pods without a matching Sandbox still
// contribute using pod metadata alone.
func BuildAttentionObservations(
	pods []*corev1.Pod,
	sandboxes SandboxLookup,
	eligibleNamespaces map[string]struct{},
) []SandboxObservation {
	out := make([]SandboxObservation, 0, len(pods))
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		if _, ok := eligibleNamespaces[pod.Namespace]; !ok {
			continue
		}
		if !hasSandboxLabel(pod) {
			continue
		}
		sb := FindSandboxForPod(pod, sandboxes)
		out = append(out, ObservationFromPod(pod, sb))
	}
	return out
}

// FormatAttentionCounts returns a compact log-friendly summary.
func FormatAttentionCounts(c AttentionCounts) string {
	return fmt.Sprintf("orphaned=%d expiring=%d idle=%d", c.Orphaned, c.Expiring, c.Idle)
}
