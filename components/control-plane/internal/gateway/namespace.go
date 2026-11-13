package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/openshift-online/hypershell/components/control-plane/internal/helm"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// OpenShellGatewayIDLabel is the label the OpenShell Kubernetes driver stamps on
// the per-workspace namespaces it creates in managed workspace mode, carrying
// the gateway's configured gateway_id.
const OpenShellGatewayIDLabel = "openshell.ai/gateway-id"

// Labels EnsureManagedNamespace stamps on namespaces this control-plane instance
// owns. Periodic GC selects on the instance label so two HyperShells on one
// cluster never treat each other's gateway namespaces as orphans.
const (
	ManagedByLabel    = "app.kubernetes.io/managed-by"
	ManagedByValue    = "hypershell-control-plane"
	ManagedLabel      = "hypershell.redhat.io/managed"
	ManagedLabelValue = "true"
	// InstanceLabel identifies which control plane created the namespace. The
	// value is unique to that controller: the namespace the controller pod
	// runs in (HYPERSHELL_NAMESPACE, populated from the downward API). Periodic
	// GC selects on this label so two HyperShells on one cluster never treat
	// each other's gateways as orphans.
	InstanceLabel = "hypershell.redhat.io/instance"

	// ControlPlaneLabel marks the namespace a HyperShell control plane runs in. The
	// per-cluster provider Vault binds its provisioner role (openshell-gateway-provisioner)
	// with a namespace selector on this label, so ONLY a control-plane namespace may log in
	// to create/delete per-tenant mounts (HYPERSHELL-369). The control plane self-applies it
	// at startup; a namespace-scoped attacker cannot forge it, because labeling a namespace
	// needs cluster-scoped patch on the namespaces resource.
	ControlPlaneLabel      = "hypershell.redhat.io/control-plane"
	ControlPlaneLabelValue = "true"

	// GatewayNamespacePrefix mirrors the namespace name the API server assigns in
	// its BeforeCreate hook (gatewayNamespacePrefix in
	// components/api-server/plugins/gateways/model.go), producing
	// "openshell-<16 hex chars>". Keep it in sync with the API server; if that
	// naming ever changes, gateway GC would silently start reaping (or sparing)
	// the wrong namespaces.
	GatewayNamespacePrefix = "openshell-"

	// GCEligibleSinceAnnotation records, in RFC3339, when a managed namespace was
	// first observed orphaned (no live Gateway). The grace period is measured
	// from this timestamp so it survives control-plane restarts.
	GCEligibleSinceAnnotation = "hypershell.redhat.io/gc-eligible-since"
)

// ManagedNamespaceSelector selects namespaces created by this control-plane
// instance. An empty instance is an error: listing by the generic management
// labels alone would treat every other HyperShell on the cluster as an orphan.
// The returned selector is still a valid Kubernetes label selector that matches
// nothing, so a caller that ignores the error cannot trigger an HTTP 400 or
// list every namespace (an empty LabelSelector would).
func ManagedNamespaceSelector(instance string) (string, error) {
	if instance == "" {
		return InstanceLabel + "=no-such-instance", fmt.Errorf("instance identity is empty; refusing to build a managed-namespace selector")
	}
	return fmt.Sprintf("%s=%s,%s=%s,%s=%s",
		ManagedLabel, ManagedLabelValue, ManagedByLabel, ManagedByValue, InstanceLabel, instance), nil
}

// ManagedNamespaceLabels is the label set stamped on namespaces this instance
// creates and reconciles.
func ManagedNamespaceLabels(instance string) map[string]string {
	return map[string]string{
		ManagedByLabel: ManagedByValue,
		ManagedLabel:   ManagedLabelValue,
		InstanceLabel:  instance,
	}
}

// hasManagementLabels reports whether ns carries the two generic HyperShell
// management labels, regardless of which instance created it.
func hasManagementLabels(ns *corev1.Namespace) bool {
	if ns == nil {
		return false
	}
	return ns.Labels[ManagedLabel] == ManagedLabelValue &&
		ns.Labels[ManagedByLabel] == ManagedByValue
}

