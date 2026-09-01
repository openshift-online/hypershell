package gateway

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const testNS = "openshell-test"

func getQuota(t *testing.T, cs *fake.Clientset) (*corev1.ResourceQuota, bool) {
	t.Helper()
	q, err := cs.CoreV1().ResourceQuotas(testNS).Get(context.Background(), GatewayResourceQuotaName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, false
	}
	if err != nil {
		t.Fatalf("get quota: %v", err)
	}
	return q, true
}

func getLimitRange(t *testing.T, cs *fake.Clientset) (*corev1.LimitRange, bool) {
	t.Helper()
	lr, err := cs.CoreV1().LimitRanges(testNS).Get(context.Background(), GatewayLimitRangeName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, false
	}
	if err != nil {
		t.Fatalf("get limitrange: %v", err)
	}
	return lr, true
}

// A nil config reconciles toward absence: both managed objects are deleted.
func TestReconcileNamespaceQuota_NilConfigDeletesBoth(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()

	cfg := &QuotaConfig{
		CPURequestTotal:            "2",
		MemoryLimitTotal:           "8Gi",
		ContainerCPURequestDefault: "100m",
		ContainerMemoryLimitMax:    "4Gi",
	}
	if err := ReconcileNamespaceQuota(ctx, cs, testNS, cfg); err != nil {
		t.Fatalf("reconcile create: %v", err)
	}
	if _, ok := getQuota(t, cs); !ok {
		t.Fatal("expected ResourceQuota to be created")
	}
	if _, ok := getLimitRange(t, cs); !ok {
		t.Fatal("expected LimitRange to be created")
	}

	if err := ReconcileNamespaceQuota(ctx, cs, testNS, nil); err != nil {
		t.Fatalf("reconcile to absent: %v", err)
	}
	if _, ok := getQuota(t, cs); ok {
		t.Fatal("expected ResourceQuota to be deleted when profile cleared")
	}
	if _, ok := getLimitRange(t, cs); ok {
		t.Fatal("expected LimitRange to be deleted when profile cleared")
	}
}

// Reassigning to a profile that only sets namespace-level totals drops the
// LimitRange but keeps the ResourceQuota.
func TestReconcileNamespaceQuota_ReassignDropsLimitRangeOnly(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()

	full := &QuotaConfig{
		CPURequestTotal:            "2",
		ContainerCPURequestDefault: "100m",
	}
	if err := ReconcileNamespaceQuota(ctx, cs, testNS, full); err != nil {
		t.Fatalf("reconcile create: %v", err)
	}
	if _, ok := getLimitRange(t, cs); !ok {
		t.Fatal("expected LimitRange to be created")
	}

	quotaOnly := &QuotaConfig{CPURequestTotal: "2"}
	if err := ReconcileNamespaceQuota(ctx, cs, testNS, quotaOnly); err != nil {
		t.Fatalf("reconcile reassign: %v", err)
	}
	if _, ok := getQuota(t, cs); !ok {
		t.Fatal("expected ResourceQuota to remain")
	}
	if _, ok := getLimitRange(t, cs); ok {
		t.Fatal("expected LimitRange to be removed when container fields drop")
	}
}

// Create writes the expected quantities, and an update-on-diverge rewrites them.
func TestReconcileNamespaceQuota_CreateWritesQuantitiesAndUpdatesOnDiverge(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()

	small := &QuotaConfig{CPURequestTotal: "2"}
	if err := ReconcileNamespaceQuota(ctx, cs, testNS, small); err != nil {
		t.Fatalf("reconcile create: %v", err)
	}
	q, ok := getQuota(t, cs)
	if !ok {
		t.Fatal("expected ResourceQuota to be created")
	}
	initialQty := q.Spec.Hard[corev1.ResourceRequestsCPU]
	if initialQty.String() != "2" {
		t.Fatalf("initial requests.cpu = %q, want %q", initialQty.String(), "2")
	}
	if !isManagedObject(q.Labels) {
		t.Fatalf("expected created ResourceQuota to carry the managed label, got %v", q.Labels)
	}

	large := &QuotaConfig{CPURequestTotal: "4"}
	if err := ReconcileNamespaceQuota(ctx, cs, testNS, large); err != nil {
		t.Fatalf("reconcile update: %v", err)
	}
	q, ok = getQuota(t, cs)
	if !ok {
		t.Fatal("expected ResourceQuota to still exist after update")
	}
	updatedQty := q.Spec.Hard[corev1.ResourceRequestsCPU]
	if updatedQty.String() != "4" {
		t.Fatalf("after reassign: requests.cpu = %q, want %q", updatedQty.String(), "4")
	}
}

// A second reconcile with an unchanged config must perform no writes; this
// guards the apiequality.Semantic.DeepEqual no-op behavior.
func TestReconcileNamespaceQuota_IdempotentWhenUnchanged(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()

	cfg := &QuotaConfig{CPURequestTotal: "2", ContainerCPURequestDefault: "100m"}
	if err := ReconcileNamespaceQuota(ctx, cs, testNS, cfg); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	actionsAfterFirst := len(cs.Actions())

	if err := ReconcileNamespaceQuota(ctx, cs, testNS, cfg); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}

	for _, a := range cs.Actions()[actionsAfterFirst:] {
		if a.GetVerb() == "create" || a.GetVerb() == "update" {
			t.Fatalf("second reconcile performed %q on %s, want no writes when spec is unchanged",
				a.GetVerb(), a.GetResource().Resource)
		}
	}
}

// An operator-owned object carrying the reserved name but WITHOUT the managed
// label must not be deleted or modified when the desired state is empty.
func TestReconcileNamespaceQuota_LeavesUnmanagedObjectsAlone(t *testing.T) {
	unmanagedRQ := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: GatewayResourceQuotaName, Namespace: testNS},
		Spec:       corev1.ResourceQuotaSpec{Hard: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("5")}},
	}
	unmanagedLR := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{Name: GatewayLimitRangeName, Namespace: testNS},
		Spec: corev1.LimitRangeSpec{Limits: []corev1.LimitRangeItem{{
			Type: corev1.LimitTypeContainer,
			Max:  corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
		}}},
	}
	cs := fake.NewSimpleClientset(unmanagedRQ, unmanagedLR)
	ctx := context.Background()

	if err := ReconcileNamespaceQuota(ctx, cs, testNS, nil); err != nil {
		t.Fatalf("reconcile to absent: %v", err)
	}
	if _, ok := getQuota(t, cs); !ok {
		t.Fatal("expected unmanaged ResourceQuota to be left intact")
	}
	if _, ok := getLimitRange(t, cs); !ok {
		t.Fatal("expected unmanaged LimitRange to be left intact")
	}
}
