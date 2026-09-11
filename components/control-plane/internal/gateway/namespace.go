package gateway

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

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

	// GatewayNamespacePrefix and DatabaseNamespacePrefix mirror the namespace names
	// the API server assigns in its BeforeCreate hooks (gatewayNamespacePrefix in
	// components/api-server/plugins/gateways/model.go and dbNamespacePrefix in
	// components/api-server/plugins/managedDatabases/model.go). Both produce
	// "<prefix><16 hex chars>", and both namespace kinds carry the same management
	// labels, so GC cannot tell them apart by label alone and falls back to the name.
	//
	// The trailing dash in DatabaseNamespacePrefix is load-bearing: a gateway hash
	// may legitimately begin with the hex letters "db" (e.g. openshell-db1a2b...),
	// but never with "openshell-db-" because the character after "db" is always a
	// hex digit, never a dash. Keep these two constants in sync with the API server;
	// if that naming ever changes, gateway GC would silently start reaping (or
	// sparing) the wrong namespaces.
	GatewayNamespacePrefix  = "openshell-"
	DatabaseNamespacePrefix = "openshell-db-"

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

// isGatewayWorkloadName reports whether name is a gateway workload namespace
// (openshell-<hex>) rather than a ManagedDatabase namespace (openshell-db-<hex>).
func isGatewayWorkloadName(name string) bool {
	return strings.HasPrefix(name, GatewayNamespacePrefix) &&
		!strings.HasPrefix(name, DatabaseNamespacePrefix)
}

// IsGatewayNamespaceForGC reports whether ns is a gateway workload namespace
// this control-plane instance owns and that periodic garbage collection may
// reap. ManagedDatabase CNPG namespaces (openshell-db-*) carry the same
// management labels but are owned by the ManagedDatabase reconciler. Namespaces
// owned by a different instance, or lacking this instance's identity label, are
// never eligible: another HyperShell's live gateways would otherwise look
// orphaned because they are absent from this instance's API server.
func IsGatewayNamespaceForGC(ns *corev1.Namespace, instance string) bool {
	if !IsManagedNamespace(ns, instance) {
		return false
	}
	return isGatewayWorkloadName(ns.Name)
}

// DeleteManagedNamespace deletes a gateway namespace, best-effort and
// idempotent. It only deletes namespaces this instance is allowed to remove:
// unmanaged, non-gateway, already-absent, or foreign-instance namespaces are
// treated as a no-op success. A legacy namespace that carries the two management
// labels but no instance label MAY be deleted, because this path is keyed to a
// Gateway from this instance's API server rather than a cluster-wide sweep. It
// returns deleted=true only when a delete call was issued.
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
		log.Printf("INFO namespace %s is not a gateway workload namespace, skipping deletion", namespace)
		return false, nil
	}
	if labeled := ns.Labels[InstanceLabel]; labeled != "" && labeled != instance {
		log.Printf("INFO namespace %s is managed by instance %q, not %q; skipping deletion", namespace, labeled, instance)
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
	log.Printf("INFO deleted namespace %s", namespace)
	return true, nil
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
			log.Printf("INFO created namespace %s (instance=%s)", namespace, instance)
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

// BackfillInstanceLabel stamps this instance's identity label onto an existing
// managed gateway namespace that predates the label, so periodic GC -- which
// selects on hypershell.redhat.io/instance -- can observe it once its Gateway is
// deleted. It is the startup reclaim path for legacy namespaces this instance
// still owns per its API server, and is deliberately narrower than
// EnsureManagedNamespace so it is safe to run against a shared cluster:
//
//   - It never creates a namespace. An absent namespace is a no-op success; the
//     gateway reconciler creates it when the Gateway is next reconciled.
//   - It only touches namespaces that already carry BOTH management labels, i.e.
//     namespaces this control plane created. A namespace lacking them is left
//     untouched even if a Gateway records its name.
//   - It never overwrites a foreign instance label; a namespace already claimed
//     by another HyperShell is left untouched.
//
// The label is applied with a merge patch so a concurrent reconcile writing other
// labels is not clobbered. It returns labeled=true only when it stamped the label.
func BackfillInstanceLabel(ctx context.Context, client kubernetes.Interface, namespace, instance string) (bool, error) {
	if instance == "" {
		return false, fmt.Errorf("refusing to backfill namespace %s without a control-plane instance identity", namespace)
	}
	ns, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Nothing to reclaim; the reconciler creates and labels it when the
			// Gateway next provisions. Never create it here.
			return false, nil
		}
		return false, fmt.Errorf("get namespace %s: %w", namespace, err)
	}
	if !hasManagementLabels(ns) {
		// Not a namespace this control plane created. Refuse to claim it even
		// though a Gateway records its name, so a mis-recorded namespace field can
		// never adopt an unrelated namespace.
		log.Printf("INFO namespace %s lacks HyperShell management labels; not backfilling instance label", namespace)
		return false, nil
	}
	if existing := ns.Labels[InstanceLabel]; existing != "" {
		if existing != instance {
			log.Printf("INFO namespace %s is managed by instance %q, not %q; not backfilling instance label", namespace, existing, instance)
		}
		return false, nil
	}
	patch := fmt.Appendf(nil, `{"metadata":{"labels":{%q:%q}}}`, InstanceLabel, instance)
	if _, err := client.CoreV1().Namespaces().Patch(ctx, namespace, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return false, fmt.Errorf("backfill instance label on namespace %s: %w", namespace, err)
	}
	log.Printf("INFO backfilled instance label %s=%s onto legacy namespace %s", InstanceLabel, instance, namespace)
	return true, nil
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
		log.Printf("WARN namespace %s has unparseable %s=%q; overwriting", ns.Name, GCEligibleSinceAnnotation, existing)
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
