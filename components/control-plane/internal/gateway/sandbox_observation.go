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
		Name:         pod.Name,
		Active:       IsActiveSandboxPod(pod),
		CreationTime: pod.CreationTimestamp.Time,
	}
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "Sandbox" && ref.Name != "" {
			obs.Name = ref.Name
			break
		}
	}
	if sandbox != nil {
		if name := sandbox.GetName(); name != "" {
			obs.Name = name
		}
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

// ProvisionedSandboxCounts returns the number of Sandbox custom resources in
// each eligible namespace, including namespaces with zero sandboxes.
func ProvisionedSandboxCounts(
	sandboxes []*unstructured.Unstructured,
	eligibleNamespaces map[string]struct{},
) map[string]int {
	counts := make(map[string]int, len(eligibleNamespaces))
	for namespace := range eligibleNamespaces {
		counts[namespace] = 0
	}
	for _, sandbox := range sandboxes {
		if sandbox == nil {
			continue
		}
		namespace := sandbox.GetNamespace()
		if _, eligible := eligibleNamespaces[namespace]; eligible {
			counts[namespace]++
		}
	}
	return counts
}

// ReconcileActiveSandboxes returns the current active sandbox set and the
// sandboxes that left it since the previous observation. A nil previous set is
// the initial baseline and produces no departures.
func ReconcileActiveSandboxes(
	previous map[string]SandboxObservation,
	observations []SandboxObservation,
) (map[string]SandboxObservation, []SandboxObservation) {
	current := make(map[string]SandboxObservation, len(observations))
	for _, obs := range observations {
		if !obs.Active || obs.Name == "" {
			continue
		}
		current[sandboxKey(obs.Namespace, obs.Name)] = obs
	}
	if previous == nil {
		return current, nil
	}

	var departed []SandboxObservation
	for key, obs := range previous {
		if _, active := current[key]; !active {
			departed = append(departed, obs)
		}
	}
	return current, departed
}

// SandboxLifetime returns the observed lifetime of a sandbox. It reports false
// when the creation time is missing or in the future.
func SandboxLifetime(obs SandboxObservation, now time.Time) (time.Duration, bool) {
	if obs.CreationTime.IsZero() || obs.CreationTime.After(now) {
		return 0, false
	}
	return now.Sub(obs.CreationTime), true
}

// FormatAttentionCounts returns a compact log-friendly summary.
func FormatAttentionCounts(c AttentionCounts) string {
	return fmt.Sprintf("orphaned=%d expiring=%d idle=%d", c.Orphaned, c.Expiring, c.Idle)
}