// IsManagedNamespace reports whether a namespace was created and is managed by
// this control-plane instance, based on the labels EnsureManagedNamespace stamps.
func IsManagedNamespace(ns *corev1.Namespace, instance string) bool {
	if instance == "" || !hasManagementLabels(ns) {
		return false
	}
	return ns.Labels[InstanceLabel] == instance
}

// EnsureControlPlaneNamespaceLabel idempotently stamps ControlPlaneLabel on the control
// plane's OWN namespace, so the per-cluster provider Vault's provisioner-role namespace
// selector admits it (HYPERSHELL-369). It is best-effort at startup: on failure the caller
// logs a warning, and the fail-closed mount provisioning surfaces a still-missing label as a
// clear gateway-provisioning error rather than a silent security gap.
func EnsureControlPlaneNamespaceLabel(ctx context.Context, client kubernetes.Interface, namespace string) error {
	if namespace == "" {
		return fmt.Errorf("control-plane namespace is empty; cannot apply the %s label", ControlPlaneLabel)
	}
	ns, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get control-plane namespace %s: %w", namespace, err)
	}
	if ns.Labels[ControlPlaneLabel] == ControlPlaneLabelValue {
		return nil // already labeled
	}
	patch := []byte(fmt.Sprintf(`{"metadata":{"labels":{%q:%q}}}`, ControlPlaneLabel, ControlPlaneLabelValue))
	if _, err := client.CoreV1().Namespaces().Patch(ctx, namespace, types.StrategicMergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("label control-plane namespace %s with %s: %w", namespace, ControlPlaneLabel, err)
	}
	slog.InfoContext(ctx, "Labeled control-plane namespace", "namespace", namespace, "label", ControlPlaneLabel, "value", ControlPlaneLabelValue)
	return nil
}

// isGatewayWorkloadName reports whether name is a gateway workload namespace
// (openshell-<hex>).
func isGatewayWorkloadName(name string) bool {
	return strings.HasPrefix(name, GatewayNamespacePrefix)
}

// IsGatewayNamespaceForGC reports whether ns is a gateway workload namespace
// this control-plane instance owns and that periodic garbage collection may
// reap. Namespaces owned by a different instance, or lacking this instance's
// identity label, are never eligible: another HyperShell's live gateways would
// otherwise look orphaned because they are absent from this instance's API
// server.
func IsGatewayNamespaceForGC(ns *corev1.Namespace, instance string) bool {
	if !IsManagedNamespace(ns, instance) {
		return false
	}
	return isGatewayWorkloadName(ns.Name)
}

// DeleteManagedNamespace deletes a gateway namespace, best-effort and
// idempotent. It only deletes namespaces this instance is allowed to remove:
// unmanaged, non-gateway, already-absent, or foreign-instance namespaces are
// treated as a no-op success. A namespace without this instance's label is
// foreign and is never deleted. It returns deleted=true only when a delete call
// was issued.
func DeleteManagedNamespace(ctx context.Context, client kubernetes.Interface, namespace, instance string) (bool, error) {
	ns, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Already gone: the delete is complete.
			return false, nil
		}
		return false, fmt.Errorf("get namespace %s: %w", namespace, err)
	}
	if !isGatewayWorkloadName(ns.Name) || !hasManagementLabels(ns) {
		slog.InfoContext(ctx, "Namespace is not a gateway workload namespace, skipping deletion", "namespace", namespace)
		return false, nil
	}
	if labeled := ns.Labels[InstanceLabel]; instance == "" || labeled != instance {
		slog.InfoContext(ctx, "Namespace is managed by another instance; skipping deletion", "namespace", namespace, "labeled_instance", labeled, "instance", instance)
		return false, nil
	}
	if ns.DeletionTimestamp != nil {
		// Already terminating; nothing more to do.
		return false, nil
	}
	if err := client.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{}); err != nil {
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("delete namespace %s: %w", namespace, err)
	}
	slog.InfoContext(ctx, "Deleted namespace", "namespace", namespace)
	return true, nil
}

