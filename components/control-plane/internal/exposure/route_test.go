package exposure

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// routeObj builds an OpenShift Route unstructured with the given per-ingress
// Admitted condition statuses. A nil conds slice means status.ingress is absent.
func routeObj(namespace, name string, admittedStatuses []string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "route.openshift.io/v1",
			"kind":       "Route",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{},
		},
	}
	if admittedStatuses != nil {
		ingress := make([]interface{}, 0, len(admittedStatuses))
		for _, s := range admittedStatuses {
			ingress = append(ingress, map[string]interface{}{
				"host": name + "." + namespace,
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Admitted",
						"status": s,
						"reason": "HostAlreadyClaimed",
					},
				},
			})
		}
		obj.Object["status"] = map[string]interface{}{"ingress": ingress}
	}
	return obj
}

func newRouteClient(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{
		routeGVR: "RouteList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, objs...)
}

func TestRouteObserveReadiness(t *testing.T) {
	tests := []struct {
		name      string
		route     *unstructured.Unstructured // nil means the Route does not exist
		wantReady bool
	}{
		{
			name:      "admitted true is ready",
			route:     routeObj("openshell-tenant-a", gatewayRouteName, []string{"True"}),
			wantReady: true,
		},
		{
			name:      "admitted false is not ready",
			route:     routeObj("openshell-tenant-a", gatewayRouteName, []string{"False"}),
			wantReady: false,
		},
		{
			name:      "any admitted true among shards is ready",
			route:     routeObj("openshell-tenant-a", gatewayRouteName, []string{"False", "True"}),
			wantReady: true,
		},
		{
			name:      "no ingress yet is not ready",
			route:     routeObj("openshell-tenant-a", gatewayRouteName, nil),
			wantReady: false,
		},
		{
			name:      "missing route is not ready",
			route:     nil,
			wantReady: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var client *dynamicfake.FakeDynamicClient
			if tt.route != nil {
				client = newRouteClient(tt.route)
			} else {
				client = newRouteClient()
			}

			adapter := NewRouteExposure(client)
			got, err := adapter.ObserveReadiness(context.Background(), Request{Namespace: "openshell-tenant-a"})
			if err != nil {
				t.Fatalf("ObserveReadiness returned error: %v", err)
			}
			if got.Ready != tt.wantReady {
				t.Fatalf("Ready = %v, want %v (reason=%q)", got.Ready, tt.wantReady, got.Reason)
			}
			if !got.Ready && got.Reason == "" {
				t.Fatalf("expected a reason when not ready")
			}
		})
	}
}

func TestRouteResolveAddress(t *testing.T) {
	adapter := NewRouteExposure(newRouteClient())

	t.Run("derived from base domain", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.com")
		got, err := adapter.ResolveAddress(context.Background(), Request{Namespace: "tenant-a"})
		if err != nil {
			t.Fatalf("ResolveAddress error: %v", err)
		}
		if want := "grpcs://gw-tenant-a.apps.example.com:443"; got != want {
			t.Fatalf("address = %q, want %q", got, want)
		}
	})

	t.Run("empty when base domain unset", func(t *testing.T) {
		t.Setenv("GATEWAY_API_BASE_DOMAIN", "")
		got, err := adapter.ResolveAddress(context.Background(), Request{Namespace: "tenant-a"})
		if err != nil {
			t.Fatalf("ResolveAddress error: %v", err)
		}
		if got != "" {
			t.Fatalf("address = %q, want empty", got)
		}
	})
}
