package gateway

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"testing"
)

func TestMemoryRequestChangesOnlyGatewayContainer(t *testing.T) {
	t.Setenv("GATEWAY_MEMORY_REQUEST", "128Mi")
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"spec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{"containers": []interface{}{
		map[string]interface{}{"name": "openshell-gateway", "resources": map[string]interface{}{"limits": map[string]interface{}{"memory": "512Mi"}}},
		map[string]interface{}{"name": "other", "resources": map[string]interface{}{"requests": map[string]interface{}{"memory": "32Mi"}}},
	}}}}}}
	if err := applyMemoryRequest(obj); err != nil {
		t.Fatal(err)
	}
	containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	gateway := containers[0].(map[string]interface{})
	other := containers[1].(map[string]interface{})
	request, _, _ := unstructured.NestedString(gateway, "resources", "requests", "memory")
	limit, _, _ := unstructured.NestedString(gateway, "resources", "limits", "memory")
	unchanged, _, _ := unstructured.NestedString(other, "resources", "requests", "memory")
	if request != "128Mi" || limit != "512Mi" || unchanged != "32Mi" {
		t.Fatalf("unexpected resources: %#v", containers)
	}
}
