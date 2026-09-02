package reconciler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openshift-online/hypershell/components/control-plane/internal/gateway"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func managedNS(name string, annotations map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				gateway.ManagedByLabel: gateway.ManagedByValue,
				gateway.ManagedLabel:   gateway.ManagedLabelValue,
				gateway.InstanceLabel:  "hypershell",
			},
			Annotations: annotations,
		},
	}
}

func nsWithLabels(name string, labels, annotations map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
	}
}

// newTestGC builds a NamespaceGCReconciler with a fixed clock and no gRPC
// connection. reconcileNamespace re-confirms liveness through the liveNamespaces
// seam immediately before a delete, so the seam is defaulted here to "nothing is
// live" - past-grace orphans are reaped - and overridden by the tests that
// exercise the stale-liveness protection.
func newTestGC(client kubernetes.Interface, now time.Time) *NamespaceGCReconciler {
	r := NewNamespaceGCReconciler(client, nil, time.Minute, 10*time.Minute, "hypershell")
	r.now = func() time.Time { return now }
	r.liveNamespaces = func(context.Context) (map[string]struct{}, error) {
		return map[string]struct{}{}, nil
	}
	return r
}

func nsExists(t *testing.T, client kubernetes.Interface, name string) bool {
	t.Helper()
	_, err := client.CoreV1().Namespaces().Get(context.Background(), name, metav1.GetOptions{})
	if err == nil {
		return true
	}
	if k8serrors.IsNotFound(err) {
		return false
	}
	t.Fatalf("get namespace %s: %v", name, err)
	return false
}

func TestReconcileNamespace_OrphanWithinGraceIsStampedNotDeleted(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	ns := managedNS("openshell-gw", nil)
	client := fake.NewSimpleClientset(ns)
	r := newTestGC(client, now)

	if err := r.reconcileNamespace(ctx, ns, map[string]struct{}{}); err != nil {
		t.Fatalf("reconcileNamespace() error = %v", err)
	}

	if !nsExists(t, client, "openshell-gw") {
		t.Fatalf("namespace deleted within grace period, want retained")
	}
	updated, err := client.CoreV1().Namespaces().Get(ctx, "openshell-gw", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if updated.Annotations[gateway.GCEligibleSinceAnnotation] != now.Format(time.RFC3339) {
		t.Errorf("gc-eligible-since = %q, want %q", updated.Annotations[gateway.GCEligibleSinceAnnotation], now.Format(time.RFC3339))
	}
}

func TestReconcileNamespace_OrphanPastGraceIsReaped(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	// Orphaned 20 minutes ago; grace is 10 minutes.
	ns := managedNS("openshell-gw", map[string]string{
		gateway.GCEligibleSinceAnnotation: now.Add(-20 * time.Minute).Format(time.RFC3339),
	})
	client := fake.NewSimpleClientset(ns)
	r := newTestGC(client, now)

	if err := r.reconcileNamespace(ctx, ns, map[string]struct{}{}); err != nil {
		t.Fatalf("reconcileNamespace() error = %v", err)
	}

	if nsExists(t, client, "openshell-gw") {
		t.Fatalf("namespace retained past grace period, want reaped")
	}

	// A GC event should have been recorded in the control-plane namespace.
	events, err := client.CoreV1().Events("hypershell").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events.Items) != 1 {
		t.Fatalf("event count = %d, want 1", len(events.Items))
	}
	if events.Items[0].Reason != "GarbageCollected" {
		t.Errorf("event reason = %q, want GarbageCollected", events.Items[0].Reason)
	}
	if events.Items[0].InvolvedObject.Name != "openshell-gw" {
		t.Errorf("event involved object name = %q, want openshell-gw", events.Items[0].InvolvedObject.Name)
	}
	if events.Items[0].InvolvedObject.Namespace != "hypershell" {
		t.Errorf("event involved object namespace = %q, want hypershell", events.Items[0].InvolvedObject.Namespace)
	}
}

func TestRecordGCEvent_InvolvedObjectNamespaceMatchesEventNamespace(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	client := fake.NewSimpleClientset()
	r := NewNamespaceGCReconciler(client, nil, time.Minute, 10*time.Minute, "hypershell-stage")
	r.now = func() time.Time { return now }

	if err := r.recordGCEvent(ctx, "openshell-gw", "test message"); err != nil {
		t.Fatalf("recordGCEvent() error = %v", err)
	}

	events, err := client.CoreV1().Events("hypershell-stage").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events.Items) != 1 {
		t.Fatalf("event count = %d, want 1", len(events.Items))
	}
	ev := events.Items[0]
	if ev.InvolvedObject.APIVersion != "v1" {
		t.Errorf("involvedObject.apiVersion = %q, want v1", ev.InvolvedObject.APIVersion)
	}
	if ev.Namespace != "hypershell-stage" {
		t.Errorf("event namespace = %q, want hypershell-stage", ev.Namespace)
	}
	if ev.InvolvedObject.Namespace != ev.Namespace {
		t.Errorf("involvedObject.namespace = %q, want %q", ev.InvolvedObject.Namespace, ev.Namespace)
	}
	if ev.InvolvedObject.Name != "openshell-gw" {
		t.Errorf("involvedObject.name = %q, want openshell-gw", ev.InvolvedObject.Name)
	}
}

