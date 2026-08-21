package otel

import "testing"

func TestCanonicalizePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			"core namespace resource",
			"/api/v1/namespaces/openshell-abc123/secrets/db-password",
			"/api/v1/namespaces/{name}/secrets/{name}",
		},
		{
			"list pods in namespace",
			"/api/v1/namespaces/openshell-abc123/pods",
			"/api/v1/namespaces/{name}/pods",
		},
		{
			"deployment in namespace",
			"/apis/apps/v1/namespaces/openshell-abc123/deployments/openshell-gateway",
			"/apis/apps/v1/namespaces/{name}/deployments/{name}",
		},
		{
			"configmap",
			"/api/v1/namespaces/openshell-abc123/configmaps/gateway-config",
			"/api/v1/namespaces/{name}/configmaps/{name}",
		},
		{
			"cluster-scoped resource unchanged",
			"/api/v1/nodes",
			"/api/v1/nodes",
		},
		{
			"service account",
			"/api/v1/namespaces/ns/serviceaccounts/sa-name",
			"/api/v1/namespaces/{name}/serviceaccounts/{name}",
		},
		{
			"networkpolicy",
			"/apis/networking.k8s.io/v1/namespaces/ns/networkpolicies/deny-all",
			"/apis/networking.k8s.io/v1/namespaces/{name}/networkpolicies/{name}",
		},
		{
			"rolebinding",
			"/apis/rbac.authorization.k8s.io/v1/namespaces/ns/rolebindings/rb",
			"/apis/rbac.authorization.k8s.io/v1/namespaces/{name}/rolebindings/{name}",
		},
		{
			"clusterrole is cluster-scoped but named",
			"/apis/rbac.authorization.k8s.io/v1/clusterroles/admin",
			"/apis/rbac.authorization.k8s.io/v1/clusterroles/{name}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalizePath(tt.path)
			if got != tt.want {
				t.Errorf("canonicalizePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
