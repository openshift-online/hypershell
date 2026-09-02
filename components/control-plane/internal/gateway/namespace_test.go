package gateway

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func managedNamespace(name string, annotations map[string]string) *corev1.Namespace {
	return managedNamespaceForInstance(name, "hypershell", annotations)
}

func managedNamespaceForInstance(name, instance string, annotations map[string]string) *corev1.Namespace {
	labels := map[string]string{
		ManagedByLabel: ManagedByValue,
		ManagedLabel:   ManagedLabelValue,
	}
	if instance != "" {
		labels[InstanceLabel] = instance
	}
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
	}
}

func TestManagedNamespaceSelector(t *testing.T) {
	got := ManagedNamespaceSelector("alice")
	want := "hypershell.redhat.io/managed=true,app.kubernetes.io/managed-by=hypershell-control-plane,hypershell.redhat.io/instance=alice"
	if got != want {
		t.Errorf("ManagedNamespaceSelector() = %q, want %q", got, want)
	}
	if ManagedNamespaceSelector("") == ManagedNamespaceSelector("hypershell") {
		t.Errorf("empty instance selector must not match a real instance")
	}
}

func TestIsManagedNamespace(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		instance string
		want     bool
	}{
		{"this instance", map[string]string{ManagedByLabel: ManagedByValue, ManagedLabel: ManagedLabelValue, InstanceLabel: "hypershell"}, "hypershell", true},
		{"foreign instance", map[string]string{ManagedByLabel: ManagedByValue, ManagedLabel: ManagedLabelValue, InstanceLabel: "stage"}, "hypershell", false},
		{"missing instance label", map[string]string{ManagedByLabel: ManagedByValue, ManagedLabel: ManagedLabelValue}, "hypershell", false},
		{"empty instance identity", map[string]string{ManagedByLabel: ManagedByValue, ManagedLabel: ManagedLabelValue, InstanceLabel: "hypershell"}, "", false},
		{"missing managed-by", map[string]string{ManagedLabel: ManagedLabelValue, InstanceLabel: "hypershell"}, "hypershell", false},
		{"missing managed", map[string]string{ManagedByLabel: ManagedByValue, InstanceLabel: "hypershell"}, "hypershell", false},
		{"wrong managed-by value", map[string]string{ManagedByLabel: "someone-else", ManagedLabel: ManagedLabelValue, InstanceLabel: "hypershell"}, "hypershell", false},
		{"no labels", nil, "hypershell", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "x", Labels: tt.labels}}
			if got := IsManagedNamespace(ns, tt.instance); got != tt.want {
				t.Errorf("IsManagedNamespace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsGatewayNamespaceForGC(t *testing.T) {
	tests := []struct {
		name     string
		ns       string
		instance string
		labels   map[string]string
		want     bool
	}{
		{"this instance gateway", "openshell-a14873d1631f1b74", "hypershell", nil, true},
		{"e2e orphan", "openshell-e2e-orphan-123", "hypershell", nil, true},
		{"gateway hash starting with db", "openshell-db1a2b3c4d5e6f70", "hypershell", nil, true},
		{"managed database namespace", "openshell-db-a1b2c3d4e5f67890", "hypershell", nil, false},
		{"foreign instance", "openshell-a14873d1631f1b74", "hypershell", map[string]string{
			ManagedByLabel: ManagedByValue, ManagedLabel: ManagedLabelValue, InstanceLabel: "stage",
		}, false},
		{"unlabeled legacy", "openshell-a14873d1631f1b74", "hypershell", map[string]string{
			ManagedByLabel: ManagedByValue, ManagedLabel: ManagedLabelValue,
		}, false},
		{"unmanaged", "openshell-gw", "hypershell", map[string]string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := managedNamespace(tt.ns, nil)
			if tt.labels != nil {
				ns.Labels = tt.labels
			}
			if got := IsGatewayNamespaceForGC(ns, tt.instance); got != tt.want {
				t.Errorf("IsGatewayNamespaceForGC() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeleteManagedNamespace(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes a managed namespace", func(t *testing.T) {
		client := fake.NewSimpleClientset(managedNamespace("openshell-gw", nil))
		deleted, err := DeleteManagedNamespace(ctx, client, "openshell-gw", "hypershell")
		if err != nil {
			t.Fatalf("DeleteManagedNamespace() error = %v", err)
		}
		if !deleted {
			t.Errorf("deleted = false, want true")
		}
		if _, err := client.CoreV1().Namespaces().Get(ctx, "openshell-gw", metav1.GetOptions{}); !k8serrors.IsNotFound(err) {
			t.Errorf("namespace still exists after delete, err = %v", err)
		}
	})

	t.Run("skips an unmanaged namespace", func(t *testing.T) {
		unmanaged := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared"}}
		client := fake.NewSimpleClientset(unmanaged)
		deleted, err := DeleteManagedNamespace(ctx, client, "shared", "hypershell")
		if err != nil {
			t.Fatalf("DeleteManagedNamespace() error = %v", err)
		}
		if deleted {
			t.Errorf("deleted = true, want false for unmanaged namespace")
		}
		if _, err := client.CoreV1().Namespaces().Get(ctx, "shared", metav1.GetOptions{}); err != nil {
			t.Errorf("unmanaged namespace should be preserved, err = %v", err)
		}
	})

	t.Run("skips a managed database namespace", func(t *testing.T) {
		client := fake.NewSimpleClientset(managedNamespace("openshell-db-a1b2c3d4e5f67890", nil))
		deleted, err := DeleteManagedNamespace(ctx, client, "openshell-db-a1b2c3d4e5f67890", "hypershell")
		if err != nil {
			t.Fatalf("DeleteManagedNamespace() error = %v", err)
		}
		if deleted {
			t.Errorf("deleted = true, want false for ManagedDatabase namespace")
		}
		if _, err := client.CoreV1().Namespaces().Get(ctx, "openshell-db-a1b2c3d4e5f67890", metav1.GetOptions{}); err != nil {
			t.Errorf("ManagedDatabase namespace should be preserved, err = %v", err)
		}
	})

	t.Run("absent namespace is a no-op success", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		deleted, err := DeleteManagedNamespace(ctx, client, "gone", "hypershell")
		if err != nil {
			t.Fatalf("DeleteManagedNamespace() error = %v", err)
		}
		if deleted {
			t.Errorf("deleted = true, want false for absent namespace")
		}
	})

	t.Run("skips a foreign instance namespace", func(t *testing.T) {
		client := fake.NewSimpleClientset(managedNamespaceForInstance("openshell-gw", "stage", nil))
		deleted, err := DeleteManagedNamespace(ctx, client, "openshell-gw", "hypershell")
		if err != nil {
			t.Fatalf("DeleteManagedNamespace() error = %v", err)
		}
		if deleted {
			t.Errorf("deleted = true, want false for foreign instance")
		}
		if _, err := client.CoreV1().Namespaces().Get(ctx, "openshell-gw", metav1.GetOptions{}); err != nil {
			t.Errorf("foreign instance namespace should be preserved, err = %v", err)
		}
	})

	t.Run("deletes a legacy unlabeled managed gateway namespace", func(t *testing.T) {
		client := fake.NewSimpleClientset(managedNamespaceForInstance("openshell-gw", "", nil))
		deleted, err := DeleteManagedNamespace(ctx, client, "openshell-gw", "hypershell")
		if err != nil {
			t.Fatalf("DeleteManagedNamespace() error = %v", err)
		}
		if !deleted {
			t.Errorf("deleted = false, want true for unlabeled legacy namespace on delete-driven path")
		}
	})
}

func TestEnsureManagedNamespace(t *testing.T) {
	ctx := context.Background()

	t.Run("creates with instance label", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		if err := EnsureManagedNamespace(ctx, client, "openshell-gw", "hypershell"); err != nil {
			t.Fatalf("EnsureManagedNamespace() error = %v", err)
		}
		got, err := client.CoreV1().Namespaces().Get(ctx, "openshell-gw", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get namespace: %v", err)
		}
		if got.Labels[InstanceLabel] != "hypershell" {
			t.Errorf("instance label = %q, want hypershell", got.Labels[InstanceLabel])
		}
		if got.Labels[ManagedLabel] != ManagedLabelValue || got.Labels[ManagedByLabel] != ManagedByValue {
			t.Errorf("management labels missing: %v", got.Labels)
		}
	})

	t.Run("stamps instance on unlabeled legacy namespace", func(t *testing.T) {
		client := fake.NewSimpleClientset(managedNamespaceForInstance("openshell-gw", "", nil))
		if err := EnsureManagedNamespace(ctx, client, "openshell-gw", "hypershell"); err != nil {
			t.Fatalf("EnsureManagedNamespace() error = %v", err)
		}
		got, err := client.CoreV1().Namespaces().Get(ctx, "openshell-gw", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get namespace: %v", err)
		}
		if got.Labels[InstanceLabel] != "hypershell" {
			t.Errorf("instance label = %q, want hypershell after reconcile", got.Labels[InstanceLabel])
		}
	})

	t.Run("refuses a foreign instance namespace", func(t *testing.T) {
		client := fake.NewSimpleClientset(managedNamespaceForInstance("openshell-gw", "stage", nil))
		if err := EnsureManagedNamespace(ctx, client, "openshell-gw", "hypershell"); err == nil {
			t.Fatalf("EnsureManagedNamespace() error = nil, want foreign instance error")
		}
		got, err := client.CoreV1().Namespaces().Get(ctx, "openshell-gw", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get namespace: %v", err)
		}
		if got.Labels[InstanceLabel] != "stage" {
			t.Errorf("instance label overwritten to %q, want stage", got.Labels[InstanceLabel])
		}
	})

	t.Run("refuses an empty instance identity", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		if err := EnsureManagedNamespace(ctx, client, "openshell-gw", ""); err == nil {
			t.Fatalf("EnsureManagedNamespace() error = nil, want empty instance error")
		}
	})
}

func TestMarkGCEligible(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

	t.Run("stamps annotation when absent", func(t *testing.T) {
		ns := managedNamespace("openshell-gw", nil)
		client := fake.NewSimpleClientset(ns)

		got, err := MarkGCEligible(ctx, client, ns, now)
		if err != nil {
			t.Fatalf("MarkGCEligible() error = %v", err)
		}
		if !got.Equal(now) {
			t.Errorf("eligibleSince = %v, want %v", got, now)
		}

		updated, err := client.CoreV1().Namespaces().Get(ctx, "openshell-gw", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get namespace: %v", err)
		}
		if updated.Annotations[GCEligibleSinceAnnotation] != now.Format(time.RFC3339) {
			t.Errorf("annotation = %q, want %q", updated.Annotations[GCEligibleSinceAnnotation], now.Format(time.RFC3339))
		}
	})

	t.Run("returns existing timestamp without overwriting", func(t *testing.T) {
		earlier := now.Add(-30 * time.Minute)
		ns := managedNamespace("openshell-gw", map[string]string{
			GCEligibleSinceAnnotation: earlier.Format(time.RFC3339),
		})
		client := fake.NewSimpleClientset(ns)

		got, err := MarkGCEligible(ctx, client, ns, now)
		if err != nil {
			t.Fatalf("MarkGCEligible() error = %v", err)
		}
		if !got.Equal(earlier) {
			t.Errorf("eligibleSince = %v, want existing %v", got, earlier)
		}
	})

	t.Run("overwrites a corrupt timestamp", func(t *testing.T) {
		ns := managedNamespace("openshell-gw", map[string]string{
			GCEligibleSinceAnnotation: "not-a-timestamp",
		})
		client := fake.NewSimpleClientset(ns)

		got, err := MarkGCEligible(ctx, client, ns, now)
		if err != nil {
			t.Fatalf("MarkGCEligible() error = %v", err)
		}
		if !got.Equal(now) {
			t.Errorf("eligibleSince = %v, want %v after overwrite", got, now)
		}
	})
}

func TestClearGCEligible(t *testing.T) {
	ctx := context.Background()

	t.Run("removes annotation when present", func(t *testing.T) {
		ns := managedNamespace("openshell-gw", map[string]string{
			GCEligibleSinceAnnotation: time.Now().UTC().Format(time.RFC3339),
		})
		client := fake.NewSimpleClientset(ns)

		if err := ClearGCEligible(ctx, client, ns); err != nil {
			t.Fatalf("ClearGCEligible() error = %v", err)
		}

		updated, err := client.CoreV1().Namespaces().Get(ctx, "openshell-gw", metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get namespace: %v", err)
		}
		if _, ok := updated.Annotations[GCEligibleSinceAnnotation]; ok {
			t.Errorf("annotation still present after clear")
		}
	})

	t.Run("no-op when absent", func(t *testing.T) {
		ns := managedNamespace("openshell-gw", nil)
		client := fake.NewSimpleClientset(ns)
		if err := ClearGCEligible(ctx, client, ns); err != nil {
			t.Fatalf("ClearGCEligible() error = %v", err)
		}
	})
}