// DeleteManagedWorkspaceNamespaces deletes the per-workspace namespaces a
// managed-mode gateway auto-created (openshell-{gateway_id}-{workspace}),
// best-effort and idempotent. The OpenShell Kubernetes driver labels those
// namespaces with its gateway id; HyperShell provisions managed gateways with
// gateway_id derived from the gateway's API-assigned namespace, so they are
// selectable here. The namespaces are cluster-scoped and created by the gateway
// process rather than the control plane, so neither the Helm uninstall nor the
// managed-namespace GC reaps them; without this sweep every deleted managed
// gateway would leak its workspace namespaces. A namespace that does not follow
// the API-assigned openshell-{16-hex} shape cannot have created conventional
// managed namespaces, so it is a no-op. Per-namespace failures are aggregated so
// the caller can record orphans and retry the delete reconcile.
func DeleteManagedWorkspaceNamespaces(ctx context.Context, client kubernetes.Interface, namespace string) error {
	gatewayID, err := helm.ManagedGatewayID(namespace)
	if err != nil {
		return nil
	}
	selector := fmt.Sprintf("%s=%s", OpenShellGatewayIDLabel, gatewayID)
	list, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list managed workspace namespaces of %s: %w", namespace, err)
	}
	var errs []error
	for i := range list.Items {
		name := list.Items[i].Name
		if name == namespace {
			// Defensive: the gateway namespace itself never carries the label.
			continue
		}
		if err := client.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("delete managed workspace namespace %s: %w", name, err))
		} else {
			slog.InfoContext(ctx, "Deleted managed workspace namespace", "name", name, "namespace", namespace)
		}
	}
	return errors.Join(errs...)
}

// DeleteOperatorWorkspaceNamespaces deletes the tenant workspace namespaces an
// operator-mode gateway's external operator created for it, best-effort and
// idempotent. In operator mode the external operator (not the control plane)
// creates and labels these namespaces with the administrator-approved label key
// set to the owning gateway's own namespace name, so they are selected by
// labelKey=gatewayNamespace (OSG-OPWS-02). Because that value is unique per
// gateway (PLAT-DATA-03), the sweep only ever matches this gateway's namespaces.
// They live outside the gateway's own namespace, so neither the Helm uninstall
// nor the managed-namespace GC reaps them; without this sweep every deleted
// operator gateway would leak its tenant namespaces (OSG-OPWS-06). An empty
// labelKey or gatewayNamespace selects nothing (no-op), so a control plane with
// no operator label configured never errors on delete. An already-absent or
// already-terminating namespace is success, and the gateway's own namespace is
// skipped defensively. Per-namespace failures are aggregated so the caller can
// record orphans and retry the delete reconcile.
func DeleteOperatorWorkspaceNamespaces(ctx context.Context, client kubernetes.Interface, labelKey, gatewayNamespace string) error {
	if labelKey == "" || gatewayNamespace == "" {
		return nil
	}
	selector := fmt.Sprintf("%s=%s", labelKey, gatewayNamespace)
	list, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return fmt.Errorf("list operator workspace namespaces of %s: %w", gatewayNamespace, err)
	}
	var errs []error
	for i := range list.Items {
		name := list.Items[i].Name
		if name == gatewayNamespace {
			// Defensive: never delete the gateway's own namespace here; it is
			// reclaimed by the managed-namespace delete path.
			continue
		}
		if list.Items[i].DeletionTimestamp != nil {
			// Already terminating; treat as success.
			continue
		}
		if err := client.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
			if k8serrors.IsNotFound(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("delete operator workspace namespace %s: %w", name, err))
		} else {
			slog.InfoContext(ctx, "Deleted operator workspace namespace", "name", name, "gateway_namespace", gatewayNamespace)
		}
	}
	return errors.Join(errs...)
}

