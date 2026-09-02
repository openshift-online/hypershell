package reconciler

import (
	"context"
	"errors"
	"testing"

	pb "github.com/openshift-online/hypershell/components/api-server/pkg/api/grpc/hypershell/v1"
	"github.com/openshift-online/hypershell/components/control-plane/internal/watcher"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func deploymentCleanupObjects(namespace string) []runtime.Object {
	return []runtime.Object{
		&unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "openshell-gateway-db", "namespace": namespace}}},
		&unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Service", "metadata": map[string]interface{}{"name": "openshell-gateway-db", "namespace": namespace}}},
		&unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": map[string]interface{}{"name": "openshell-gateway-db-data", "namespace": namespace}}},
		&unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]interface{}{"name": "openshell-db-credentials", "namespace": namespace}}},
	}
}
func TestManagedDatabaseDeleteUsesTombstoneAndIsIdempotent(t *testing.T) {
	const namespace = "database-ns"
	dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), deploymentCleanupObjects(namespace)...)
	typed := kubernetesfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
	r := NewManagedDatabaseReconciler(dynamic, typed, nil, "hypershell")
	event := watcher.Event[*pb.ManagedDatabase]{Type: watcher.EventDeleted, ResourceID: "db-1", Resource: &pb.ManagedDatabase{Name: "database", Namespace: namespace, Provider: "deployment"}}
	if err := r.Handle(context.Background(), event); err != nil {
		t.Fatalf("delete: %v", err)
	}
	deploymentGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	if _, err := dynamic.Resource(deploymentGVR).Namespace(namespace).Get(context.Background(), "openshell-gateway-db", metav1.GetOptions{}); err == nil {
		t.Fatal("deployment was not deleted")
	}
	if _, err := typed.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{}); err == nil {
		t.Fatal("namespace was not deleted")
	}
	if err := r.Handle(context.Background(), event); err != nil {
		t.Fatalf("duplicate delete: %v", err)
	}
}

func TestManagedDatabaseDeleteNilTombstoneUsesLastSeenAndRetainsOnFailure(t *testing.T) {
	const namespace = "database-ns"
	dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), deploymentCleanupObjects(namespace)...)
	typed := kubernetesfake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
	r := NewManagedDatabaseReconciler(dynamic, typed, nil, "hypershell")
	fail := true
	dynamic.PrependReactor("delete", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		if fail {
			return true, nil, errors.New("delete failed")
		}
		return false, nil, nil
	})
	deleted := watcher.Event[*pb.ManagedDatabase]{Type: watcher.EventDeleted, ResourceID: "db-1", Resource: &pb.ManagedDatabase{Name: "database", Namespace: namespace, Provider: "deployment"}}
	if err := r.Handle(context.Background(), deleted); err == nil {
		t.Fatal("want cleanup failure")
	}
	if r.lastSeenManagedDatabase("db-1") == nil {
		t.Fatal("cache was removed after failed cleanup")
	}
	fail = false
	deleted.Resource = nil // Simulate an older API server retry carrying only the ID.
	if err := r.Handle(context.Background(), deleted); err != nil {
		t.Fatalf("retry delete: %v", err)
	}
	if r.lastSeenManagedDatabase("db-1") != nil {
		t.Fatal("cache was not removed after successful cleanup")
	}
}

func TestManagedDatabaseReconcilerNilClientsReturnsError(t *testing.T) {
	r := NewManagedDatabaseReconciler(nil, nil, nil, "")
	err := r.Handle(context.Background(), watcher.Event[*pb.ManagedDatabase]{Type: watcher.EventDeleted, ResourceID: "db-1", Resource: &pb.ManagedDatabase{Namespace: "must-not-guess", Provider: "deployment"}})
	if err == nil {
		t.Fatal("want nil client error")
	}
}

func TestDeleteCNPGClusterPropagatesErrorsAndNotFoundIsIdempotent(t *testing.T) {
	dynamic := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	typed := kubernetesfake.NewSimpleClientset()
	r := NewManagedDatabaseReconciler(dynamic, typed, nil, "hypershell")
	dynamic.PrependReactor("delete", "clusters", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("CNPG unavailable")
	})
	if err := r.deleteCNPGCluster(context.Background(), "database-ns"); err == nil {
		t.Fatal("want CNPG cleanup error")
	}
	dynamic = dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	r.dynamicClient = dynamic
	if err := r.deleteCNPGCluster(context.Background(), "database-ns"); err != nil {
		t.Fatalf("NotFound cleanup must be idempotent: %v", err)
	}
}

func TestStripOpenShiftPostgresSecurityContext(t *testing.T) {
	deployment := &unstructured.Unstructured{Object: map[string]interface{}{"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{
		"securityContext": map[string]interface{}{"runAsUser": int64(999), "runAsGroup": int64(999), "fsGroup": int64(999), "fsGroupChangePolicy": "OnRootMismatch", "runAsNonRoot": true, "seccompProfile": map[string]interface{}{"type": "RuntimeDefault"}},
		"containers":      []interface{}{map[string]interface{}{"securityContext": map[string]interface{}{"runAsUser": int64(999), "runAsGroup": int64(999), "runAsNonRoot": true, "readOnlyRootFilesystem": true, "allowPrivilegeEscalation": false, "capabilities": map[string]interface{}{"drop": []interface{}{"ALL"}}}}},
		"initContainers":  []interface{}{map[string]interface{}{"securityContext": map[string]interface{}{"runAsUser": int64(999), "runAsGroup": int64(999), "runAsNonRoot": true, "readOnlyRootFilesystem": true, "allowPrivilegeEscalation": false}}},
	}}}}}
	stripped := stripOpenShiftPostgresSecurityContext(deployment)
	pod, _, _ := unstructured.NestedMap(stripped.Object, "spec", "template", "spec", "securityContext")
	for _, field := range []string{"runAsUser", "runAsGroup", "fsGroup", "fsGroupChangePolicy"} {
		if _, ok := pod[field]; ok {
			t.Fatalf("pod %s was retained", field)
		}
	}
	if pod["runAsNonRoot"] != true {
		t.Fatal("runAsNonRoot was removed")
	}
	for _, containerField := range []string{"containers", "initContainers"} {
		containers, _, _ := unstructured.NestedSlice(stripped.Object, "spec", "template", "spec", containerField)
		security := containers[0].(map[string]interface{})["securityContext"].(map[string]interface{})
		for _, field := range []string{"runAsUser", "runAsGroup"} {
			if _, ok := security[field]; ok {
				t.Fatalf("%s %s was retained", containerField, field)
			}
		}
		if security["allowPrivilegeEscalation"] != false || security["readOnlyRootFilesystem"] != true || security["runAsNonRoot"] != true {
			t.Fatalf("%s hardening was removed", containerField)
		}
	}
	originalPod, _, _ := unstructured.NestedMap(deployment.Object, "spec", "template", "spec", "securityContext")
	if originalPod["runAsUser"] != int64(999) {
		t.Fatal("transform mutated vanilla deployment")
	}
}