func TestReconcileNamespace_SkipsManagedDatabaseNamespace(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	// ManagedDatabase namespaces share management labels but use the openshell-db- prefix.
	ns := managedNS("openshell-db-a1b2c3d4e5f67890", map[string]string{
		gateway.GCEligibleSinceAnnotation: now.Add(-20 * time.Minute).Format(time.RFC3339),
	})
	client := fake.NewSimpleClientset(ns)
	r := newTestGC(client, now)

	if err := r.reconcileNamespace(ctx, ns, map[string]struct{}{}); err != nil {
		t.Fatalf("reconcileNamespace() error = %v", err)
	}

	if !nsExists(t, client, "openshell-db-a1b2c3d4e5f67890") {
		t.Fatalf("ManagedDatabase namespace deleted by gateway GC, want retained")
	}
}

func TestReconcileNamespace_LiveGatewayClearsGraceAndRetains(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	// Stamped orphaned earlier, but the Gateway is live again this sweep.
	ns := managedNS("openshell-gw", map[string]string{
		gateway.GCEligibleSinceAnnotation: now.Add(-20 * time.Minute).Format(time.RFC3339),
	})
	client := fake.NewSimpleClientset(ns)
	r := newTestGC(client, now)

	live := map[string]struct{}{"openshell-gw": {}}
	if err := r.reconcileNamespace(ctx, ns, live); err != nil {
		t.Fatalf("reconcileNamespace() error = %v", err)
	}

	if !nsExists(t, client, "openshell-gw") {
		t.Fatalf("live gateway namespace deleted, want retained")
	}
	updated, err := client.CoreV1().Namespaces().Get(ctx, "openshell-gw", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if _, ok := updated.Annotations[gateway.GCEligibleSinceAnnotation]; ok {
		t.Errorf("gc-eligible-since annotation still present, want cleared")
	}
}

func TestReconcileNamespace_StaleLivenessRecheckSparesRevivedGateway(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	// Past grace by the sweep-start view, but a Gateway was (re)created for this
	// namespace after the sweep captured its live set.
	ns := managedNS("openshell-gw", map[string]string{
		gateway.GCEligibleSinceAnnotation: now.Add(-20 * time.Minute).Format(time.RFC3339),
	})
	client := fake.NewSimpleClientset(ns)
	r := newTestGC(client, now)
	// The fresh recheck, run immediately before the delete, observes the
	// namespace as live again.
	r.liveNamespaces = func(context.Context) (map[string]struct{}, error) {
		return map[string]struct{}{"openshell-gw": {}}, nil
	}

	// Pass the stale sweep-start view, which still shows the namespace orphaned.
	if err := r.reconcileNamespace(ctx, ns, map[string]struct{}{}); err != nil {
		t.Fatalf("reconcileNamespace() error = %v", err)
	}

	if !nsExists(t, client, "openshell-gw") {
		t.Fatalf("namespace reaped despite becoming live before delete, want retained")
	}
	// Nothing was reaped, so no GC event should have been recorded.
	events, err := client.CoreV1().Events("hypershell").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events.Items) != 0 {
		t.Fatalf("event count = %d, want 0 (no reap)", len(events.Items))
	}
	// The grace timer should be cleared now that the gateway is live again.
	updated, err := client.CoreV1().Namespaces().Get(ctx, "openshell-gw", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if _, ok := updated.Annotations[gateway.GCEligibleSinceAnnotation]; ok {
		t.Errorf("gc-eligible-since annotation still present, want cleared after revival")
	}
}

func TestReconcileNamespace_RecheckErrorDefersReap(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	ns := managedNS("openshell-gw", map[string]string{
		gateway.GCEligibleSinceAnnotation: now.Add(-20 * time.Minute).Format(time.RFC3339),
	})
	client := fake.NewSimpleClientset(ns)
	r := newTestGC(client, now)
	// The recheck cannot confirm liveness (e.g. the API server is unreachable).
	r.liveNamespaces = func(context.Context) (map[string]struct{}, error) {
		return nil, errors.New("api server unreachable")
	}

	if err := r.reconcileNamespace(ctx, ns, map[string]struct{}{}); err == nil {
		t.Fatalf("reconcileNamespace() error = nil, want error deferring the reap")
	}

	if !nsExists(t, client, "openshell-gw") {
		t.Fatalf("namespace reaped despite a failed liveness recheck, want retained")
	}
	// The reap was deferred, not performed, so no GC event should exist.
	events, err := client.CoreV1().Events("hypershell").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events.Items) != 0 {
		t.Fatalf("event count = %d, want 0 (reap deferred)", len(events.Items))
	}
}

