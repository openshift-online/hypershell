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

// Labels createNamespace stamps on managed namespaces. Gateway namespace garbage
// collection sweeps managed namespaces whose names match the gateway prefix
// (openshell-<hex>) and excludes ManagedDatabase namespaces (openshell-db-<hex>).
const (
	ManagedByLabel    = "app.kubernetes.io/managed-by"
	ManagedByValue    = "hypershell-control-plane"
	ManagedLabel      = "hypershell.redhat.io/managed"
	ManagedLabelValue = "true"

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

// ManagedNamespaceSelector selects namespaces created by this control plane.
var ManagedNamespaceSelector = fmt.Sprintf("%s=%s,%s=%s",
	ManagedLabel, ManagedLabelValue, ManagedByLabel, ManagedByValue)

// IsManagedNamespace reports whether a namespace was created and is managed by
// this control plane, based on the labels createNamespace stamps.
func IsManagedNamespace(ns *corev1.Namespace) bool {
	return ns.Labels[ManagedLabel] == ManagedLabelValue &&
		ns.Labels[ManagedByLabel] == ManagedByValue
}

// IsGatewayNamespaceForGC reports whether ns is a gateway workload namespace
// subject to gateway namespace garbage collection. ManagedDatabase CNPG
// namespaces (openshell-db-*) carry the same management labels but are owned by
// the ManagedDatabase reconciler. Name-prefix matching keeps pre-existing orphaned
// gateway namespaces eligible for periodic GC without a label migration.
func IsGatewayNamespaceForGC(ns *corev1.Namespace) bool {
	if !IsManagedNamespace(ns) {
		return false
	}
	return strings.HasPrefix(ns.Name, GatewayNamespacePrefix) &&
		!strings.HasPrefix(ns.Name, DatabaseNamespacePrefix)
}

// DeleteManagedNamespace deletes a gateway namespace, best-effort and
// idempotent. It only deletes namespaces this control plane manages (see
// IsManagedNamespace): an unmanaged or already-absent namespace is treated as a
// no-op success. It returns deleted=true only when a delete call was issued.
func DeleteManagedNamespace(ctx context.Context, client kubernetes.Interface, namespace string) (bool, error) {
	ns, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Already gone: the delete is complete.
			return false, nil
		}
		return false, fmt.Errorf("get namespace %s: %w", namespace, err)
	}
	if !IsManagedNamespace(ns) {
		log.Printf("INFO namespace %s is not managed by hypershell-control-plane, skipping deletion", namespace)
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
