package policyinterceptor

import (
	"context"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/openshift-online/hypershell/components/control-plane/internal/policybundle"
)

// BundleWatcher watches the one policy bundle ConfigMap in the gateway
// namespace through the Kubernetes API and applies each new revision to the
// Store without a restart (PLAT-PGP-12). The informer list/watch is restricted
// to the ConfigMap by a metadata.name field selector, so the sidecar needs only
// get, list, and watch on that one ConfigMap (resourceNames-scoped RBAC works).
type BundleWatcher struct {
	Client    kubernetes.Interface
	Namespace string
	Store     *Store
	Metrics   *Metrics
	Log       *slog.Logger
	// OnApplied is called after a new revision is applied (optional); the
	// reporter uses it to send a heartbeat with the new applied revision.
	OnApplied func(revision string)
}

func (w *BundleWatcher) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.Default()
}

// Run watches until ctx is done. The informer re-lists and re-watches on its
// own after API server disconnects.
func (w *BundleWatcher) Run(ctx context.Context) error {
	factory := informers.NewSharedInformerFactoryWithOptions(w.Client, 0,
		informers.WithNamespace(w.Namespace),
		informers.WithTweakListOptions(func(o *metav1.ListOptions) {
			o.FieldSelector = fields.OneTermEqualSelector("metadata.name", policybundle.ConfigMapName).String()
		}),
	)
	informer := factory.Core().V1().ConfigMaps().Informer()
	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj any) { w.handle(ctx, obj) },
		UpdateFunc: func(_, obj any) { w.handle(ctx, obj) },
		DeleteFunc: func(obj any) { w.handleDelete(ctx, obj) },
	}); err != nil {
		return err
	}
	factory.Start(ctx.Done())
	<-ctx.Done()
	factory.Shutdown()
	return nil
}

func (w *BundleWatcher) handle(ctx context.Context, obj any) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok || cm.Name != policybundle.ConfigMapName {
		return
	}
	w.Apply(ctx, cm)
}

// Apply loads the bundle from one ConfigMap.
func (w *BundleWatcher) Apply(ctx context.Context, cm *corev1.ConfigMap) {
	raw, ok := cm.Data[policybundle.DataKey]
	if !ok {
		w.reject(ctx, cm, "policy bundle ConfigMap has no "+policybundle.DataKey+" key")
		return
	}
	changed, err := w.Store.Apply([]byte(raw))
	if err != nil {
		w.reject(ctx, cm, err.Error())
		return
	}
	if changed {
		rev := w.Store.AppliedRevision()
		snap := w.Store.Current()
		w.log().InfoContext(ctx, "applied policy bundle", "revision", rev,
			"enforced", len(snap.bundle.Enforced), "audit_only", len(snap.bundle.AuditOnly),
			"resource_policies_supported", snap.bundle.ResourcePoliciesSupported)
		if w.OnApplied != nil {
			w.OnApplied(rev)
		}
	}
}

func (w *BundleWatcher) reject(ctx context.Context, cm *corev1.ConfigMap, reason string) {
	if w.Metrics != nil {
		w.Metrics.BundleRejections.Inc()
	}
	w.log().ErrorContext(ctx, "rejected policy bundle; keeping the previously applied revision",
		"configmap", cm.Namespace+"/"+cm.Name, "resource_version", cm.ResourceVersion,
		"applied_revision", w.Store.AppliedRevision(), "reason", reason)
}

// handleDelete keeps the applied revision: the controller always writes a
// bundle for a gateway with the interceptor, so a deletion is transient (for
// example a manual delete before the controller recreates it), and dropping
// the policies would either fail every protected RPC closed or silently stop
// enforcing.
func (w *BundleWatcher) handleDelete(ctx context.Context, obj any) {
	if tomb, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tomb.Obj
	}
	if cm, ok := obj.(*corev1.ConfigMap); ok && cm.Name == policybundle.ConfigMapName {
		w.log().WarnContext(ctx, "policy bundle ConfigMap deleted; keeping the applied revision until it is recreated",
			"applied_revision", w.Store.AppliedRevision())
	}
}
