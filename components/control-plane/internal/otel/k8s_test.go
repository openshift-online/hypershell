package otel

import (
	"strings"
	"testing"
)

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
		{
			"httproute",
			"/apis/gateway.networking.k8s.io/v1/namespaces/ns/httproutes/my-route",
			"/apis/gateway.networking.k8s.io/v1/namespaces/{name}/httproutes/{name}",
		},
		{
			"grpcroute",
			"/apis/gateway.networking.k8s.io/v1/namespaces/ns/grpcroutes/my-grpc",
			"/apis/gateway.networking.k8s.io/v1/namespaces/{name}/grpcroutes/{name}",
		},
		{
			"backendtlspolicy",
			"/apis/gateway.networking.k8s.io/v1alpha3/namespaces/ns/backendtlspolicies/tls-pol",
			"/apis/gateway.networking.k8s.io/v1alpha3/namespaces/{name}/backendtlspolicies/{name}",
		},
		{
			"openshift route",
			"/apis/route.openshift.io/v1/namespaces/ns/routes/gw-route",
			"/apis/route.openshift.io/v1/namespaces/{name}/routes/{name}",
		},
		{
			"cnpg cluster",
			"/apis/postgresql.cnpg.io/v1/namespaces/ns/clusters/openshell-db",
			"/apis/postgresql.cnpg.io/v1/namespaces/{name}/clusters/{name}",
		},
		{
			"cert-manager certificate",
			"/apis/cert-manager.io/v1/namespaces/ns/certificates/my-cert",
			"/apis/cert-manager.io/v1/namespaces/{name}/certificates/{name}",
		},
		{
			"cert-manager issuer",
			"/apis/cert-manager.io/v1/namespaces/ns/issuers/my-issuer",
			"/apis/cert-manager.io/v1/namespaces/{name}/issuers/{name}",
		},
		{
			"gateway resource (previously missing from allowlist)",
			"/apis/gateway.networking.k8s.io/v1/namespaces/ns/gateways/my-gw",
			"/apis/gateway.networking.k8s.io/v1/namespaces/{name}/gateways/{name}",
		},
		{
			"persistentvolumeclaim (previously missing from allowlist)",
			"/api/v1/namespaces/ns/persistentvolumeclaims/data-vol",
			"/api/v1/namespaces/{name}/persistentvolumeclaims/{name}",
		},
		{
			"cnpg database (previously missing from allowlist)",
			"/apis/postgresql.cnpg.io/v1/namespaces/ns/databases/app-db",
			"/apis/postgresql.cnpg.io/v1/namespaces/{name}/databases/{name}",
		},
		{
			"cnpg databaserole (previously missing from allowlist)",
			"/apis/postgresql.cnpg.io/v1/namespaces/ns/databaseroles/reader",
			"/apis/postgresql.cnpg.io/v1/namespaces/{name}/databaseroles/{name}",
		},
		{
			"cluster-scoped named resource",
			"/api/v1/nodes/worker-01",
			"/api/v1/nodes/{name}",
		},
		{
			"subresource path",
			"/api/v1/namespaces/ns/pods/my-pod/log",
			"/api/v1/namespaces/{name}/pods/{name}/log",
		},
		{
			"non-api path passthrough",
			"/healthz",
			"/healthz",
		},
		{
			"empty path passthrough",
			"/",
			"/",
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

func TestCanonicalizePathNoIdentifierLeak(t *testing.T) {
	seeded := []struct {
		path        string
		identifiers []string
	}{
		{
			"/api/v1/namespaces/openshell-abc123/secrets/db-password",
			[]string{"openshell-abc123", "db-password"},
		},
		{
			"/apis/gateway.networking.k8s.io/v1/namespaces/my-ns/httproutes/my-route",
			[]string{"my-ns", "my-route"},
		},
		{
			"/apis/postgresql.cnpg.io/v1/namespaces/db-ns/clusters/openshell-db",
			[]string{"db-ns", "openshell-db"},
		},
		{
			"/apis/gateway.networking.k8s.io/v1/namespaces/prod/gateways/main-gw",
			[]string{"prod", "main-gw"},
		},
		{
			"/api/v1/namespaces/tenant-42/persistentvolumeclaims/data-vol-0",
			[]string{"tenant-42", "data-vol-0"},
		},
	}

	for _, tt := range seeded {
		canonical := canonicalizePath(tt.path)
		for _, id := range tt.identifiers {
			if strings.Contains(canonical, id) {
				t.Errorf("canonicalizePath(%q) = %q still contains identifier %q", tt.path, canonical, id)
			}
		}
	}
}