func TestReconcileNamespace_UnmanagedIsUntouched(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	unmanaged := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared"}}
	client := fake.NewSimpleClientset(unmanaged)
	r := newTestGC(client, now)

	if err := r.reconcileNamespace(ctx, unmanaged, map[string]struct{}{}); err != nil {
		t.Fatalf("reconcileNamespace() error = %v", err)
	}

	if !nsExists(t, client, "shared") {
		t.Fatalf("unmanaged namespace deleted, want retained")
	}
	updated, err := client.CoreV1().Namespaces().Get(ctx, "shared", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if _, ok := updated.Annotations[gateway.GCEligibleSinceAnnotation]; ok {
		t.Errorf("unmanaged namespace was annotated, want untouched")
	}
}

func TestReconcileOnce_OnlySweepsThisInstance(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	own := managedNS("openshell-own", nil)
	foreign := nsWithLabels("openshell-stage", map[string]string{
		gateway.ManagedByLabel: gateway.ManagedByValue,
		gateway.ManagedLabel:   gateway.ManagedLabelValue,
		gateway.InstanceLabel:  "hypershell-stage",
	}, nil)
	unlabeled := nsWithLabels("openshell-legacy", map[string]string{
		gateway.ManagedByLabel: gateway.ManagedByValue,
		gateway.ManagedLabel:   gateway.ManagedLabelValue,
	}, nil)
	client := fake.NewSimpleClientset(own, foreign, unlabeled)
	r := newTestGC(client, now)

	r.reconcileOnce(ctx)

	ownUpdated, err := client.CoreV1().Namespaces().Get(ctx, "openshell-own", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get own namespace: %v", err)
	}
	if ownUpdated.Annotations[gateway.GCEligibleSinceAnnotation] != now.Format(time.RFC3339) {
		t.Errorf("this instance's orphan was not stamped gc-eligible-since")
	}

	for _, name := range []string{"openshell-stage", "openshell-legacy"} {
		got, err := client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get %s: %v", name, err)
		}
		if _, ok := got.Annotations[gateway.GCEligibleSinceAnnotation]; ok {
			t.Errorf("%s was treated as an orphan of this instance, want ignored", name)
		}
	}
}

func TestReconcileOnce_DoesNotReapForeignInstancePastGrace(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	foreign := nsWithLabels("openshell-stage", map[string]string{
		gateway.ManagedByLabel: gateway.ManagedByValue,
		gateway.ManagedLabel:   gateway.ManagedLabelValue,
		gateway.InstanceLabel:  "hypershell-stage",
	}, map[string]string{
		gateway.GCEligibleSinceAnnotation: now.Add(-20 * time.Minute).Format(time.RFC3339),
	})
	client := fake.NewSimpleClientset(foreign)
	r := newTestGC(client, now)

	r.reconcileOnce(ctx)

	if !nsExists(t, client, "openshell-stage") {
		t.Fatalf("foreign instance namespace reaped, want retained")
	}
}

func TestReconcileNamespace_SkipsForeignInstance(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	foreign := nsWithLabels("openshell-stage", map[string]string{
		gateway.ManagedByLabel: gateway.ManagedByValue,
		gateway.ManagedLabel:   gateway.ManagedLabelValue,
		gateway.InstanceLabel:  "hypershell-stage",
	}, nil)
	client := fake.NewSimpleClientset(foreign)
	r := newTestGC(client, now)

	if err := r.reconcileNamespace(ctx, foreign, map[string]struct{}{}); err != nil {
		t.Fatalf("reconcileNamespace() error = %v", err)
	}

	updated, err := client.CoreV1().Namespaces().Get(ctx, "openshell-stage", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if _, ok := updated.Annotations[gateway.GCEligibleSinceAnnotation]; ok {
		t.Errorf("foreign instance namespace was annotated, want untouched")
	}
}

func TestReconcileOnce_EmptyInstanceAbortsSweep(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	ns := managedNS("openshell-gw", nil)
	client := fake.NewSimpleClientset(ns)
	r := newTestGC(client, now)
	r.cpNamespace = ""

	r.reconcileOnce(ctx)

	updated, err := client.CoreV1().Namespaces().Get(ctx, "openshell-gw", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	if _, ok := updated.Annotations[gateway.GCEligibleSinceAnnotation]; ok {
		t.Errorf("sweep with empty instance annotated a namespace, want aborted")
	}
}
