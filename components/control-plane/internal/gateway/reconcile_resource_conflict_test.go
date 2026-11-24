package gateway

import (
	"context"
	"testing"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func grpcRoute(namespace, hostname string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "GRPCRoute",
		"metadata":   map[string]interface{}{"name": "openshell-gateway", "namespace": namespace},
		"spec":       map[string]interface{}{"hostnames": []interface{}{hostname}},
	}}
}

// Verifies: STD-RCON-02.03
func TestReconcileResourceRetriesAnUpdateConflict(t *testing.T) {
	const namespace = "openshell-tenant"
	routeGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "grpcroutes"}
	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{routeGVR: "GRPCRouteList"},
		grpcRoute(namespace, "old.example.com"))

	// Another writer changes the route between the first read and update.
	updates := 0
	dc.PrependReactor("update", "grpcroutes", func(k8stesting.Action) (bool, runtime.Object, error) {
		updates++
		if updates == 1 {
			return true, nil, k8serrors.NewConflict(routeGVR.GroupResource(), "openshell-gateway", nil)
		}
		return false, nil, nil
	})

	if err := reconcileResource(context.Background(), dc, grpcRoute(namespace, "new.example.com")); err != nil {
		t.Fatalf("reconcileResource must retry a conflict, got %v", err)
	}
	if updates != 2 {
		t.Fatalf("want one conflicting and one successful update, got %d updates", updates)
	}
	got, err := dc.Resource(routeGVR).Namespace(namespace).Get(context.Background(), "openshell-gateway", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	hosts, _, _ := unstructured.NestedStringSlice(got.Object, "spec", "hostnames")
	if len(hosts) != 1 || hosts[0] != "new.example.com" {
		t.Fatalf("the retried update must apply the desired spec, got hostnames %v", hosts)
	}
}

// Verifies: STD-RCON-02.03
func TestReconcileResourceGivesUpOnPersistentConflict(t *testing.T) {
	const namespace = "openshell-tenant"
	routeGVR := schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "grpcroutes"}
	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{routeGVR: "GRPCRouteList"},
		grpcRoute(namespace, "old.example.com"))
	dc.PrependReactor("update", "grpcroutes", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, k8serrors.NewConflict(routeGVR.GroupResource(), "openshell-gateway", nil)
	})

	err := reconcileResource(context.Background(), dc, grpcRoute(namespace, "new.example.com"))
	if err == nil || !k8serrors.IsConflict(err) {
		t.Fatalf("a persistent conflict must surface as a conflict error, got %v", err)
	}
}