// EnsureManagedNamespace creates namespace if it is absent, and otherwise
// reconciles the management and instance labels onto it. It refuses to adopt a
// namespace already labeled as a different control-plane instance. An empty
// instance is a configuration error: unlabeled namespaces must not be claimed
// without an identity.
func EnsureManagedNamespace(ctx context.Context, client kubernetes.Interface, namespace, instance string) error {
	if instance == "" {
		return fmt.Errorf("refusing to manage namespace %s without a control-plane instance identity", namespace)
	}
	desired := ManagedNamespaceLabels(instance)

	ns, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		created := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace, Labels: desired},
		}
		_, err = client.CoreV1().Namespaces().Create(ctx, created, metav1.CreateOptions{})
		if err == nil {
			slog.InfoContext(ctx, "Created namespace", "namespace", namespace, "instance", instance)
			return nil
		}
		if !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("create namespace %s: %w", namespace, err)
		}
		ns, err = client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get namespace %s after create race: %w", namespace, err)
		}
	} else if err != nil {
		return fmt.Errorf("get namespace %s: %w", namespace, err)
	}

	if existing := ns.Labels[InstanceLabel]; existing != "" && existing != instance {
		return fmt.Errorf("namespace %s is managed by instance %q, not %q", namespace, existing, instance)
	}

	updated := ns.DeepCopy()
	if updated.Labels == nil {
		updated.Labels = map[string]string{}
	}
	changed := false
	for k, v := range desired {
		if updated.Labels[k] != v {
			updated.Labels[k] = v
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if _, err := client.CoreV1().Namespaces().Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update namespace %s labels: %w", namespace, err)
	}
	return nil
}

// MarkGCEligible stamps the gc-eligible-since annotation with the given time if
// it is not already present, returning the effective eligible-since time. The
// timestamp is persisted on the namespace so the grace period survives
// control-plane restarts. A corrupt existing value is overwritten.
func MarkGCEligible(ctx context.Context, client kubernetes.Interface, ns *corev1.Namespace, now time.Time) (time.Time, error) {
	if existing := ns.Annotations[GCEligibleSinceAnnotation]; existing != "" {
		if t, err := time.Parse(time.RFC3339, existing); err == nil {
			return t, nil
		}
		slog.WarnContext(ctx, "Namespace has unparseable GC-eligible annotation; overwriting", "namespace", ns.Name, "annotation", GCEligibleSinceAnnotation, "value", existing)
	}

	stamp := now.UTC().Format(time.RFC3339)
	patch := fmt.Appendf(nil, `{"metadata":{"annotations":{%q:%q}}}`, GCEligibleSinceAnnotation, stamp)
	if _, err := client.CoreV1().Namespaces().Patch(ctx, ns.Name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return time.Time{}, fmt.Errorf("annotate namespace %s %s: %w", ns.Name, GCEligibleSinceAnnotation, err)
	}
	// Parse back the persisted value so the returned time matches exactly what a
	// later sweep will read from the annotation (RFC3339 drops sub-second parts).
	t, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse stamped %s for %s: %w", GCEligibleSinceAnnotation, ns.Name, err)
	}
	return t, nil
}

// ClearGCEligible removes the gc-eligible-since annotation if present, used when
// a namespace's Gateway reappears (the namespace is no longer orphaned).
func ClearGCEligible(ctx context.Context, client kubernetes.Interface, ns *corev1.Namespace) error {
	if _, ok := ns.Annotations[GCEligibleSinceAnnotation]; !ok {
		return nil
	}
	// JSON merge patch: setting the key to null removes the annotation.
	patch := fmt.Appendf(nil, `{"metadata":{"annotations":{%q:null}}}`, GCEligibleSinceAnnotation)
	if _, err := client.CoreV1().Namespaces().Patch(ctx, ns.Name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("clear namespace %s %s: %w", ns.Name, GCEligibleSinceAnnotation, err)
	}
	return nil
}
