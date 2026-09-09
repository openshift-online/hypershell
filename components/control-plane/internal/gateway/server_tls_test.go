package gateway

import (
	"context"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubernetesfake "k8s.io/client-go/kubernetes/fake"
)

func TestServerIssuerPreservesClientCAAndRouteSAN(t *testing.T) {
	ctx := context.Background()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	t.Setenv("GATEWAY_API_BASE_DOMAIN", "apps.example.test")
	cfg := NamespaceConfig{Name: "gateway-a", Gateway: GatewayConfig{ServerDnsNames: []string{"openshell-gateway.gateway-a.svc.cluster.local"}}}
	route, err := deriveGatewayHostname(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Gateway.ServerDnsNames = appendDNSNameIfMissing(cfg.Gateway.ServerDnsNames, route)
	certificates := client.Resource(schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}).Namespace(cfg.Name)
	for _, setting := range []string{"", " shared-server-ca ", ""} {
		t.Setenv("GATEWAY_SERVER_TLS_CLUSTER_ISSUER", setting)
		if err := reconcileCertManagerResources(ctx, client, cfg); err != nil {
			t.Fatal(err)
		}
		for name, want := range map[string]string{"openshell-ca": "openshell-selfsigned", "openshell-client": "openshell-ca-issuer", "openshell-server": "openshell-ca-issuer"} {
			kind := "Issuer"
			if name == "openshell-server" && strings.TrimSpace(setting) != "" {
				want = strings.TrimSpace(setting)
				kind = "ClusterIssuer"
			}
			cert, err := certificates.Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			actual, _, _ := unstructured.NestedString(cert.Object, "spec", "issuerRef", "name")
			actualKind, _, _ := unstructured.NestedString(cert.Object, "spec", "issuerRef", "kind")
			if actual != want || actualKind != kind {
				t.Fatalf("%s issuer=%s/%s; want %s/%s", name, actualKind, actual, kind, want)
			}
			if name == "openshell-server" {
				dns, _, _ := unstructured.NestedStringSlice(cert.Object, "spec", "dnsNames")
				if !reflect.DeepEqual(dns, cfg.Gateway.ServerDnsNames) {
					t.Fatalf("server SANs changed: %v", dns)
				}
			}
		}
	}
}

