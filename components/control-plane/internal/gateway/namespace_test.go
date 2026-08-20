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
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				ManagedByLabel: ManagedByValue,
				ManagedLabel:   ManagedLabelValue,
			},
			Annotations: annotations,
		},
	}
}

func TestIsManagedNamespace(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{"both labels", map[string]string{ManagedByLabel: ManagedByValue, ManagedLabel: ManagedLabelValue}, true},
		{"missing managed-by", map[string]string{ManagedLabel: ManagedLabelValue}, false},
		{"missing managed", map[string]string{ManagedByLabel: ManagedByValue}, false},
		{"wrong managed-by value", map[string]string{ManagedByLabel: "someone-else", ManagedLabel: ManagedLabelValue}, false},
		{"no labels", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "x", Labels: tt.labels}}
			if got := IsManagedNamespace(ns); got != tt.want {
				t.Errorf("IsManagedNamespace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsGatewayNamespaceForGC(t *testing.T) {
	tests := []struct {
		name string
		ns   string
		want bool
	}{
		{"gateway namespace", "openshell-a14873d1631f1b74", true},
		{"e2e orphan", "openshell-e2e-orphan-123", true},
		{"managed database namespace", "openshell-db-a1b2c3d4e5f67890", false},
		{"unmanaged", "openshell-gw", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := managedNamespace(tt.ns, nil)
			if tt.name == "unmanaged" {
				ns.Labels = nil
			}
			if got := IsGatewayNamespaceForGC(ns); got != tt.want {
				t.Errorf("IsGatewayNamespaceForGC() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeleteManagedNamespace(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes a managed namespace", func(t *testing.T) {
		client := fake.NewSimpleClientset(managedNamespace("openshell-gw", nil))
		deleted, err := DeleteManagedNamespace(ctx, client, "openshell-gw")
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
		deleted, err := DeleteManagedNamespace(ctx, client, "shared")
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

	t.Run("absent namespace is a no-op success", func(t *testing.T) {
		client := fake.NewSimpleClientset()
		deleted, err := DeleteManagedNamespace(ctx, client, "gone")
		if err != nil {
			t.Fatalf("DeleteManagedNamespace() error = %v", err)
		}
		if deleted {
			t.Errorf("deleted = true, want false for absent namespace")
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