func TestSandboxTLSSeparatesServerTrustAndReconcilesRotation(t *testing.T) {
	ctx := context.Background()
	t.Setenv("GATEWAY_SERVER_TLS_CLUSTER_ISSUER", "shared-ca")
	server := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "openshell-server-tls", Namespace: "gateway", Annotations: map[string]string{"cert-manager.io/issuer-name": "shared-ca", "cert-manager.io/issuer-kind": "ClusterIssuer"}}, Data: map[string][]byte{"ca.crt": []byte("server-ca"), "tls.crt": []byte("server-certificate"), "tls.key": []byte("server-private-key")}}
	originalClient := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "openshell-client-tls", Namespace: "gateway", UID: "client-secret-id"}, Data: map[string][]byte{"ca.crt": []byte("client-ca"), "tls.crt": []byte("client-certificate"), "tls.key": []byte("client-private-key")}}
	client := kubernetesfake.NewSimpleClientset(server, originalClient)
	if err := ReconcileSandboxTLS(ctx, client, "gateway"); err != nil {
		t.Fatal(err)
	}
	payload, err := client.CoreV1().Secrets("gateway").Get(ctx, "openshell-sandbox-tls", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(payload.Data["ca.crt"]) != "server-ca" || string(payload.Data["tls.crt"]) != "client-certificate" || string(payload.Data["tls.key"]) != "client-private-key" || len(payload.Data) != 3 {
		t.Fatal("sandbox TLS mixed server private material or wrong trust")
	}
	if len(payload.OwnerReferences) != 1 || payload.OwnerReferences[0].UID != originalClient.UID {
		t.Fatal("payload has no source owner")
	}
	untouched, err := client.CoreV1().Secrets("gateway").Get(ctx, originalClient.Name, metav1.GetOptions{})
	if err != nil || !reflect.DeepEqual(untouched.Data, originalClient.Data) {
		t.Fatal("client auth CA was changed")
	}
	client.ClearActions()
	if err := ReconcileSandboxTLS(ctx, client, "gateway"); err != nil {
		t.Fatal(err)
	}
	for _, action := range client.Actions() {
		if action.GetVerb() == "update" || action.GetVerb() == "create" {
			t.Fatal("unchanged material caused a write")
		}
	}
	server.Data["ca.crt"] = []byte("rotated-server-ca")
	originalClient.Data["tls.key"] = []byte("rotated-client-key")
	if _, err := client.CoreV1().Secrets("gateway").Update(ctx, server, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().Secrets("gateway").Update(ctx, originalClient, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileSandboxTLS(ctx, client, "gateway"); err != nil {
		t.Fatal(err)
	}
	payload, err = client.CoreV1().Secrets("gateway").Get(ctx, "openshell-sandbox-tls", metav1.GetOptions{})
	if err != nil || string(payload.Data["ca.crt"]) != "rotated-server-ca" || string(payload.Data["tls.key"]) != "rotated-client-key" {
		t.Fatal("rotated payload was not applied")
	}
	server.Annotations["cert-manager.io/issuer-name"] = "old-ca"
	if _, err := client.CoreV1().Secrets("gateway").Update(ctx, server, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileSandboxTLS(ctx, client, "gateway"); err == nil {
		t.Fatal("old issuer material was accepted")
	}
	t.Setenv("GATEWAY_SERVER_TLS_CLUSTER_ISSUER", "")
	client.ClearActions()
	if err := ReconcileSandboxTLS(ctx, client, "gateway"); err != nil {
		t.Fatal(err)
	}
	if len(client.Actions()) != 0 {
		t.Fatal("default issuer mode changed client TLS behavior")
	}
}

func TestSharedServerIssuerSelectsSeparateSandboxPayload(t *testing.T) {
	for _, issuer := range []string{"", "shared-ca"} {
		t.Setenv("GATEWAY_SERVER_TLS_CLUSTER_ISSUER", issuer)
		original := `client_tls_secret_name = "openshell-client-tls"` + "\nserver_sans = []\n[openshell.gateway.tls]\ncert_path = \"/server/tls.crt\""
		cm := &unstructured.Unstructured{Object: map[string]interface{}{"kind": "ConfigMap", "metadata": map[string]interface{}{"name": "openshell-gateway-config"}, "data": map[string]interface{}{"gateway.toml": original}}}
		if err := ApplyConfigOverrides(cm, GatewayConfig{ServerDnsNames: []string{"gateway.example.test"}}); err != nil {
			t.Fatal(err)
		}
		result, _, _ := unstructured.NestedString(cm.Object, "data", "gateway.toml")
		want := "openshell-client-tls"
		if issuer != "" {
			want = "openshell-sandbox-tls"
		}
		if !strings.Contains(result, `client_tls_secret_name = "`+want+`"`) || strings.Contains(result, "client_ca_path") {
			t.Fatal("wrong sandbox secret or broadened gateway client authentication")
		}
	}
}

func TestSharedIssuerPreservesTemplateSANWithoutOverride(t *testing.T) {
	t.Setenv("GATEWAY_SERVER_TLS_CLUSTER_ISSUER", "shared-ca")
	original := `client_tls_secret_name = "openshell-client-tls"` + "\n" + `server_sans = ["gateway.internal"]`
	cm := &unstructured.Unstructured{Object: map[string]interface{}{"kind": "ConfigMap", "metadata": map[string]interface{}{"name": "openshell-gateway-config"}, "data": map[string]interface{}{"gateway.toml": original}}}
	if err := ApplyConfigOverrides(cm, GatewayConfig{}); err != nil {
		t.Fatal(err)
	}
	result, _, _ := unstructured.NestedString(cm.Object, "data", "gateway.toml")
	if !strings.Contains(result, `server_sans = ["gateway.internal"]`) {
		t.Fatal("issuer selection removed template SAN")
	}
}
